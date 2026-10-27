// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// writeBaseRecord writes this machine's record of the base: its name and the
// image it was built from.
func writeBaseRecord(t *testing.T, home, name, image string) {
	t.Helper()
	rec := `{"schema":"wsl-toolkit-base/1","name":"` + name + `","image":"` + image + `","user":"agent"}`
	if err := forksafe.WriteFile(filepath.Join(home, "base.json"), []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ⛔ WSL-169, widened by the audit. `--preset` was compared with the configured
// image, which differs from the built one after a `--preset` without `--save` or
// a saved switch whose rebuild was refused. A preset the base already had then
// rebuilt it, and the one it lacked did not.
func TestAPresetIsComparedWithTheImageTheBaseWasBuiltFrom(t *testing.T) {
	alpine, err := toolkit.ResolveBasePreset("alpine")
	if err != nil {
		t.Fatal(err)
	}
	debian, err := toolkit.ResolveBasePreset("debian")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, configured, recorded string
		want                       bool
	}{
		{"built from the preset, configured otherwise", debian, alpine, false},
		{"configured as the preset, built otherwise", alpine, debian, true},
		// ⚠ NO RECORD, NO CHANGE: the configuration answers, as it always did.
		{"no record, configured as the preset", alpine, "", false},
		{"no record, configured otherwise", debian, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("WSL_TOOLKIT_HOME", home)
			cfg := toolkit.DefaultConfig()
			cfg.Base.Name = "wsl-toolkit-wsl169"
			cfg.Base.Image = tc.configured
			if tc.recorded != "" {
				writeBaseRecord(t, home, cfg.Base.Name, tc.recorded)
			}
			switched, err := applyPreset(context.Background(), &cfg, "alpine", false)
			if err != nil {
				t.Fatal(err)
			}
			if switched != tc.want || cfg.Base.Image != alpine {
				t.Fatalf("switched %v to %s; want %v to %s", switched, cfg.Base.Image, tc.want, alpine)
			}
		})
	}
	// ⚠ A record for another name is not this base's.
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	cfg := toolkit.DefaultConfig()
	cfg.Base.Name = "wsl-toolkit-wsl169"
	cfg.Base.Image = alpine
	writeBaseRecord(t, home, "wsl-toolkit-other", debian)
	if switched, err := applyPreset(context.Background(), &cfg, "alpine", false); err != nil || switched {
		t.Fatalf("another base's record decided the switch: %v, %v", switched, err)
	}
}

// ⛔ WSL-172. `base grant` read the configuration, changed it and wrote it back
// with nothing held, so two grants at once lost one. It waits for the base's
// lock now, and reads the configuration only once it holds it.
func TestAGrantWaitsForTheBasesLock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	// ⚠ An instance no real base carries, so a grant that ran anyway would change
	// only the configuration.
	previous := toolkit.SelectedInstance
	t.Cleanup(func() { toolkit.SelectedInstance = previous })
	toolkit.SelectedInstance = toolkit.Instance{Name: "wsl172", Distro: "wsl-toolkit-wsl172", Home: home}
	cfg := toolkit.DefaultConfig()
	cfg.Base.Name = "wsl-toolkit-wsl172"
	if err := cfg.Write(); err != nil {
		t.Fatal(err)
	}
	release, err := toolkit.LockBase(context.Background(), "base ensure", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err = cmdBaseGrant(ctx, "grant", []string{"--source", t.TempDir()})
	holder := "process " + strconv.Itoa(os.Getpid()) + " running `base ensure`"
	if err == nil || !strings.Contains(err.Error(), holder) {
		t.Fatalf("a grant beside a held lock answered %v; want the wait naming %s", err, holder)
	}
	after, err := toolkit.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Base.Mounts) != 0 {
		t.Fatalf("the grant was written while another process held the base: %+v", after.Base.Mounts)
	}
}
