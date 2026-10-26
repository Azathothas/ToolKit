// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ⛔ WSL-188: A BUILD THROUGH THE HELPER NAMES THE ENGINE IT BUILT WITH, AND ITS
// HOST. The host was a line of the build's log, and through the helper that log is
// the helper's, so a caller whose podman connection named another machine of this
// architecture got a base built there without a word. The helper's answer carries
// it now, and a plain ensure after the build carries none.
func TestABuildThroughTheHelperNamesItsEngineAndHost(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	_ = newFakeRegistry(t)
	h.say(t, "info-podman", nativeArch()+" linux build-host.example")
	h.say(t, "provision.out", "wsl-conf unchanged\nprovision-complete\n")
	h.say(t, "grants.out", "fstab unchanged\ngrants-complete\n")
	h.say(t, "identity-write", "ok")
	h.stageGuest(t, "verify=ok", "binfmt=ok")
	srv := &HelperServer{cfg: b.cfg, log: func(string) {}, runners: map[string]*Runner{},
		runner: &Runner{cfg: b.cfg, base: b, wsl: b.wsl, home: b.home, log: func(string) {}}}
	ensure := func(body string) BaseState {
		t.Helper()
		rec := httptest.NewRecorder()
		srv.handleBaseEnsure(rec, httptest.NewRequest(http.MethodPost, "/v1/base/ensure", strings.NewReader(body)))
		var out struct {
			Error string    `json:"error"`
			State BaseState `json:"state"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("the helper answered %q: %v", rec.Body.String(), err)
		}
		if out.Error != "" {
			t.Fatalf("the helper's %s answered %s; calls %v", body, out.Error, h.calls(t))
		}
		return out.State
	}
	built := ensure(`{"force":true}`)
	want := BuildEngine{Name: "podman", Platform: "linux/" + nativeArch(), Host: "build-host.example"}
	if built.BuiltWith == nil || *built.BuiltWith != want {
		t.Fatalf("a build through the helper answered built_with %+v; want %+v", built.BuiltWith, want)
	}
	if got := built.BuiltWith.String(); !strings.Contains(got, `answering from host "build-host.example"`) {
		t.Errorf("the engine reads %q, which does not name its host", got)
	}
	// ⛔ THE HELPER KEEPS ONE Base, and a plain ensure built nothing.
	if again := ensure(`{}`); again.BuiltWith != nil {
		t.Fatalf("a plain ensure after the build answered built_with %+v, which it did not build with", again.BuiltWith)
	}
}
