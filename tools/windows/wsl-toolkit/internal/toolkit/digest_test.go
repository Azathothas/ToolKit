// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// -- WSL-186: the digest a base was built from is recorded ---------------------
// -- WSL-192: a pull that fails builds from the engine's own copy --------------

// fakeDigest is the digest the fake engine names for the image it holds.
const fakeDigest = "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// fakeImage is the engine's own copy of image, for this machine's platform, as
// podman names it: its own digest, and the repository's.
func fakeImage(image string) engineImage {
	return engineImage{
		Digest:       fakeDigest,
		RepoDigests:  []string{repositoryOf(image) + "@" + fakeDigest},
		Architecture: runtime.GOARCH, Os: "linux",
	}
}

// fakeBuildable stages every answer a build needs: an engine that answers and
// holds img, a volume with room, the provisioner's and the grants step's last
// lines, the marker's write, and a verifier and binfmt step that pass.
func fakeBuildable(t *testing.T, h *fakeHost, img engineImage) {
	t.Helper()
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	data, err := json.Marshal([]engineImage{img})
	if err != nil {
		t.Fatal(err)
	}
	h.say(t, "image.json", string(data))
	h.say(t, "provision.out", "wsl-conf unchanged\nprovision-complete\n")
	h.say(t, "grants.out", "fstab unchanged\ngrants-complete\n")
	h.say(t, "identity-write", "ok")
	h.stageGuest(t, "verify=ok", "binfmt=ok")
}

