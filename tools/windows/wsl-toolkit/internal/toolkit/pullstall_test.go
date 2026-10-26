// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// -- WSL-176: a pull's stall is sized from its largest layer -----------------

// pullCaseRef is the image these cases pull through the fake podman.
const pullCaseRef = "ghcr.io/pkgforge-dev/archlinux:latest"

// roundTripper answers HTTP requests in place of a network.
type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// ⛔ NO CASE READS A REAL REGISTRY. A case that wants layers stages a fake one,
// and every other read is refused at once, as an unreachable registry is.
func init() {
	registryTransport = roundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no case reads a real registry")
	})
}

// fakeRegistry is one case's registry, with no socket in it. Its manifest route
// asks for an anonymous token first, as ghcr.io and Docker Hub do.
type fakeRegistry struct {
	mu        sync.Mutex
	manifests map[string]string
	// served is every manifest it answered, as host and path, in order.
	served []string
	tokens int
}

// fakeRegistryToken is the one token the fake hands out and accepts.
const fakeRegistryToken = "anonymous-pull-token"

// newFakeRegistry makes every registry read in this case reach a fake.
func newFakeRegistry(t *testing.T) *fakeRegistry {
	t.Helper()
	reg := &fakeRegistry{manifests: map[string]string{}}
	was := registryTransport
	registryTransport = roundTripper(reg.answer)
	t.Cleanup(func() { registryTransport = was })
	return reg
}

// stage makes a registry host answer a repository's tag or digest with body.
func (reg *fakeRegistry) stage(host, repo, reference, body string) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.manifests[host+"/v2/"+repo+"/manifests/"+reference] = body
}

