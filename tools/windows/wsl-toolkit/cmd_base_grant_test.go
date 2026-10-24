// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// grantConfigFile writes a configuration that accepts grants and selects it.
func grantConfigFile(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", root)
	t.Setenv(toolkit.InstanceRootEnv, root)
	prevInstance, prevConfig := toolkit.SelectedInstance, toolkit.ExplicitConfigPath
	t.Cleanup(func() { toolkit.SelectedInstance, toolkit.ExplicitConfigPath = prevInstance, prevConfig })
	toolkit.SelectedInstance = toolkit.Instance{Name: "grants"}
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit-grants","automount":"off","interop":"off"}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	toolkit.ExplicitConfigPath = path
	return path
}

// TestAGrantLoadsItsConfigurationUnderTheLock holds where `base grant` and `base
// revoke` read their file. ⛔ Read before any lock, two at once each wrote back the
// file without the other's grant. WSL-179.
func TestAGrantLoadsItsConfigurationUnderTheLock(t *testing.T) {
	grantConfigFile(t)
	held, err := toolkit.LockGrants(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, release, err := lockedGrantConfig(ctx); err == nil {
		release()
		t.Fatal("a grant loaded its configuration while another grant held the lock")
	}
	held()
	_, release, err := lockedGrantConfig(context.Background())
	if err != nil {
		t.Fatalf("a grant could not load its configuration with the lock free: %v", err)
	}
	release()
}

// grantRecorder stands in for a registered base and records the take-back.
type grantRecorder struct {
	changed  bool
	backs    int
	target   string
	asked    bool
	prevPath string
}

func (r *grantRecorder) ChangeGrantsLive(context.Context, toolkit.Config, toolkit.BaseMount) (bool, error) {
	return r.changed, nil
}

func (r *grantRecorder) TakeGrantsBack(_ context.Context, prev toolkit.Config, grant toolkit.BaseMount, changed bool) error {
	r.backs++
	r.target, r.asked, r.prevPath = grant.Target, changed, prev.Path()
	return nil
}

// TestAFileThatCannotBeWrittenTakesBackWhatTheStepDid drives the command's take-back
// with a configuration path that cannot be written. ⛔ A step that mounted nothing
// is taken back by its block alone, with the grant's own target: a live take-back
// unmounted another configuration's checkout. WSL-179.
func TestAFileThatCannotBeWrittenTakesBackWhatTheStepDid(t *testing.T) {
	path := grantConfigFile(t)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	change, err := toolkit.PlanGrant(cfg, t.TempDir(), "/workspaces/shared", "rw")
	if err != nil {
		t.Fatal(err)
	}
	// ⚠ A directory with something in it where the file goes: the rename that
	// writes the file fails on every host.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path, "busy"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []bool{false, true} {
		r := &grantRecorder{changed: changed}
		code, err := changeGrantLive(context.Background(), r, cfg, change)
		if code != exitCannot || err == nil || !strings.Contains(err.Error(), "taken back") {
			t.Fatalf("an unwritable file answered %d, %v", code, err)
		}
		if r.backs != 1 || r.target != "/workspaces/shared" || r.asked != changed || r.prevPath != cfg.Path() {
			t.Fatalf("a step that changed=%v was taken back %d time(s), target %q, changed=%v, from %q", changed, r.backs, r.target, r.asked, r.prevPath)
		}
	}
}
