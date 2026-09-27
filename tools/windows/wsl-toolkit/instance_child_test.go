// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// TestAChildResolvesItsParentsInstanceOnce plays the parent and then the child
// in one process. The child meets the resolved home in the two ways a child of
// this tool does: inherited, as `run --detach` passes it, and as `--home`, as
// `helper serve --detach` passes it.
func TestAChildResolvesItsParentsInstanceOnce(t *testing.T) {
	previous := toolkit.SelectedInstance
	t.Cleanup(func() { toolkit.SelectedInstance = previous })
	root := t.TempDir()
	want := filepath.Join(root, "instances", "zz")
	for _, how := range []string{"inherited", "as --home"} {
		t.Setenv("WSL_TOOLKIT_HOME", root)
		t.Setenv(toolkit.InstanceEnv, "")
		t.Setenv(instanceRootEnv, "")
		t.Setenv(instanceHomeEnv, "")
		if code, err := applyInstance(context.Background(), "zz"); code != exitOK || err != nil {
			t.Fatalf("%s: the parent answered %d, %v", how, code, err)
		}
		if got := os.Getenv("WSL_TOOLKIT_HOME"); !samePath(got, want) {
			t.Fatalf("%s: the parent resolved %s, want %s", how, got, want)
		}
		if how == "as --home" {
			os.Setenv("WSL_TOOLKIT_HOME", want)
		}
		// The child: the same environment, and no --instance flag.
		if code, err := applyInstance(context.Background(), ""); code != exitOK || err != nil {
			t.Fatalf("%s: the child answered %d, %v", how, code, err)
		}
		if got := os.Getenv("WSL_TOOLKIT_HOME"); !samePath(got, want) {
			t.Errorf("%s: the child resolved %s, want %s", how, got, want)
		}
	}
}

// TestAHomeNamedByTheCallerStillNests: a caller's own --home that happens to
// be an instance directory is a root like any other, because nothing this tool
// started put it there.
func TestAHomeNamedByTheCallerStillNests(t *testing.T) {
	previous := toolkit.SelectedInstance
	t.Cleanup(func() { toolkit.SelectedInstance = previous })
	root := filepath.Join(t.TempDir(), "instances", "zz")
	t.Setenv("WSL_TOOLKIT_HOME", root)
	t.Setenv(toolkit.InstanceEnv, "")
	t.Setenv(instanceRootEnv, "")
	t.Setenv(instanceHomeEnv, "")
	if code, err := applyInstance(context.Background(), "zz"); code != exitOK || err != nil {
		t.Fatalf("answered %d, %v", code, err)
	}
	if got, want := os.Getenv("WSL_TOOLKIT_HOME"), filepath.Join(root, "instances", "zz"); !samePath(got, want) {
		t.Fatalf("resolved %s, want %s", got, want)
	}
}