func (reg *fakeRegistry) answer(req *http.Request) (*http.Response, error) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	rec := httptest.NewRecorder()
	host, path := req.URL.Host, req.URL.Path
	at := strings.Index(path, "/manifests/")
	switch {
	case path == "/token":
		q := req.URL.Query()
		if q.Get("service") != host || !strings.HasPrefix(q.Get("scope"), "repository:") || req.Header.Get("Authorization") != "" {
			rec.WriteHeader(http.StatusBadRequest)
			break
		}
		reg.tokens++
		_, _ = rec.WriteString(`{"token":"` + fakeRegistryToken + `"}`)
	case strings.HasPrefix(path, "/v2/") && at > 0:
		if req.Header.Get("Authorization") != "Bearer "+fakeRegistryToken {
			rec.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="https://%s/token",service="%s",scope="repository:%s:pull"`,
				host, host, strings.TrimPrefix(path[:at], "/v2/")))
			rec.WriteHeader(http.StatusUnauthorized)
			break
		}
		body, ok := reg.manifests[host+path]
		if !ok {
			rec.WriteHeader(http.StatusNotFound)
			_, _ = rec.WriteString(`{"errors":[{"code":"MANIFEST_UNKNOWN","message":"manifest unknown"}]}`)
			break
		}
		reg.served = append(reg.served, host+path)
		_, _ = rec.WriteString(body)
	default:
		rec.WriteHeader(http.StatusNotFound)
	}
	res := rec.Result()
	res.Request = req
	return res, nil
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

// lineAt is the position of the first line that holds sub, or -1.
func lineAt(lines []string, sub string) int {
	for i, l := range lines {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}

// ⛔ THE TRIGGER, DRIVEN. A layer that takes longer than the floor is silent
// for longer than the floor, as podman is while it downloads one. The registry
// asks for an anonymous token, then answers an index naming this platform's
// image, whose largest layer takes 20 seconds at the floor rate. So the pull
// may be silent 20 seconds. The fake is silent for 3, against a floor of 1.
// ⛔ THE ENGINE IS NOT ASKED FOR THE MANIFEST: a remote podman prints a list's
// shape for an image, with no layers in it.
func TestAPullSilentWhileItsLargestLayerDownloadsIsNotGivenUp(t *testing.T) {
	h := newFakeHost(t)
	e := pullCaseEngine(t, h)
	reg := newFakeRegistry(t)
	lowerPullFloor(t, time.Second)
	t.Setenv(PullStallEnv, "")
	const repo = "pkgforge-dev/archlinux"
	mine, theirs := "sha256:"+strings.Repeat("b", 64), "sha256:"+strings.Repeat("a", 64)
	reg.stage("ghcr.io", repo, "latest", `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[`+
		`{"digest":"`+theirs+`","platform":{"architecture":"`+otherArch()+`","os":"linux"}},`+
		`{"digest":"sha256:`+strings.Repeat("c", 64)+`","platform":{"architecture":"unknown","os":"unknown"}},`+
		`{"digest":"`+mine+`","platform":{"architecture":"`+e.Arch+`","os":"linux"}}]}`)
	// ⚠ The other platform's layer would give 2 seconds, which the fake outlasts.
	reg.stage("ghcr.io", repo, theirs, layersManifest(2*PullFloorRate))
	reg.stage("ghcr.io", repo, mine, layersManifest(1000, 20*PullFloorRate, 3000))
	h.say(t, "pull-silent", "3s")
	var said sayings
	tar := filepath.Join(t.TempDir(), "rootfs.tar")
	if err := e.ExportRootfs(context.Background(), pullCaseRef, tar, said.log); err != nil {
		t.Fatalf("a pull silent for 3s while a 20s layer downloads was given up: %v", err)
	}
	if want := "It may be silent for 20s, the time its largest layer, 5.0 MiB, takes at 256.0 KiB/s"; !said.said(want) {
		t.Errorf("the build did not say how long its pull may be silent, %q: %v", want, said.lines)
	}
	if want := []string{"ghcr.io/v2/" + repo + "/manifests/latest", "ghcr.io/v2/" + repo + "/manifests/" + mine}; !slices.Equal(reg.served, want) || reg.tokens != 1 {
		t.Errorf("the registry served %v with %d token(s); want this platform's image after its index, with one token", reg.served, reg.tokens)
	}
	if read, pulling := lineAt(said.lines, "reading the layers of "+pullCaseRef), lineAt(said.lines, "pulling "+pullCaseRef); read < 0 || pulling < read {
		t.Errorf("the read of the layers was not said before it was made: %v", said.lines)
	}
	if callAt(h.calls(t), "podman manifest") >= 0 {
		t.Errorf("the engine was asked for a manifest, which a remote podman prints with no layers: %v", h.calls(t))
	}
}

// ⚠ Docker Hub answers at registry-1.docker.io, and keeps a one-word
// repository under library/.
func TestDockerHubsOneWordRepositoryIsReadUnderLibrary(t *testing.T) {
	reg := newFakeRegistry(t)
	reg.stage("registry-1.docker.io", "library/alpine", "3.20", layersManifest(7, 11))
	largest, total, err := registryLayerBytes(context.Background(), "docker.io/alpine:3.20", "amd64")
	if err != nil || largest != 11 || total != 18 {
		t.Fatalf("docker.io/alpine:3.20 answered %d and %d, %v; want 11 and 18 from library/alpine", largest, total, err)
	}
}

// ⛔ A layer list that cannot be read leaves the floor, and the refusal of a
// pull that outlasts it says a slow link reads the same, why the limit was the
// floor, and the variable. ⭐ WSL_TOOLKIT_PULL_STALL then names the limit
// outright, and a value that is no duration refuses before anything is pulled.
func TestTheStallVariableNamesAPullsLimitAndTheRefusalSaysWhy(t *testing.T) {
	h := newFakeHost(t)
	e := pullCaseEngine(t, h)
	_ = newFakeRegistry(t)
	lowerPullFloor(t, time.Second)
	h.say(t, "pull-silent", "3s")
	tar := filepath.Join(t.TempDir(), "rootfs.tar")

	t.Setenv(PullStallEnv, "")
	var floor sayings
	err := e.ExportRootfs(context.Background(), pullCaseRef, tar, floor.log)
	if err == nil {
		t.Fatal("a pull silent for 3s against a floor of 1s was not given up")
	}
	for _, want := range []string{"produced nothing for 1s", "STALL", "reads as silence too", PullStallEnv,
		"This limit was the floor of 1s, because its layers could not be read.", "stopped: podman system connection list"} {
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

// ⛔ THE VARIABLE NEVER SHORTENS THE CEILING. It skipped the layers and set the
// ceiling to its own value, so 45m gave a 1 GB image 45 minutes where its
// layers sized 63m35s. The layers are still read, and every layer may take the
// time the variable allows the largest.
func TestTheStallVariableNeverShortensTheCeiling(t *testing.T) {
	for _, c := range []struct {
		stall          time.Duration
		largest, total int64
		ceiling        time.Duration
	}{
		{45 * time.Minute, 1_000_000_000, 1_000_000_000, 63*time.Minute + 35*time.Second},
		// ⚠ A link slower than the floor rate: three layers' worth of 45 minutes.
		{45 * time.Minute, 500_000_000, 1_500_000_000, 135 * time.Minute},
		// Layers that could not be read leave the variable's own span.
		{45 * time.Minute, 0, 0, 45 * time.Minute},
		{10 * time.Minute, 0, 0, EngineTransferTimeout},
	} {
		lim := namedPullLimit(c.stall, c.largest, c.total)
		if lim.Stall != c.stall || lim.Total != c.ceiling {
			t.Errorf("%s over layers of %d and %d bytes gives %s silent and %s in all; want %s and %s",
				c.stall, c.largest, c.total, lim.Stall, lim.Total, c.stall, c.ceiling)
		}
	}
	reg := newFakeRegistry(t)
	reg.stage("ghcr.io", "pkgforge-dev/archlinux", "latest", layersManifest(1_000_000_000))
	t.Setenv(PullStallEnv, "45m")
	lim, err := (&Engine{Name: "podman", Arch: runtime.GOARCH}).pullLimits(context.Background(), pullCaseRef)
	if err != nil || lim.Stall != 45*time.Minute || lim.Total != 63*time.Minute+35*time.Second {
		t.Errorf("%s=45m over a 1 GB layer gives %s silent and %s in all, %v; want 45m and 63m35s", PullStallEnv, lim.Stall, lim.Total, err)
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
	// ⛔ THE FLOOR WON, AND THE SENTENCE SAYS SO. It read "This limit was its
	// largest layer, 10.0 MiB, takes less at 256.0 KiB/s", which is no sentence
	// and names the layer for a limit the floor set.
	small := sizedPullLimit(10<<20, 12<<20)
	if want := " This limit was the floor of 4m00s, because its largest layer, 10.0 MiB, takes less at 256.0 KiB/s."; small.Hint != want {
		t.Errorf("a small layer's refusal says %q; want %q", small.Hint, want)
	}
	if want := "the floor, because its largest layer, 10.0 MiB, takes less at 256.0 KiB/s"; small.Why != want {
		t.Errorf("a small layer's pulling line says %q; want %q", small.Why, want)
	}
	// ⚠ THE HINT NAMES THE ENGINE THAT PULLED, and not podman for every one.
	for engine, want := range map[string]string{"podman": "podman system connection list", "docker": "docker info", "wslc": "wslc version"} {
		if hint := pullStallHint(engine, small); !strings.HasSuffix(hint, "stopped: "+want) {
			t.Errorf("a stalled %s pull's hint ends %q; want %q", engine, hint, want)
		}
	}
	for ref, want := range map[string][3]string{
		"ghcr.io/pkgforge-dev/archlinux:latest":                      {"ghcr.io", "pkgforge-dev/archlinux", "latest"},
		"docker.io/library/debian:trixie@sha256:0123":                {"docker.io", "library/debian", "sha256:0123"},
		"docker.io/alpine":                                           {"docker.io", "library/alpine", "latest"},
		"localhost:5000/team/image":                                  {"localhost:5000", "team/image", "latest"},
		"localhost:5000/team/image:v1":                               {"localhost:5000", "team/image", "v1"},
		"registry.example:443/a/b@sha256:" + strings.Repeat("f", 64): {"registry.example:443", "a/b", "sha256:" + strings.Repeat("f", 64)},
	} {
		if host, repo, reference := splitImageRef(ref); [3]string{host, repo, reference} != want {
			t.Errorf("splitImageRef(%q) = %q, %q, %q; want %q", ref, host, repo, reference, want)
		}
	}
}

// ⛔ A registry that answers an index for the image itself names no layer, and
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
	if _, _, err := layerBytes(context.Background(), again, "latest", "amd64"); err == nil || reads != 2 {
		t.Errorf("a registry that answers an index for the image itself was read %d times and answered %v; want 2 and an error", reads, err)
	}
	other := func(context.Context, string) ([]byte, error) {
		return index("riscv64", "sha256:"+strings.Repeat("e", 64)), nil
	}
	if _, _, err := layerBytes(context.Background(), other, "latest", "amd64"); err == nil || !strings.Contains(err.Error(), "no linux/amd64 image") {
		t.Errorf("an index with no image for this platform answered %v", err)
	}
	refused := func(context.Context, string) ([]byte, error) { return nil, errors.New("manifest unknown") }
	if _, _, err := layerBytes(context.Background(), refused, "latest", "amd64"); err == nil {
		t.Error("a manifest the registry refused was read as one")
	}
	direct := func(context.Context, string) ([]byte, error) { return []byte(layersManifest(5, 9, 2)), nil }
	if largest, total, err := layerBytes(context.Background(), direct, "latest", "amd64"); err != nil || largest != 9 || total != 16 {
		t.Errorf("an image manifest answered %d and %d, %v; want 9 and 16", largest, total, err)
	}
}

// ⚠ A challenge is read as registries write it, and one that asks for a
// credential is no token request.
func TestABearerChallengeIsReadAsRegistriesWriteIt(t *testing.T) {
	for h, want := range map[string]map[string]string{
		`Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/alpine:pull"`: {
			"realm": "https://auth.docker.io/token", "service": "registry.docker.io", "scope": "repository:library/alpine:pull"},
		`Bearer realm="https://ghcr.io/token", service="ghcr.io",scope="repository:a/b:pull,push"`: {
			"realm": "https://ghcr.io/token", "service": "ghcr.io", "scope": "repository:a/b:pull,push"},
		`bearer realm=https://r.example/token,service=r.example`: {"realm": "https://r.example/token", "service": "r.example"},
	} {
		got, ok := bearerChallenge(h)
		if !ok || len(got) != len(want) {
			t.Errorf("%s read as %v, %v; want %v", h, got, ok, want)
			continue
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s read %s as %q; want %q", h, k, got[k], v)
			}
		}
	}
	for _, h := range []string{`Basic realm="Registry"`, `Bearer service="x"`, ``, `Bearer realm="https://unterminated`} {
		if got, ok := bearerChallenge(h); ok {
			t.Errorf("%q was read as a token request: %v", h, got)
		}
	}
}

