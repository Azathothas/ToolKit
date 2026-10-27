// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// builtWithLine is what `base ensure` and `ready` print for the fake helper's build.
const builtWithLine = `built with  podman for linux/amd64, answering from host "build-host.example"`

// fakeBaseHelper is a helper on this machine whose base reads unusable, and whose
// ensure answers a build podman made on build-host.example, as the state
// directory's endpoint names it.
func fakeBaseHelper(t *testing.T, home string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/status":
			_, _ = fmt.Fprintf(w, `{"schema":"x","version":"8.0.0","pid":%d}`, os.Getpid())
		case "/v1/base/status":
			_, _ = fmt.Fprintf(w, `{"schema":%q,"state":{"name":"wsl-toolkit","registered":false}}`, toolkit.HelperSchema)
		case "/v1/base/ensure":
			_, _ = fmt.Fprintf(w, `{"schema":%q,"state":{"name":"wsl-toolkit","registered":true,"healthy":true,`+
				`"built_with":{"name":"podman","platform":"linux/amd64","host":"build-host.example"}}}`, toolkit.HelperSchema)
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
	if err := forksafe.WriteFile(filepath.Join(home, "helper.json"), ep, 0o600); err != nil {
		t.Fatal(err)
	}
	return srv
}

// ⛔ WSL-188: `base ensure` THROUGH THE HELPER SAYS WHICH ENGINE BUILT THE BASE, AND
// ON WHICH HOST. Only the build's log said it, and through the helper that log is
// the helper's. The human answer prints it, and `--json` carries it.
func TestABaseEnsureThroughTheHelperSaysWhereItWasBuilt(t *testing.T) {
	home := quietCLI(t)
	fakeBaseHelper(t, home)
	if got := captureStderr(t, func() { _ = run(context.Background(), []string{"base", "ensure", "--via-helper"}) }); !strings.Contains(got, builtWithLine) {
		t.Fatalf("base ensure through the helper does not say %q:\n%s", builtWithLine, got)
	}
	out := captureStdout(t, func() { _ = run(context.Background(), []string{"base", "ensure", "--via-helper", "--json"}) })
	var st toolkit.BaseState
	if err := json.Unmarshal([]byte(out), &st); err != nil || st.BuiltWith == nil || st.BuiltWith.Host != "build-host.example" {
		t.Fatalf("base ensure --json through the helper answered %+v, %v: %q", st.BuiltWith, err, out)
	}
}

// ⛔ AND `ready --ensure` THROUGH THE HELPER SAYS IT, in its report and its text.
func TestReadyEnsureThroughTheHelperSaysWhereItWasBuilt(t *testing.T) {
	home := quietCLI(t)
	fakeBaseHelper(t, home)
	ctx := context.Background()
	client, err := toolkit.DialHelper(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := toolkit.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	var r ReadyReport
	r.Route.Selected = "helper"
	_ = captureStderr(t, func() { r.fillBase(ctx, cfg, nil, client, true) })
	if r.Base.BuiltWith == nil || r.Base.BuiltWith.Host != "build-host.example" {
		t.Fatalf("ready --ensure through the helper reported built_with %+v; problems %v", r.Base.BuiltWith, r.Problems)
	}
	if got := captureStderr(t, func() { renderReady(r) }); !strings.Contains(got, builtWithLine) {
		t.Fatalf("ready does not say %q:\n%s", builtWithLine, got)
	}
}
