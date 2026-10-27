// SPDX-License-Identifier: 0BSD

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// ⛔ A MATRIX ROW SAYS ITS LIMITS UNDER IT, AND A ROW WITH NONE PRINTS ONE LINE,
// as before. WSL-136.
func TestAMatrixRowSaysWhetherItsLimitsHeld(t *testing.T) {
	row := toolkit.JobResult{Label: "alpine", Exit: 0}
	if got := rowLines(row); len(got) != 1 || !strings.Contains(got[0], "ok") || !strings.Contains(got[0], "alpine") {
		t.Fatalf("a row with no limit prints %q", got)
	}
	row.Limits = &toolkit.LimitsReport{MemoryBytes: 64 << 20, Enforced: toolkit.LimitsUnenforced}
	got := rowLines(row)
	if len(got) != 2 || !strings.Contains(got[1], "! limits memory 64.0 MiB: NOT enforced") {
		t.Fatalf("a row whose limit went unenforced prints %q", got)
	}
}

// -- WSL-130: base image load and base.runtime_dir ------------------------------

// ⛔ A FAILED LOAD UNDER --json STILL WRITES ITS DOCUMENT, with the reason in
// error, and keeps its exit code. WSL-130.
func TestAFailedImageLoadAnswersItsJSON(t *testing.T) {
	rep := toolkit.ImageLoad{Schema: toolkit.ImageLoadSchema, Source: "docker.io/library/alpine:3.20", Kind: toolkit.ImageSourceHost, Distro: "wsl-toolkit", User: "toolkit"}
	var code int
	var err error
	out := captureStdout(t, func() {
		code, err = imageLoadFailed(rep, true, exitFailed, errors.New("wslc save docker.io/library/alpine:3.20: exit status 1"))
	})
	var got toolkit.ImageLoad
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("a failed load under --json wrote %q: %v", out, jerr)
	}
	if code != exitFailed || err == nil || got.Schema != toolkit.ImageLoadSchema || !strings.Contains(got.Error, "wslc save") {
		t.Fatalf("a failed load answered %d, %v, %+v", code, err, got)
	}
	if out := captureStdout(t, func() { _, _ = imageLoadFailed(rep, false, exitFailed, errors.New("x")) }); out != "" {
		t.Fatalf("a failed load without --json wrote %q to stdout", out)
	}
}

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
	scoped := toolkit.StopReport{Kind: "session", ContainedBy: toolkit.ContainedByScope, Unit: rec.Unit, Stopped: true}
	if got := containmentHint(scoped); !strings.Contains(got, rec.Unit) {
		t.Errorf("a stop through a scope says %q", got)
	}
	grouped := toolkit.StopReport{Kind: "session", ContainedBy: toolkit.ContainedByGroup, Stopped: true}
	if got := containmentHint(grouped); !strings.Contains(got, "not reached") {
		t.Errorf("a stop through a group says %q", got)
	}
	if got := containmentHint(toolkit.StopReport{Kind: "job"}); got != "" {
		t.Errorf("a job's stop says %q", got)
	}
}

