// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// TestBaseDoorsIsRefusedThroughAServingHelper holds WSL-142's refusal on any
// host: a helper that serves, and a call that names it.
//
// ⛔ THE PROBE NEVER CROSSES THE HELPER. It mounts, writes and opens sockets in
// the base, which the restricted protocol does not accept, so the call asks the
// helper nothing past the status its dial reads, and writes no report. Before
// this, only a live acceptance run met the refusal.
//
// ⚠ THE CONFIGURATION NAMES A BASE NO HOST REGISTERS, so a call that went past
// the refusal is refused for another reason, and never probes a base on the
// machine that runs this case.
func TestBaseDoorsIsRefusedThroughAServingHelper(t *testing.T) {
	home := quietCLI(t)
	selectInstance(t, "doorsless")
	was := toolkit.ExplicitConfigPath
	t.Cleanup(func() { toolkit.ExplicitConfigPath = was })
	cfg := filepath.Join(t.TempDir(), "config.json")
	if err := forksafe.WriteFile(cfg, []byte(`{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit-doorsless"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked = append(asked, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/status" {
			_, _ = fmt.Fprintf(w, `{"schema":"x","version":"8.0.0","pid":%d}`, os.Getpid())
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"error":"this fake answers no %s"}`, r.URL.Path)
	}))
	defer srv.Close()
	writeHelperEndpoint(t, home, srv)

	var code int
	var out string
	said := captureStderr(t, func() {
		out = captureStdout(t, func() {
			code = run(context.Background(), []string{"--config", cfg, "base", "doors", "--via-helper", "--json"})
		})
	})
	if code != exitCannot {
		t.Errorf("base doors --via-helper answered %d, want %d: %s", code, exitCannot, said)
	}
	if !strings.Contains(said, "restricted helper protocol does not accept") || !strings.Contains(said, "WSL approval path") {
		t.Errorf("the refusal does not say why, or where to go instead: %q", said)
	}
	if out != "" {
		t.Errorf("a refused probe wrote a report: %q", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"GET /v1/status"}; !reflect.DeepEqual(asked, want) {
		t.Errorf("the helper was asked %v, and a refusal asks it %v", asked, want)
	}
}
