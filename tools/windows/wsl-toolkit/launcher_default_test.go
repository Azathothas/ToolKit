// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// ⛔ `muse.exe` REACHES THE DEFAULT BASE, and neither the environment nor a working
// tree's pointer moves it. It reached the instance `base` until WSL-167, which made
// the default base the agents' one and reserved that name.
func TestAnUnsuffixedLauncherReachesTheDefaultBase(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	t.Setenv(toolkit.InstanceEnv, "podbox")
	work := t.TempDir()
	if _, err := toolkit.WritePointer(work, "muse"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)
	previous, previousQuiet := toolkit.SelectedInstance, quiet
	t.Cleanup(func() { toolkit.SelectedInstance, quiet = previous, previousQuiet })
	toolkit.SelectedInstance = toolkit.Instance{}
	// ⚠ THE DEFAULT BASE'S NAME IS THIS HOST'S. Every state directory acts on the
	// host's registered base, so the name is read, not assumed.
	cfg, err := toolkit.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	want := cfg.Base.Name
	toolkit.SelectedInstance = toolkit.Instance{}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	code := runLauncher(context.Background(), "muse", toolkit.DefaultInstance, []string{"--version"})
	_ = w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	if code == exitOK || !strings.Contains(string(out), want+"'s configuration names no agent adapter") {
		t.Fatalf("muse.exe answered %d with %q; want the default base %s named", code, out, want)
	}
	if toolkit.SelectedInstance.Name != toolkit.DefaultInstance {
		t.Fatalf("muse.exe selected the instance %q", toolkit.SelectedInstance.Name)
	}
}
