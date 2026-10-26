// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-191: a `--preset` switch through the helper is a switch -------------

// fakeEnsureHelper is a helper on this machine, as the state directory's endpoint
// names it, that keeps each base/ensure body it is sent and answers an empty state.
func fakeEnsureHelper(t *testing.T, home string) *[]map[string]any {
	t.Helper()
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/status":
			_, _ = fmt.Fprintf(w, `{"schema":"x","version":"8.0.0","pid":%d}`, os.Getpid())
		case "/v1/base/ensure":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			bodies = append(bodies, body)
			_, _ = fmt.Fprintf(w, `{"schema":%q,"state":{}}`, toolkit.HelperSchema)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, `{"error":"this fake answers no %s"}`, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	ep, err := json.Marshal(toolkit.HelperEndpoint{Schema: toolkit.HelperSchema, Address: strings.TrimPrefix(srv.URL, "http://"), Token: "t", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "helper.json"), ep, 0o600); err != nil {
		t.Fatal(err)
	}
	return &bodies
}

// ⛔ THE DEFECT: `base ensure --preset` through the helper crossed as a forced
// rebuild, so a second switch rebuilt the base the first had just built. It
// crosses as a switch the helper decides under the base's lock. ⛔ And without
// `--save` the helper built from the file's image, because the client sends the
// file it read: the preset's image crosses now. `base recreate` still forces.
func TestAPresetSwitchThroughTheHelperCrossesAsASwitch(t *testing.T) {
	home := quietCLI(t)
	t.Setenv(toolkit.PullStallEnv, "")
	bodies := fakeEnsureHelper(t, home)
	debian, err := toolkit.ResolveBasePreset("debian")
	if err != nil {
		t.Fatal(err)
	}
	wasQuiet := quiet
	t.Cleanup(func() { quiet = wasQuiet })
	quiet = false
	said := captureStderr(t, func() {
		run(context.Background(), []string{"base", "ensure", "--via-helper", "--preset", "debian"})
		run(context.Background(), []string{"base", "recreate", "--via-helper"})
	})
	// ⛔ THROUGH THE HELPER THE SWITCH IS A REQUEST, AND THE LINE SAYS SO. It said
	// the base rebuilds, and the helper's word that it did not stayed in its log.
	if !strings.Contains(said, "so this asks the helper to switch it") || strings.Contains(said, "so this rebuilds it") {
		t.Errorf("a switch through the helper said:\n%s\nwant the request, and no word of a rebuild", said)
	}
	image := func(body map[string]any) any {
		c, _ := body["config"].(map[string]any)
		base, _ := c["base"].(map[string]any)
		return base["image"]
	}
	if len(*bodies) != 2 {
		t.Fatalf("the helper was sent %d ensure(s); want 2: %v", len(*bodies), *bodies)
	}
	switched, recreated := (*bodies)[0], (*bodies)[1]
	if switched["switch"] != true || switched["force"] != false || image(switched) != debian {
		t.Errorf("an unsaved --preset switch crossed as %v; want a switch, no force, and the image %s", switched, debian)
	}
	if recreated["force"] != true || recreated["switch"] == true || image(recreated) != toolkit.DefaultBaseImage {
		t.Errorf("base recreate crossed as %v; want force alone, and the file's image", recreated)
	}
}

// -- WSL-192: a job's base built from the engine's own copy is said ----------

// ⛔ THE DEFECT: through the helper, `run --ensure-base` over a base the helper
// built from the engine's own copy said nothing to its caller. The helper's log
// said it. The state the helper answers carries it, and the caller says it.
func TestARunThroughTheHelperSaysItsBaseWasBuiltFromALocalCopy(t *testing.T) {
	wasQuiet := quiet
	t.Cleanup(func() { quiet = wasQuiet })
	quiet = false
	built := toolkit.BaseState{Remediations: []toolkit.Remediation{{
		ID: toolkit.BuiltFromLocalCopyID, Advisory: true, Repairable: true,
		What:  "the pull of x failed, so the base was built from the copy podman already held",
		Costs: "that copy may be older than the registry's image",
	}}}
	said := captureStderr(t, func() {
		if err := helperEnsureBase(context.Background(), func(context.Context, bool) (toolkit.BaseState, error) { return built, nil }); err != nil {
			t.Errorf("an ensure that built answered %v", err)
		}
	})
	if !strings.Contains(said, "⚠ the pull of x failed, so the base was built from the copy podman already held") ||
		!strings.Contains(said, "older than the registry's image") {
		t.Errorf("a run whose base the helper built from a local copy said:\n%s", said)
	}
	if said := captureStderr(t, func() {
		_ = helperEnsureBase(context.Background(), func(context.Context, bool) (toolkit.BaseState, error) { return toolkit.BaseState{}, nil })
	}); said != "" {
		t.Errorf("a run whose base was not built said:\n%s", said)
	}
}

// -- WSL-191: a case answers for the registry the configuration reads --------

// ⛔ A CASE HERE READ THE OPERATOR'S OWN REGISTRY, so a host with both default
// names refused every configuration a case loaded. The registry a case supplies
// is the one the configuration reads: both names refuse, and none resolves.
func TestTheRegistryACaseSuppliesIsTheOneTheConfigurationReads(t *testing.T) {
	quietCLI(t)
	// ⚠ THE DEFAULT AS A COMMAND SELECTS IT, with no distribution named, so the
	// name is resolved from the registry.
	toolkit.SelectedInstance = toolkit.Instance{Name: toolkit.DefaultInstance}
	both := map[string]string{
		toolkit.DefaultBaseName: filepath.Join(t.TempDir(), "base"),
		toolkit.LegacyBaseName:  filepath.Join(t.TempDir(), "base"),
	}
	restore := toolkit.RegistrationsForCase(func() (map[string]string, error) { return both, nil })
	_, err := toolkit.LoadConfig()
	restore()
	if !errors.Is(err, toolkit.ErrDefaultBaseAmbiguous) {
		t.Fatalf("a configuration over a registry with both default names answered %v; want it refused", err)
	}
	cfg, err := toolkit.LoadConfig()
	if err != nil || cfg.Base.Name != toolkit.DefaultBaseName {
		t.Fatalf("a configuration over an empty registry answered %q, %v; want %s", cfg.Base.Name, err, toolkit.DefaultBaseName)
	}
}

// -- WSL-186: `base status` names the image at the digest it was built from ---

// ⭐ THE ROW NAMES THE IMAGE AT ITS DIGEST, as `base.image` would name it to build
// the same base again. A base an older release built has no digest, and no row.
func TestBaseStatusNamesTheDigestItWasBuiltFrom(t *testing.T) {
	digest := "sha256:" + strings.Repeat("0123456789abcdef", 4)
	st := toolkit.BaseState{Name: "wsl-toolkit", Image: toolkit.DefaultBaseImage, BuiltFrom: toolkit.DefaultBaseImage, Digest: digest}
	out := captureStderr(t, func() { renderBaseState(st, false) })
	if want := "  digest      ghcr.io/pkgforge-dev/archlinux@" + digest + "\n"; !strings.Contains(out, want) {
		t.Errorf("status of a base with a digest prints:\n%s\nwant the row %q", out, want)
	}
	st.Digest = ""
	if out := captureStderr(t, func() { renderBaseState(st, false) }); strings.Contains(out, "digest") {
		t.Errorf("status of a base with no digest prints a digest row:\n%s", out)
	}
}
