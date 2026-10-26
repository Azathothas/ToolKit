// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// -- WSL-176: a pull's stall is sized from its largest layer -----------------

// pullCaseRef is the image these cases pull through the fake podman.
const pullCaseRef = "ghcr.io/pkgforge-dev/archlinux:latest"

// fakeManifestFile is the file in the fake host's state that answers
// `manifest inspect REF`.
func fakeManifestFile(ref string) string {
	return "manifest-" + strings.NewReplacer("/", "_", ":", "_", "@", "_").Replace(ref)
}

// lowerPullFloor makes a pull's floor a span a case can wait out.
//
// ⭐ THE FLOOR IS THE ONLY THING SCALED. The stall watch, the fake's silence
// and the sizing are the real ones, so a case drives the trigger in seconds.
func lowerPullFloor(t *testing.T, d time.Duration) {
	t.Helper()
	was := pullStallFloor
	pullStallFloor = d
	t.Cleanup(func() { pullStallFloor = was })
}

// layersManifest is an image manifest with these layer sizes.
func layersManifest(sizes ...int64) string {
	var parts []string
	for _, s := range sizes {
		parts = append(parts, fmt.Sprintf(`{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip","size":%d,"digest":"sha256:%064d"}`, s, s))
	}
	return `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","layers":[` + strings.Join(parts, ",") + `]}`
}

// otherArch is an architecture this machine does not run.
func otherArch() string {
	if runtime.GOARCH == "arm64" {
		return "amd64"
	}
	return "arm64"
}

// pullCaseEngine is the fake podman, found the way a build finds it.
func pullCaseEngine(t *testing.T, h *fakeHost) *Engine {
	t.Helper()
	h.engine(t, true)
	e, err := FindEngine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "podman" {
		t.Fatalf("the fake host answered %s, not its podman", e.Name)
	}
	return e
}

// ⛔ THE TRIGGER, DRIVEN. A layer that takes longer than the floor is silent
// for longer than the floor, as podman is while it downloads one. The index
// names this platform's image, whose largest layer takes 20 seconds at the
// floor rate, so the pull may be silent 20 seconds. The fake is silent for 3,
// against a floor of 1, and finishes.
func TestAPullSilentWhileItsLargestLayerDownloadsIsNotGivenUp(t *testing.T) {
	h := newFakeHost(t)
	e := pullCaseEngine(t, h)
	lowerPullFloor(t, time.Second)
	repo := imageRepository(pullCaseRef)
	mine, theirs := "sha256:"+strings.Repeat("b", 64), "sha256:"+strings.Repeat("a", 64)
	h.say(t, fakeManifestFile(pullCaseRef), `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[`+
		`{"digest":"`+theirs+`","platform":{"architecture":"`+otherArch()+`","os":"linux"}},`+
		`{"digest":"sha256:`+strings.Repeat("c", 64)+`","platform":{"architecture":"unknown","os":"unknown"}},`+
		`{"digest":"`+mine+`","platform":{"architecture":"`+e.Arch+`","os":"linux"}}]}`)
	// ⚠ The other platform's layer would give 2 seconds, which the fake outlasts.
	h.say(t, fakeManifestFile(repo+"@"+theirs), layersManifest(2*PullFloorRate))
	h.say(t, fakeManifestFile(repo+"@"+mine), layersManifest(1000, 20*PullFloorRate, 3000))
	h.say(t, "pull-silent", "3s")
	var said sayings
	tar := filepath.Join(t.TempDir(), "rootfs.tar")
	if err := e.ExportRootfs(context.Background(), pullCaseRef, tar, said.log); err != nil {
		t.Fatalf("a pull silent for 3s while a 20s layer downloads was given up: %v", err)
	}
	if want := "It may be silent for 20s, the time its largest layer, 5.0 MiB, takes at 256.0 KiB/s"; !said.said(want) {
		t.Errorf("the build did not say how long its pull may be silent, %q: %v", want, said.lines)
	}
	calls := h.calls(t)
	index, image, pull := callAt(calls, "podman manifest inspect "+pullCaseRef), callAt(calls, "podman manifest inspect "+repo+"@"+mine), callAt(calls, "podman pull")
	if index < 0 || image < index || pull < image {
		t.Errorf("the layers were not read from this platform's image before the pull: %v", calls)
	}
}