// ⛔ A VALUE THAT IS NO DURATION REFUSES BEFORE THE ENGINE IS ASKED. It was read
// only at the pull, so a forced rebuild asked the engine first, and a
// `distro new --image --dry-run` passed with exit 0 what the real run refused.
func TestAStallVariableThatIsNoDurationRefusesBeforeTheEngine(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	t.Setenv(PullStallEnv, "soon")
	noEngine := func(what string) {
		t.Helper()
		if i := callAt(h.calls(t), "podman"); i >= 0 {
			t.Fatalf("%s with %s=soon asked the engine: %v", what, PullStallEnv, h.calls(t))
		}
	}

	_, err := b.EnsureWith(context.Background(), true, false)
	if err == nil || !strings.Contains(err.Error(), fakeBaseName+" was NOT removed") || !strings.Contains(err.Error(), PullStallEnv) {
		t.Fatalf("a forced rebuild with %s=soon answered %v", PullStallEnv, err)
	}
	assertKept(t, h, b, disk)
	noEngine("a forced rebuild")

	h.say(t, "registered", "")
	h.say(t, "disks", "")
	if err := os.RemoveAll(b.Dir()); err != nil {
		t.Fatal(err)
	}
	if _, err := b.EnsureWith(context.Background(), false, false); err == nil || !strings.Contains(err.Error(), PullStallEnv) {
		t.Fatalf("a new build with %s=soon answered %v", PullStallEnv, err)
	}
	noEngine("a new build")

	tw := &Throwaways{dir: t.TempDir(), wsl: &Wsl{Path: h.wsl}, log: func(string) {}, disks: h.registry}
	if _, err := tw.Preflight(context.Background(), ThrowawaySpec{Image: "docker.io/library/alpine:latest"}); err == nil ||
		!IsRefusal(err) || !strings.Contains(err.Error(), PullStallEnv) {
		t.Fatalf("distro new --image, and its dry run, with %s=soon answered %v", PullStallEnv, err)
	}
	noEngine("distro new --image")
}

