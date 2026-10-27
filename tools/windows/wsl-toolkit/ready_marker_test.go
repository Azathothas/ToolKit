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

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-183: `ready` over a base whose marker cannot be read ------------------

// unreadMarkerHelper is a helper on this machine whose base's marker cannot be
// read. Its probe answers the state this build's helper sends, with the rebuild
// as a remediation. Its ensure refuses, and the state with the refusal carries
// none, as a helper built before the remediation sends it.
func unreadMarkerHelper(t *testing.T, home string) *int {
	t.Helper()
	unread := "the wsl-toolkit identity marker could not be read: the guest could not be started: wsl-toolkit, exit 1: The distribution failed to start."
	if !strings.Contains(unread, toolkit.ErrIdentityUnread.Error()) {
		t.Fatalf("the case's refusal %q is not the toolkit's words", unread)
	}
	probed := toolkit.BaseState{
		Name: toolkit.DefaultBaseName, Registered: true, Probed: true,
		Problems:     []string{unread},
		Remediations: []toolkit.Remediation{toolkit.UnreadMarkerRemediation()},
	}
	refused := probed
	refused.Probed, refused.Remediations = false, nil
	ensures := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/status":
			_, _ = fmt.Fprintf(w, `{"schema":%q,"version":"8.0.0","pid":%d}`, toolkit.HelperSchema, os.Getpid())
		case "/v1/base/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"schema": toolkit.HelperSchema, "state": probed})
		case "/v1/base/ensure":
			ensures++
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema": toolkit.HelperSchema, "state": refused,
				"error": unread + " Nothing was changed.",
			})
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
	return &ensures
}

// ⛔ THE DEFECT: `ready` over a base whose marker could not be read named `base
// ensure`, which refuses that base, and under `--ensure` that was the command
// that had just failed. It names the rebuild, from the probe's remediation and
// from the refusal's own words where the state carries none.
func TestReadyNamesTheRebuildOfABaseWhoseMarkerCannotBeRead(t *testing.T) {
	home := quietCLI(t)
	t.Setenv(toolkit.PullStallEnv, "")
	ensures := unreadMarkerHelper(t, home)
	cfg, err := toolkit.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	client, err := toolkit.DialHelper(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, ensure := range []bool{false, true} {
		r := ReadyReport{Route: readyRoute{Selected: "helper"}}
		_ = captureStderr(t, func() { r.fillBase(context.Background(), cfg, nil, client, ensure) })
		if len(r.Remediation) == 0 || r.Remediation[0] != toolkit.ToolCommand("base recreate") {
			t.Errorf("ensure=%v: ready names %q; want the rebuild first", ensure, r.Remediation)
		}
		for _, c := range r.Remediation {
			if c == toolkit.ToolCommand("base ensure") {
				t.Errorf("ensure=%v: ready names %q, the command this base refuses", ensure, r.Remediation)
			}
		}
	}
	if *ensures != 1 {
		t.Fatalf("the helper was asked to ensure %d time(s); want once, under --ensure", *ensures)
	}
}