// ⛔ A STOP THAT SIGNALLED NOTHING DOES NOT SAY IT SIGNALLED SOMETHING, and a
// stop of an ended session keeps exit 0 and no after, as 7.0.0 answered, when
// what its scope held did not end. WSL-126.
func TestAStopOfAnEndedSessionSaysOnlyWhatItReached(t *testing.T) {
	unit := "wtk-session-0123456789abcdef.scope"
	for _, before := range []string{"gone", "ended"} {
		rep := toolkit.StopReport{Kind: "session", ContainedBy: toolkit.ContainedByScope, Unit: unit, Before: before}
		if got := containmentHint(rep); got != "" {
			t.Errorf("a stop of a %s session that signalled nothing says %q", before, got)
		}
		rep.ContainedBy = toolkit.ContainedByGroup
		if got := containmentHint(rep); got != "" {
			t.Errorf("a stop of a %s session with no scope that signalled nothing says %q", before, got)
		}
	}
	rep := toolkit.StopReport{Kind: "session", ContainedBy: toolkit.ContainedByScope, Unit: unit, Before: "ended"}
	applyLeftovers(&rep, true, true, nil)
	if rep.Leftovers != "running" || rep.After != "" || !strings.Contains(rep.Note, "did not end") {
		t.Fatalf("leftovers that did not end answered %+v", rep)
	}
	if code, err := renderStop(rep, true); err != nil || code != exitOK {
		t.Fatalf("a stop of an ended session exited %d, %v; it exits 0", code, err)
	}
	if got := containmentHint(rep); !strings.Contains(got, unit) {
		t.Errorf("a stop that reached a scope's leftovers says %q", got)
	}
	quiet := toolkit.StopReport{Kind: "session", ContainedBy: toolkit.ContainedByScope, Unit: unit, Before: "ended", Note: "x"}
	applyLeftovers(&quiet, false, false, nil)
	if quiet.Leftovers != "" || quiet.Note != "x" {
		t.Fatalf("a scope that held nothing changed the answer: %+v", quiet)
	}
}

// -- WSL-130, WSL-136: a MINOR release adds no line to an unchanged base -------

// TestAnUnchangedBasesStatusPrintsNoNewRow: the engines and kvm rows print only
// where a setting asks for them, and the cgroup manager is in --json alone. A
// base none of the three is set for gets the rows 7.0.x printed.
func TestAnUnchangedBasesStatusPrintsNoNewRow(t *testing.T) {
	st := toolkit.BaseState{Name: "wsl-toolkit", Kvm: "absent",
		Cgroup: &toolkit.CgroupState{Version: "v2", Mechanism: "none", Manager: "cgroupfs", AccountControllers: "cpu memory pids"}}
	out := captureStderr(t, func() { renderBaseState(st, true) })
	for _, row := range []string{"  engines ", "  kvm ", "cgroup mgr", "cgroupfs"} {
		if strings.Contains(out, row) {
			t.Errorf("a base with no engine setting printed %q:\n%s", row, out)
		}
	}
	st.Access.Kvm = true
	out = captureStderr(t, func() { renderBaseState(st, true) })
	for _, row := range []string{"  engines     root engine false, runtime dir false, kvm true", "  kvm         /dev/kvm absent to the account"} {
		if !strings.Contains(out, row) {
			t.Errorf("a base with base.kvm on did not print %q:\n%s", row, out)
		}
	}
}

// TestAPrivateHerdrSaysWhatItGivesUpInItsStatus: herdr's private_net prints one row,
// probed or not, and it carries the cost beside the gain. A base without the key
// prints what it printed before. WSL-159, ruling 36.
//
// ⛔ THE ROW SAYS WHAT WAS ASKED, AND FOR THE PANES ALONE. It read the configuration and
// said the agents reach no Windows host, while a server not yet restarted, base agent
// and interop all reach it.
func TestAPrivateHerdrSaysWhatItGivesUpInItsStatus(t *testing.T) {
	st := toolkit.BaseState{Name: "wsl-toolkit"}
	for _, probed := range []bool{false, true} {
		if out := captureStderr(t, func() { renderBaseState(st, probed) }); strings.Contains(out, "herdr net") {
			t.Errorf("a base without private_net printed a herdr net row:\n%s", out)
		}
	}
	st.Access.HerdrPrivateNet = true
	for _, probed := range []bool{false, true} {
		out := captureStderr(t, func() { renderBaseState(st, probed) })
		if !strings.Contains(out, "  herdr net   private_net asked: ") || !strings.Contains(out, "give up rootless containers and sudo") {
			t.Errorf("a base with private_net, probed %v, did not say what it gives up:\n%s", probed, out)
		}
		if !strings.Contains(out, "processes in herdr's panes") || !strings.Contains(out, "base agent and interop stay outside it") {
			t.Errorf("a base with private_net, probed %v, claimed more than the panes:\n%s", probed, out)
		}
	}
}
