// SPDX-License-Identifier: 0BSD

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-196: base.registry in config and base status ----------------------------

// registryInstance selects a scratch instance whose own file is body, standing in
// a directory with no wsl-toolkit.json.
func registryInstance(t *testing.T, body string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "instances", "r196")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := forksafe.WriteFile(filepath.Join(home, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WSL_TOOLKIT_HOME", home)
	previous := toolkit.SelectedInstance
	t.Cleanup(func() { toolkit.SelectedInstance = previous })
	toolkit.SelectedInstance = toolkit.Instance{Name: "r196", Distro: "wsl-toolkit-r196", Home: home}
	was, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(was) })
}

// ⭐ config REPORTS base.registry WHERE IT IS SET, and a configuration without it
// prints and writes what it did before: no row and no field. WSL-196.
func TestConfigReportsTheRegistryKeyOnlyWhereItIsSet(t *testing.T) {
	registryInstance(t, `{"schema":"wsl-toolkit-config/1","base":{"registry":{"mirrors":{"docker.io":["mirror.gcr.io"]},"proxy":"http://proxy.example:3128","no_proxy":["localhost"]}}}`)
	out := captureStderr(t, func() {
		if code, err := cmdConfig(nil); code != exitOK || err != nil {
			t.Errorf("config answered %d, %v", code, err)
		}
	})
	for _, row := range []string{
		"  mirror      docker.io through mirror.gcr.io, then docker.io itself\n",
		"  proxy       http://proxy.example:3128, not for localhost\n",
	} {
		if !strings.Contains(out, row) {
			t.Errorf("config does not print %q:\n%s", row, out)
		}
	}
	doc := struct {
		Base toolkit.BaseConfig `json:"base"`
	}{}
	raw := captureStdout(t, func() {
		if code, err := cmdConfig([]string{"--json"}); code != exitOK || err != nil {
			t.Errorf("config --json answered %d, %v", code, err)
		}
	})
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("config --json is not JSON: %v\n%s", err, raw)
	}
	if r := doc.Base.Registry; r == nil || r.Proxy != "http://proxy.example:3128" || len(r.Mirrors["docker.io"]) != 1 {
		t.Errorf("config --json carries base.registry as %+v", r)
	}

	registryInstance(t, `{"schema":"wsl-toolkit-config/1"}`)
	out = captureStderr(t, func() { _, _ = cmdConfig(nil) })
	raw = captureStdout(t, func() { _, _ = cmdConfig([]string{"--json"}) })
	if strings.Contains(out, "  mirror ") || strings.Contains(out, "  proxy ") || strings.Contains(raw, `"registry"`) {
		t.Errorf("a configuration without base.registry reports it:\n%s\n%s", out, raw)
	}
}

// ⭐ base status PRINTS base.registry WHERE IT IS SET, one row for each mirrored
// registry and one for the proxy, and an unchanged base prints no new row.
// WSL-196.
func TestBaseStatusPrintsTheRegistryRowsOnlyWhereSet(t *testing.T) {
	st := toolkit.BaseState{Name: "wsl-toolkit-r196"}
	out := captureStderr(t, func() { renderBaseState(st, false) })
	if strings.Contains(out, "  mirror ") || strings.Contains(out, "  proxy ") {
		t.Errorf("a base without base.registry printed a row for it:\n%s", out)
	}
	st.Access.Registry = &toolkit.RegistryConfig{
		Mirrors: map[string][]string{"quay.io": {"q.example"}, "docker.io": {"mirror.gcr.io", "m2.example:5000"}},
		Proxy:   "socks5://127.0.0.1:1080",
	}
	out = captureStderr(t, func() { renderBaseState(st, false) })
	for _, row := range []string{
		"  mirror      docker.io through mirror.gcr.io, m2.example:5000, then docker.io itself\n  mirror      quay.io through q.example, then quay.io itself\n",
		"  proxy       socks5://127.0.0.1:1080\n",
	} {
		if !strings.Contains(out, row) {
			t.Errorf("base status does not print %q:\n%s", row, out)
		}
	}
}
