// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-191: a `--preset` switch through the helper is a switch -------------

// helperOver is a helper whose own runner is b, and a call to its base/ensure
// route that answers the error it sent, or empty.
func helperOver(t *testing.T, b *Base) func(body string) (int, string) {
	t.Helper()
	srv := &HelperServer{cfg: b.cfg, log: func(string) {}, runners: map[string]*Runner{},
		runner: &Runner{cfg: b.cfg, base: b, wsl: b.wsl, home: b.home, log: func(string) {}}}
	return func(body string) (int, string) {
		t.Helper()
		rec := httptest.NewRecorder()
		srv.handleBaseEnsure(rec, httptest.NewRequest(http.MethodPost, "/v1/base/ensure", strings.NewReader(body)))
		var out struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("the helper answered %q: %v", rec.Body.String(), err)
		}
		return rec.Code, out.Error
	}
}

// helperStateOver is helperOver answering the state the helper sent too.
func helperStateOver(t *testing.T, b *Base) func(body string) (int, string, BaseState) {
	t.Helper()
	srv := &HelperServer{cfg: b.cfg, log: func(string) {}, runners: map[string]*Runner{},
		runner: &Runner{cfg: b.cfg, base: b, wsl: b.wsl, home: b.home, log: func(string) {}}}
	return func(body string) (int, string, BaseState) {
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
		return rec.Code, out.Error, out.State
	}
}

// remediationNamed is the remediation id in st, or false.
func remediationNamed(st BaseState, id string) (Remediation, bool) {
	for _, r := range st.Remediations {
		if r.ID == id {
			return r, true
		}
	}
	return Remediation{}, false
}