// guestMarker is the marker the fake guest holds.
func guestMarker(t *testing.T, h *fakeHost) Identity {
	t.Helper()
	var id Identity
	data, err := os.ReadFile(filepath.Join(h.state, "identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &id); err != nil {
		t.Fatalf("the marker %q does not parse: %v", data, err)
	}
	return id
}

// ⛔ THE DEFECT: the default base is a tag rebuilt daily, and nothing recorded
// which image a base came from. After a bad daily build the working digest was
// unknown. The marker, this machine's record and `base status` carry it now.
func TestABuildRecordsTheDigestItWasBuiltFrom(t *testing.T) {
	h := newFakeHost(t)
	b, s, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	fakeBuildable(t, h, fakeImage(DefaultBaseImage))
	if _, err := b.EnsureWith(context.Background(), true, false); err != nil {
		t.Fatalf("a rebuild with every answer staged failed: %v, %v", err, s.lines)
	}
	if callAt(h.calls(t), "wsl --import "+fakeBaseName) < 0 {
		t.Fatalf("the case never reached the import it is about: %v", h.calls(t))
	}
	if id := guestMarker(t, h); id.Digest != fakeDigest || id.Image != DefaultBaseImage {
		t.Fatalf("the marker reads %+v; want the image and its digest %s", id, fakeDigest)
	}
	rec, err := b.readRecord()
	if err != nil || rec.Digest != fakeDigest {
		t.Fatalf("the record reads %+v, %v; want the digest %s", rec, err, fakeDigest)
	}
	st, err := b.Status(context.Background(), false)
	if err != nil || st.Digest != fakeDigest {
		t.Fatalf("status reads the digest %q, %v; want %s", st.Digest, err, fakeDigest)
	}
	if !s.said("digest: " + fakeDigest) {
		t.Fatalf("the build did not say the digest it read: %v", s.lines)
	}
}

// ⭐ THE GUEST'S MARKER IS THE AUTHORITY. A probe reads the digest from it, and an
// ensure writes this machine's record from it, as it does the image.
func TestTheMarkersDigestIsReadAndRecorded(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	marker, err := json.Marshal(Identity{
		Schema: IdentitySchema, Name: fakeBaseName, Image: DefaultBaseImage, User: b.cfg.Base.User,
		Built: time.Now().UTC(), Digest: fakeDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.say(t, "identity.json", string(marker))
	h.stageGuest(t, "verify=ok", "binfmt=ok")
	if rec, err := b.readRecord(); err != nil || rec.Digest != "" {
		t.Fatalf("the case's record already carries a digest: %+v, %v", rec, err)
	}
	st, err := b.Status(context.Background(), true)
	if err != nil || st.Digest != fakeDigest {
		t.Fatalf("a probe reads the digest %q, %v; want the marker's %s", st.Digest, err, fakeDigest)
	}
	if _, err := b.EnsureWith(context.Background(), false, false); err != nil {
		t.Fatalf("an ensure over a healthy base failed: %v", err)
	}
	if rec, err := b.readRecord(); err != nil || rec.Digest != fakeDigest {
		t.Fatalf("the record an ensure wrote from the marker reads %+v, %v; want the digest %s", rec, err, fakeDigest)
	}
}

// ⭐ A BASE AN OLDER RELEASE BUILT IS STAMPED WITH WHAT ITS RECORD KNOWS, the
// digest included.
func TestAStampCarriesTheRecordsDigest(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	b.exported = ExportedImage{Digest: fakeDigest}
	if err := b.writeRecord(); err != nil {
		t.Fatal(err)
	}
	b.exported = ExportedImage{}
	if err := os.Remove(filepath.Join(h.state, "identity.json")); err != nil {
		t.Fatal(err)
	}
	h.say(t, "identity-write", "ok")
	h.stageGuest(t, "verify=ok", "binfmt=ok")
	if _, err := b.EnsureWith(context.Background(), false, false); err != nil {
		t.Fatalf("an ensure over a base with no marker failed: %v", err)
	}
	if id := guestMarker(t, h); id.Digest != fakeDigest {
		t.Fatalf("the stamp reads %+v; want the record's digest %s", id, fakeDigest)
	}
}

// ⛔ THE DIGEST IS THE IMAGE'S OWN, OR ITS REPOSITORY'S. podman names its own, and
// docker names one per repository the image was pulled from. Another repository's
// digest pins another image, so it is never taken.
func TestTheDigestIsTheEnginesOrItsRepositorys(t *testing.T) {
	other := "sha256:" + strings.Repeat("e", 64)
	for _, c := range []struct {
		why  string
		ref  string
		img  engineImage
		want string
	}{
		{"podman's own", DefaultBaseImage, engineImage{Digest: fakeDigest, RepoDigests: []string{"ghcr.io/pkgforge-dev/archlinux@" + other}}, fakeDigest},
		{"docker's, for the repository asked for", DefaultBaseImage, engineImage{RepoDigests: []string{"docker.io/library/alpine@" + other, "ghcr.io/pkgforge-dev/archlinux@" + fakeDigest}}, fakeDigest},
		{"a short name as docker spells it", "docker.io/library/alpine:latest", engineImage{RepoDigests: []string{"alpine@" + fakeDigest}}, fakeDigest},
		{"another repository's only", DefaultBaseImage, engineImage{RepoDigests: []string{"docker.io/library/alpine@" + other}}, ""},
		{"none that is a digest", DefaultBaseImage, engineImage{Digest: "sha256:short", RepoDigests: []string{"ghcr.io/pkgforge-dev/archlinux@sha256:NOTHEX"}}, ""},
	} {
		if got := c.img.digestFor(c.ref); got != c.want {
			t.Errorf("%s: %s names %q; want %q", c.why, c.ref, got, c.want)
		}
	}
	if got := PinnedImage("docker.io/library/alpine:latest", fakeDigest); got != "docker.io/library/alpine@"+fakeDigest {
		t.Errorf("the pin of a tag reads %q", got)
	}
	if got := PinnedImage("localhost:5000/base:v1", fakeDigest); got != "localhost:5000/base@"+fakeDigest {
		t.Errorf("the pin of a registry with a port reads %q", got)
	}
	if got := PinnedImage(DefaultBaseImage, ""); got != "" {
		t.Errorf("no digest pins %q", got)
	}
}

// ⛔ THE DEFECT: a rebuild whose pull failed ended there, even where the engine
// already held the image for this machine. A host the registry does not answer
// could not rebuild its base at all. It builds from that copy now, and says so.
func TestARebuildWhosePullFailsBuildsFromTheEnginesOwnCopy(t *testing.T) {
	h := newFakeHost(t)
	b, s, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	fakeBuildable(t, h, fakeImage(DefaultBaseImage))
	h.say(t, "pull", "refused")
	if _, err := b.EnsureWith(context.Background(), true, false); err != nil {
		t.Fatalf("a rebuild whose pull failed, over an engine holding the image, answered %v: %v", err, s.lines)
	}
	calls := h.calls(t)
	pulled := callAt(calls, "podman pull")
	inspected := callAt(calls, "podman image inspect "+DefaultBaseImage)
	exported := callAt(calls, "podman export")
	imported := callAt(calls, "wsl --import "+fakeBaseName)
	if pulled < 0 || inspected < pulled || exported < inspected || imported < exported {
		t.Fatalf("want the pull, the engine's own copy read, its export, then the import; the calls were %v", calls)
	}
	if !s.said("building from the copy of "+DefaultBaseImage+" that podman already holds") || !s.said(fakeDigest) {
		t.Fatalf("the build did not say it used a local copy, and which: %v", s.lines)
	}
	if id := guestMarker(t, h); id.Digest != fakeDigest {
		t.Fatalf("the marker reads %+v; want the local copy's digest", id)
	}
}

// ⚠ A PULL THAT WAS STOPPED IS NOT A PULL THAT FAILED. Nothing stands in for it,
// and the answer is the stop, not a word about the engine's own copy.
func TestAStoppedPullIsNotStoodInFor(t *testing.T) {
	h := newFakeHost(t)
	fakeBuildable(t, h, fakeImage(DefaultBaseImage))
	h.say(t, "pull-silent", "20s")
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	e := &Engine{Name: "podman", Path: filepath.Join(filepath.Dir(h.wsl), "podman"+ext), Arch: runtime.GOARCH}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if data, err := os.ReadFile(filepath.Join(h.state, "calls")); err == nil && strings.Contains(string(data), "podman pull") {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()
	s := &sayings{}
	_, err := e.ExportBaseRootfs(ctx, DefaultBaseImage, filepath.Join(t.TempDir(), "rootfs.tar"), s.log)
	if err == nil || strings.Contains(err.Error(), "holds no copy") || s.said("building from the copy") {
		t.Fatalf("a pull stopped by its caller answered %v, saying %v; want the stop alone", err, s.lines)
	}
}

// ⛔ A COPY FOR ANOTHER PLATFORM IS NEVER BUILT FROM. The store is keyed by tag, so
// one `--platform` pull leaves another machine's image under it, and its rootfs
// imports cleanly and runs nothing. The base is kept.
func TestAPullThatFailsBuildsFromNoCopyForAnotherPlatform(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	img := fakeImage(DefaultBaseImage)
	img.Architecture = "s390x"
	fakeBuildable(t, h, img)
	h.say(t, "pull", "refused")
	_, err := b.EnsureWith(context.Background(), true, false)
	if err == nil || !strings.Contains(err.Error(), fakeBaseName+" was NOT removed") || !strings.Contains(err.Error(), "linux/s390x") ||
		!strings.Contains(err.Error(), "manifest unknown") {
		t.Fatalf("a failed pull over another platform's copy answered %v; want the refusal naming both", err)
	}
	assertKept(t, h, b, disk)
	if i := callAt(h.calls(t), "podman create"); i >= 0 {
		t.Fatalf("another platform's copy was exported: %v", h.calls(t))
	}
}