// ⛔ A layer list that cannot be read leaves the floor, and the refusal of a
// pull that outlasts it says a slow link reads the same and names the variable.
// ⭐ WSL_TOOLKIT_PULL_STALL then names the limit outright, and a value that is
// no duration refuses before anything is pulled.
func TestTheStallVariableNamesAPullsLimitAndTheRefusalSaysWhy(t *testing.T) {
	h := newFakeHost(t)
	e := pullCaseEngine(t, h)
	lowerPullFloor(t, time.Second)
	h.say(t, "pull-silent", "3s")
	tar := filepath.Join(t.TempDir(), "rootfs.tar")

	t.Setenv(PullStallEnv, "")
	var floor sayings
	err := e.ExportRootfs(context.Background(), pullCaseRef, tar, floor.log)
	if err == nil {
		t.Fatal("a pull silent for 3s against a floor of 1s was not given up")
	}
	for _, want := range []string{"produced nothing for 1s", "STALL", "reads as silence too", PullStallEnv} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "not a slow link") {
		t.Errorf("the refusal still says a slow link cannot be the cause: %v", err)
	}
	if !floor.said("its layers could not be read") {
		t.Errorf("the build did not say why its pull has the floor: %v", floor.lines)
	}

	t.Setenv(PullStallEnv, "20s")
	var named sayings
	if err := e.ExportRootfs(context.Background(), pullCaseRef, tar, named.log); err != nil {
		t.Fatalf("%s=20s did not let a pull silent for 3s finish: %v", PullStallEnv, err)
	}
	if !named.said("It may be silent for 20s, as " + PullStallEnv + " says") {
		t.Errorf("the build did not name the variable it took: %v", named.lines)
	}

	before := len(h.calls(t))
	for _, bad := range []string{"soon", "0s", "-1m"} {
		t.Setenv(PullStallEnv, bad)
		if err := e.ExportRootfs(context.Background(), pullCaseRef, tar, func(string) {}); err == nil || !strings.Contains(err.Error(), PullStallEnv) {
			t.Errorf("%s=%s answered %v; want a refusal naming it", PullStallEnv, bad, err)
		}
	}
	if after := h.calls(t)[before:]; callAt(after, "podman pull") >= 0 {
		t.Errorf("a value that is no duration still pulled: %v", after)
	}
}

// TestAPullsLimitsAreSizedFromItsLayers is the rule with no process in it.
func TestAPullsLimitsAreSizedFromItsLayers(t *testing.T) {
	// The default base's one layer, and gentoo's, from the audit's finding 7.
	for _, c := range []struct {
		largest, total int64
		stall, ceiling time.Duration
	}{
		{193_600_000, 193_600_000, 12*time.Minute + 19*time.Second, EngineTransferTimeout},
		{427_000_000, 430_000_000, 27*time.Minute + 9*time.Second, EngineTransferTimeout},
		// ⚠ A small layer keeps the floor.
		{10 << 20, 12 << 20, EngineStallTimeout, EngineTransferTimeout},
		// ⛔ An image that takes longer than thirty minutes at the floor rate gets
		// that long, or its last layers would be given up as a stall's ceiling.
		{600_000_000, 1_000_000_000, 38*time.Minute + 9*time.Second, 63*time.Minute + 35*time.Second},
	} {
		lim := sizedPullLimit(c.largest, c.total)
		if lim.Stall != c.stall || lim.Total != c.ceiling || !lim.Sized {
			t.Errorf("layers of %d and %d bytes are given %s silent and %s in all, sized %v; want %s and %s",
				c.largest, c.total, lim.Stall, lim.Total, lim.Sized, c.stall, c.ceiling)
		}
	}
	for ref, want := range map[string]string{
		"ghcr.io/pkgforge-dev/archlinux:latest":                      "ghcr.io/pkgforge-dev/archlinux",
		"docker.io/library/debian:trixie@sha256:" + "0123":           "docker.io/library/debian",
		"localhost:5000/team/image":                                  "localhost:5000/team/image",
		"localhost:5000/team/image:v1":                               "localhost:5000/team/image",
		"registry.example:443/a/b@sha256:" + strings.Repeat("f", 64): "registry.example:443/a/b",
	} {
		if got := imageRepository(ref); got != want {
			t.Errorf("imageRepository(%q) = %q, want %q", ref, got, want)
		}
	}
}

// ⛔ An engine that answers an index for the image itself names no layer, and
// reading it again would answer the same. Two reads, then an error, never a
// loop. An index with no image for this platform is an error too.
func TestALayerListIsReadAtMostTwiceAndOnlyForThisPlatform(t *testing.T) {
	index := func(arch, digest string) []byte {
		return []byte(`{"manifests":[{"digest":"` + digest + `","platform":{"os":"linux","architecture":"` + arch + `"}}]}`)
	}
	reads := 0
	again := func(context.Context, string) ([]byte, error) {
		reads++
		return index("amd64", "sha256:"+strings.Repeat("d", 64)), nil
	}
	if _, _, err := layerBytes(context.Background(), again, pullCaseRef, "amd64"); err == nil || reads != 2 {
		t.Errorf("an engine that answers an index for the image itself was read %d times and answered %v; want 2 and an error", reads, err)
	}
	other := func(context.Context, string) ([]byte, error) {
		return index("riscv64", "sha256:"+strings.Repeat("e", 64)), nil
	}
	if _, _, err := layerBytes(context.Background(), other, pullCaseRef, "amd64"); err == nil || !strings.Contains(err.Error(), "no linux/amd64 image") {
		t.Errorf("an index with no image for this platform answered %v", err)
	}
	refused := func(context.Context, string) ([]byte, error) { return nil, errors.New("manifest unknown") }
	if _, _, err := layerBytes(context.Background(), refused, pullCaseRef, "amd64"); err == nil {
		t.Error("a manifest the engine refused was read as one")
	}
	direct := func(context.Context, string) ([]byte, error) { return []byte(layersManifest(5, 9, 2)), nil }
	if largest, total, err := layerBytes(context.Background(), direct, pullCaseRef, "amd64"); err != nil || largest != 9 || total != 16 {
		t.Errorf("an image manifest answered %d and %d, %v; want 9 and 16", largest, total, err)
	}
}
