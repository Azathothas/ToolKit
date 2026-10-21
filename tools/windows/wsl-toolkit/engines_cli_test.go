// SPDX-License-Identifier: 0BSD

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-136: --memory and --cpus ---------------------------------------------

// TestAProjectsLimitsReachAJobAndTheCommandLineWins: jobs.memory and jobs.cpus
// apply where the command line names none, and a flag beats them.
func TestAProjectsLimitsReachAJobAndTheCommandLineWins(t *testing.T) {
	cfg := toolkit.DefaultConfig()
	cfg.Jobs.Memory, cfg.Jobs.CPUs = "512m", 2
	j := jobFlags{maxBytes: 1, maxEntries: 1}
	if err := j.applyConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := j.check(); err != nil {
		t.Fatal(err)
	}
	if j.resources.MemoryBytes != 512<<20 || j.resources.CPUs != 2 {
		t.Fatalf("the project's limits read as %+v", j.resources)
	}
	j = jobFlags{maxBytes: 1, maxEntries: 1, memory: "64m", cpus: "0.5"}
	if err := j.applyConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := j.check(); err != nil {
		t.Fatal(err)
	}
	if j.resources.MemoryBytes != 64<<20 || j.resources.CPUs != 0.5 {
		t.Fatalf("the command line's limits read as %+v, and they win", j.resources)
	}
	if err := (&jobFlags{maxBytes: 1, maxEntries: 1, memory: "huge"}).check(); err == nil {
		t.Fatal("an unreadable --memory was accepted")
	}
}

// TestTheHelperRouteRefusesALimit: the helper carries neither the limit nor
// the answer, so a job through it would run unlimited and say nothing.
func TestTheHelperRouteRefusesALimit(t *testing.T) {
	ctx := context.Background()
	for _, j := range []jobFlags{{memory: "64m"}, {cpus: "1"}} {
		if _, err := helperRunJob(ctx, nil, j, "alpine", "alpine", []byte("true"), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "do not cross the helper") {
			t.Errorf("a job the helper would run kept its limit: %v", err)
		}
		if _, err := helperRunMatrix(ctx, nil, j, []string{"alpine"}, 1, []byte("true"), nil, nil, nil, ""); err == nil || !strings.Contains(err.Error(), "do not cross the helper") {
			t.Errorf("a fleet the helper would run kept its limit: %v", err)
		}
	}
}

func TestAJobsAnswerSaysWhetherItsLimitsHeld(t *testing.T) {
	if limitsLine(nil) != "" {
		t.Error("a job with no limit has a limits line")
	}
	asked := toolkit.LimitsReport{MemoryBytes: 64 << 20, CPUs: 1.5}
	for verdict, want := range map[string]string{
		toolkit.LimitsEnforced:   "limits memory 64.0 MiB and 1.5 cpus: enforced",
		toolkit.LimitsUnenforced: "! limits memory 64.0 MiB and 1.5 cpus: NOT enforced",
		toolkit.LimitsUnknown:    "whether they were enforced is unknown",
	} {
		r := asked
		r.Enforced = verdict
		if got := limitsLine(&r); !strings.Contains(got, want) {
			t.Errorf("%s reads %q, want %q in it", verdict, got, want)
		}
	}
}

// -- WSL-130: base image load and base.runtime_dir ------------------------------

func TestBaseImageLoadReadsAFileFirstAndAReferenceOtherwise(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "image.tar")
	if err := os.WriteFile(archive, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p, kind, err := imageSource(archive); err != nil || kind != toolkit.ImageSourceFile || p != archive {
		t.Errorf("an archive on this machine read as %q %q %v", p, kind, err)
	}
	if _, kind, err := imageSource("docker.io/library/busybox:latest"); err != nil || kind != toolkit.ImageSourceHost {
		t.Errorf("a reference read as %q %v", kind, err)
	}
	for _, bad := range []string{dir, "busybox", filepath.Join(dir, "missing.tar")} {
		if _, _, err := imageSource(bad); err == nil {
			t.Errorf("%q was accepted as a source", bad)
		}
	}
	if code, err := cmdBaseImage(context.Background(), []string{"save", "x"}); code != exitCannot || err == nil {
		t.Errorf("a verb that is not load answered %d, %v", code, err)
	}
	if code, err := cmdBaseImage(context.Background(), []string{"load"}); code != exitCannot || err == nil {
		t.Errorf("load with nothing to load answered %d, %v", code, err)
	}
}

// TestBaseExecNamesTheEnginesRuntimeDirWhenAsked: with base.runtime_dir on,
// the account's command starts with the prologue every job's engine call has,
// and root's, which keeps its engine elsewhere, does not.
func TestBaseExecNamesTheEnginesRuntimeDirWhenAsked(t *testing.T) {
	cfg := toolkit.DefaultConfig()
	cfg.Base.RuntimeDir = true
	req, err := (baseExecFlags{command: "podman ps", dir: "~"}).request(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(req.Script, toolkit.RuntimeDirPrologue()) || !bytes.HasSuffix(req.Script, []byte("podman ps\n")) {
		t.Fatalf("the account's command does not start with the runtime prologue:\n%s", req.Script)
	}
	req, err = (baseExecFlags{command: "podman ps", dir: "~", asRoot: true}).request(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(req.Script, []byte("XDG_RUNTIME_DIR")) {
		t.Fatalf("root's command was given the account's runtime directory:\n%s", req.Script)
	}
	cfg.Base.RuntimeDir = false
	req, err = (baseExecFlags{command: "podman ps", dir: "~"}).request(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(req.Script, []byte("XDG_RUNTIME_DIR")) {
		t.Fatalf("a command with base.runtime_dir off was given a runtime directory:\n%s", req.Script)
	}
}

// -- WSL-126: what a stop reached -----------------------------------------------

func TestASessionsAnswerSaysWhatAStopReaches(t *testing.T) {
	rec := toolkit.SessionRecord{ID: "0123456789abcdef", Unit: "wtk-session-0123456789abcdef.scope"}
	if got := sessionContainmentLine(rec); !strings.Contains(got, rec.Unit) || !strings.Contains(got, "every process") {
		t.Errorf("a session in a scope says %q", got)
	}
	if got := sessionContainmentLine(toolkit.SessionRecord{ID: rec.ID}); !strings.Contains(got, "process group") || !strings.Contains(got, "setsid") {
		t.Errorf("a session with no scope says %q", got)
	}
	scoped := toolkit.StopReport{Kind: "session", ContainedBy: toolkit.ContainedByScope, Unit: rec.Unit}
	if got := containmentHint(scoped); !strings.Contains(got, rec.Unit) {
		t.Errorf("a stop through a scope says %q", got)
	}
	grouped := toolkit.StopReport{Kind: "session", ContainedBy: toolkit.ContainedByGroup}
	if got := containmentHint(grouped); !strings.Contains(got, "not reached") {
		t.Errorf("a stop through a group says %q", got)
	}
	if got := containmentHint(toolkit.StopReport{Kind: "job"}); got != "" {
		t.Errorf("a job's stop says %q", got)
	}
}