// ⛔ THE HELPER BUILDS WITH ITS CALLER'S STALL, AND NEVER ITS OWN. It read its
// own environment, which is whatever the process that started it held, so the
// variable a caller set reached no pull, and the retry the refusal advised
// failed the same way.
func TestTheHelperBuildsWithItsCallersPullStall(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	_ = newFakeRegistry(t)
	lowerPullFloor(t, time.Second)
	h.say(t, "pull-silent", "3s")
	// ⚠ THE HELPER'S OWN ENVIRONMENT, which would refuse every build were it read.
	t.Setenv(PullStallEnv, "soon")
	srv := &HelperServer{cfg: b.cfg, log: func(string) {}, runners: map[string]*Runner{},
		runner: &Runner{cfg: b.cfg, base: b, wsl: b.wsl, home: b.home, log: func(string) {}}}
	ensure := func(body string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		srv.handleBaseEnsure(rec, httptest.NewRequest(http.MethodPost, "/v1/base/ensure", strings.NewReader(body)))
		var out struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("the helper answered %q: %v", rec.Body.String(), err)
		}
		return out.Error
	}

	// A caller that set none: the floor, which the 3s pull outlasts.
	if got := ensure(`{"force":true}`); !strings.Contains(got, "was given up") || strings.Contains(got, `"soon"`) {
		t.Fatalf("a caller that set no stall was answered %q; want the floor's refusal, with the helper's own value unread", got)
	}
	// A caller that set 20s: the pull is let through.
	if got := ensure(`{"force":true,"pull_stall_ms":20000}`); strings.Contains(got, "was given up") || strings.Contains(got, PullStallEnv) {
		t.Fatalf("a caller that set a 20s stall was answered %q", got)
	}
	if callAt(h.calls(t), "podman export") < 0 {
		t.Fatalf("the pull a caller gave 20s never reached the export: %v", h.calls(t))
	}
	if got := ensure(`{"force":true,"pull_stall_ms":-1}`); !strings.Contains(got, "pull_stall_ms") {
		t.Errorf("a negative stall was answered %q", got)
	}
}

