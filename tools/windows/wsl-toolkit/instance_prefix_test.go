// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// prefixMainCase is a fresh state root with nothing selected.
func prefixMainCase(t *testing.T) string {
	t.Helper()
	previous := toolkit.SelectedInstance
	t.Cleanup(func() { toolkit.SelectedInstance = previous })
	root := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", root)
	t.Setenv(toolkit.InstanceEnv, "")
	t.Setenv(toolkit.InstanceLiteralEnv, "")
	t.Setenv(instanceRootEnv, "")
	t.Setenv(instanceHomeEnv, "")
	return root
}

// stderrOf runs f with stderr captured.
func stderrOf(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	f()
	_ = w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	return string(out)
}

// ⚠ A NEW PREFIXED INSTANCE WARNS ONCE, EVEN UNDER --quiet, AND NOT AGAIN.
// WSL-193.
func TestAPrefixedInstanceWarnsOnceWhenItIsMade(t *testing.T) {
	root := prefixMainCase(t)
	wasQuiet := quiet
	t.Cleanup(func() { quiet = wasQuiet })
	quiet = true
	said := stderrOf(t, func() {
		if code, err := applyInstance(context.Background(), "wsl-demo"); code != exitOK || err != nil {
			t.Errorf("answered %d, %v", code, err)
		}
	})
	if strings.Count(said, "⚠") != 1 || !strings.Contains(said, `"wsl-demo"`) || !strings.Contains(said, "wsl-toolkit-demo") {
		t.Fatalf("a new prefixed instance said %q; want one warning naming both", said)
	}
	if got := os.Getenv(toolkit.InstanceEnv); got != "demo" {
		t.Fatalf("the instance handed on is %q, want demo", got)
	}
	// The warning recorded the name in the instance it made, and the same name
	// reaches it quietly.
	if _, err := os.Stat(filepath.Join(root, "instances", "demo", "prefixed-from")); err != nil {
		t.Fatalf("the warning recorded nothing: %v", err)
	}
	t.Setenv("WSL_TOOLKIT_HOME", root)
	t.Setenv(toolkit.InstanceEnv, "")
	t.Setenv(instanceRootEnv, "")
	t.Setenv(instanceHomeEnv, "")
	said = stderrOf(t, func() {
		if code, err := applyInstance(context.Background(), "wsl-demo"); code != exitOK || err != nil {
			t.Errorf("again answered %d, %v", code, err)
		}
	})
	if strings.Contains(said, "⚠") {
		t.Fatalf("an existing instance was warned about again: %q", said)
	}
}

// ⭐ --literal-instance keeps the name as given.
func TestTheLiteralFlagKeepsTheNameAsGiven(t *testing.T) {
	prefixMainCase(t)
	wasQuiet := quiet
	t.Cleanup(func() { quiet = wasQuiet })
	said := stderrOf(t, func() {
		if code := run(context.Background(), []string{"--quiet", toolkit.LiteralInstanceFlag, "--instance", "wsl-demo2", "version"}); code != exitOK {
			t.Errorf("answered %d", code)
		}
	})
	if got := toolkit.SelectedInstance; got.Name != "wsl-demo2" || got.Distro != "wsl-toolkit-wsl-demo2" {
		t.Fatalf("--literal-instance resolved %+v", got)
	}
	if strings.Contains(said, "⚠") {
		t.Fatalf("a literal name was warned about: %q", said)
	}
}