// countCalls is how many calls start with prefix.
func countCalls(calls []string, prefix string) int {
	n := 0
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// ⛔ THE DEFECT: through the helper a `--preset` switch crossed as a forced
// rebuild, so a second switch rebuilt the base the first had just built from the
// same image. The helper decides it under the base's lock now: two switches, one
// build.
func TestAHelperSwitchedTwiceBuildsOnce(t *testing.T) {
	const next = "docker.io/library/debian:trixie"
	h := newFakeHost(t)
	b, s, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	fakeBuildable(t, h, fakeImage(next))
	b.cfg.Base.Image = next
	ensure := helperOver(t, b)
	for i := 1; i <= 2; i++ {
		if code, msg := ensure(`{"switch":true}`); code != http.StatusOK || msg != "" {
			t.Fatalf("switch %d answered %d %q: %v", i, code, msg, s.lines)
		}
	}
	calls := h.calls(t)
	if built := countCalls(calls, "wsl --import "+fakeBaseName); built != 1 || countCalls(calls, "wsl --unregister "+fakeBaseName) != 1 {
		t.Fatalf("two switches built %d time(s); want once. The calls were %v", built, calls)
	}
	if id := guestMarker(t, h); id.Image != next {
		t.Fatalf("the base the switch built reads %+v; want it built from %s", id, next)
	}
	if !s.said(fakeBaseName + " is already built from " + next + ", so it is not rebuilt") {
		t.Fatalf("the second switch did not say why it built nothing: %v", s.lines)
	}
}

// ⭐ `base recreate` through the helper is unchanged: it rebuilds a base already
// built from the image. ⚠ A switch and a recreate at once is two answers to one
// question, and refused with nothing changed.
func TestAHelperRecreateStillRebuildsAndASwitchWithItIsRefused(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	fakeBuildable(t, h, fakeImage(DefaultBaseImage))
	ensure := helperOver(t, b)
	code, msg := ensure(`{"force":true,"switch":true}`)
	if code != http.StatusBadRequest || !strings.Contains(msg, "send one") {
		t.Fatalf("force and switch together answered %d %q; want a refusal", code, msg)
	}
	if _, err := os.Stat(filepath.Join(h.state, "calls")); err == nil {
		t.Fatalf("a refused request still asked WSL or the engine: %v", h.calls(t))
	}
	if code, msg := ensure(`{"force":true}`); code != http.StatusOK || msg != "" {
		t.Fatalf("a recreate answered %d %q", code, msg)
	}
	if countCalls(h.calls(t), "wsl --import "+fakeBaseName) != 1 {
		t.Fatalf("a recreate through the helper did not rebuild: %v", h.calls(t))
	}
}

// ⛔ THE CLIENT SENDS A SWITCH AS A SWITCH, and force only for `base recreate`.
// ⭐ The image a `--preset` named crosses with it, and the client it came from
// keeps the file's.
func TestTheClientSendsASwitchApartFromARecreate(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema":"` + HelperSchema + `","state":{}}`))
	}))
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.Base.Image = DefaultBaseImage
	client := &HelperClient{endpoint: HelperEndpoint{Address: strings.TrimPrefix(srv.URL, "http://"), Token: "t"}, http: srv.Client(), cfg: cfg}
	t.Setenv(PullStallEnv, "")

	const next = "docker.io/library/debian:latest"
	if _, err := client.WithBaseImage(next).BaseEnsureSwitch(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.BaseEnsureWith(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.BaseEnsureWith(context.Background(), false, false); err != nil {
		t.Fatal(err)
	}
	image := func(body map[string]any) any {
		c, _ := body["config"].(map[string]any)
		base, _ := c["base"].(map[string]any)
		return base["image"]
	}
	if len(bodies) != 3 {
		t.Fatalf("the client sent %d request(s); want 3", len(bodies))
	}
	if bodies[0]["switch"] != true || bodies[0]["force"] != false || image(bodies[0]) != next {
		t.Errorf("a switch crossed as %v; want switch, no force, and the image %s", bodies[0], next)
	}
	if bodies[1]["force"] != true || bodies[1]["switch"] == true || image(bodies[1]) != DefaultBaseImage {
		t.Errorf("a recreate crossed as %v; want force alone, and the file's image", bodies[1])
	}
	if bodies[2]["force"] != false || bodies[2]["switch"] == true {
		t.Errorf("an ensure crossed as %v; want neither", bodies[2])
	}
	// ⛔ AND switch IS NOT SENT AT ALL BUT FOR A SWITCH. A helper built before it
	// refuses a field it does not know, so a plain ensure that carried it was
	// refused there.
	for i, body := range bodies[1:] {
		if _, sent := body["switch"]; sent {
			t.Errorf("request %d carried switch: %v; want the field left out", i+2, body)
		}
	}
}

// ⛔ THE DEFECT: a base built from the engine's own copy said so on the build's
// log alone, and through the helper that log is the helper's. The state the helper
// answers carries it now, and a later ensure of the same base does not.
func TestAHelperBuildFromTheEnginesOwnCopySaysSoInItsState(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	fakeBuildable(t, h, fakeImage(DefaultBaseImage))
	h.say(t, "pull", "refused")
	ensure := helperStateOver(t, b)
	code, msg, st := ensure(`{"force":true}`)
	if code != http.StatusOK || msg != "" {
		t.Fatalf("a recreate whose pull failed answered %d %q", code, msg)
	}
	r, ok := remediationNamed(st, BuiltFromLocalCopyID)
	if !ok || !r.Advisory || !strings.Contains(r.What, fakeDigest) || !strings.Contains(r.What, "podman") ||
		!strings.Contains(r.Costs, "older than the registry's") {
		t.Fatalf("the state the helper answered carries %+v; want the advisory naming the copy and its digest", st.Remediations)
	}
	code, msg, st = ensure(`{}`)
	if code != http.StatusOK || msg != "" {
		t.Fatalf("an ensure after it answered %d %q", code, msg)
	}
	if _, ok := remediationNamed(st, BuiltFromLocalCopyID); ok {
		t.Fatalf("an ensure that built nothing says the base was built from a local copy: %+v", st.Remediations)
	}
}
