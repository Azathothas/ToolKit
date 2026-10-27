// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-200: gc with no base cleans this machine's half -----------------------

// ⛔ `gc` WITH NO BASE PLANS THIS MACHINE'S HALF, AND `gc --apply` REMOVES IT. It
// said there was nothing inside the base to clean up and exited 0, so the
// incoming directories `resources` listed stayed. WSL-200.
//
// ⚠ THE DIRECT ROUTE NEEDS wsl.exe to build its runner, so it skips where there
// is none. It only lists the distributions, and the instance is one nothing
// registers. The toolkit's cases hold the same cleanup on any host.
func TestGCWithNoBaseRemovesThisMachinesHalf(t *testing.T) {
	if _, err := toolkit.FindWsl(); err != nil {
		t.Skipf("the direct route builds a runner, and %v", err)
	}
	home := filepath.Join(t.TempDir(), "instances", "wsl200gc")
	t.Setenv("WSL_TOOLKIT_HOME", home)
	previous := toolkit.SelectedInstance
	t.Cleanup(func() { toolkit.SelectedInstance = previous })
	toolkit.SelectedInstance = toolkit.Instance{Name: "wsl200gc", Distro: "wsl-toolkit-wsl200gc", Home: home}
	incoming := filepath.Join(home, "incoming", "0123456789abcdef")
	if err := os.MkdirAll(incoming, 0o700); err != nil {
		t.Fatal(err)
	}
	ledger, err := toolkit.OpenLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Append(toolkit.LedgerEntry{Event: "open", Kind: "incoming", ID: "0123456789abcdef", HostDir: incoming,
		At: time.Now().Add(-48 * time.Hour).UTC()}); err != nil {
		t.Fatal(err)
	}
	gc := func(args ...string) toolkit.CleanupPlan {
		t.Helper()
		var plan toolkit.CleanupPlan
		out := captureStdout(t, func() {
			if code, err := cmdGC(context.Background(), append(args, "--json")); code != exitOK || err != nil {
				t.Errorf("gc %v answered %d, %v", args, code, err)
			}
		})
		if err := json.Unmarshal([]byte(out), &plan); err != nil {
			t.Fatalf("gc %v --json is not a plan: %v\n%s", args, err, out)
		}
		return plan
	}
	plan := gc()
	if !plan.DryRun || len(plan.HostDirs) != 1 || plan.HostDirs[0] != incoming {
		t.Fatalf("gc with no base planned %+v; want the incoming directory", plan)
	}
	if _, err := os.Stat(incoming); err != nil {
		t.Fatalf("the dry run removed the incoming directory: %v", err)
	}
	plan = gc("--apply")
	if plan.DryRun || len(plan.Entries) != 1 || plan.Entries[0].Result != toolkit.CleanupRemoved {
		t.Fatalf("gc --apply with no base answered %+v", plan)
	}
	if _, err := os.Stat(incoming); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("gc --apply with no base left the incoming directory: %v", err)
	}
	if open, err := ledger.Open(); err != nil || len(open) != 0 {
		t.Errorf("gc --apply with no base left %d record(s) open, %v", len(open), err)
	}
}
