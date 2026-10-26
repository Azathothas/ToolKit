// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"encoding/json"
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
	_ = captureStderr(t, func() {
		run(context.Background(), []string{"base", "ensure", "--via-helper", "--preset", "debian"})
		run(context.Background(), []string{"base", "recreate", "--via-helper"})
	})
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

// -- WSL-186: `base status` names the image at the digest it was built from ---

// ⭐ THE ROW NAMES THE IMAGE AT ITS DIGEST, as `base.image` would name it to build
// the same base again. A base an older release built has no digest, and no row.
func TestBaseStatusNamesTheDigestItWasBuiltFrom(t *testing.T) {
	const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
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