// ⛔ THE CLIENT SENDS ITS STALL, and refuses one that is no duration before the
// helper is asked anything.
func TestTheClientSendsItsPullStallToTheHelper(t *testing.T) {
	_, c, _, _ := ownedElsewhere(t, "wsl-toolkit")
	cfg := c.load(t)
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema":"` + HelperSchema + `","state":{}}`))
	}))
	defer srv.Close()
	client := &HelperClient{endpoint: HelperEndpoint{Address: strings.TrimPrefix(srv.URL, "http://"), Token: "t"}, http: srv.Client(), cfg: cfg}

	t.Setenv(PullStallEnv, "45m")
	if _, err := client.BaseEnsureWith(context.Background(), false, false); err != nil {
		t.Fatal(err)
	}
	t.Setenv(PullStallEnv, "")
	if _, err := client.BaseEnsureWith(context.Background(), false, false); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || bodies[0]["pull_stall_ms"] != float64(45*time.Minute/time.Millisecond) {
		t.Fatalf("a caller's 45m crossed the wire as %v", bodies)
	}
	if _, set := bodies[1]["pull_stall_ms"]; set {
		t.Fatalf("a caller that set no stall sent one: %v", bodies[1])
	}
	t.Setenv(PullStallEnv, "soon")
	if _, err := client.BaseEnsureWith(context.Background(), false, false); err == nil || !strings.Contains(err.Error(), PullStallEnv) || len(bodies) != 2 {
		t.Fatalf("a stall that is no duration answered %v, after %d request(s); want a refusal before any", err, len(bodies))
	}
}
