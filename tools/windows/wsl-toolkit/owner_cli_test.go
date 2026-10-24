// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-121 and WSL-136: what the owning process does beyond the job ----------

// parsedOwner binds and parses the owner flags as run and matrix bind them.
func parsedOwner(t *testing.T, args ...string) ownerFlags {
	t.Helper()
	var o ownerFlags
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	o.bind(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return o
}

// ⛔ Each owner flag the helper cannot honour is refused on that route, and a
// cap the project's file set is dropped there with a note, so the job runs.
func TestOwnerFlagsAreRefusedThroughTheHelper(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	ctx := context.Background()
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--cancel-on-exit"}, "--cancel-on-exit starts a watcher"},
		{[]string{"--max-running", "2"}, "--max-running is held by the process that owns the job"},
	} {
		code, err := cmdRun(ctx, append([]string{"--image", "alpine", "-c", "true", "--via-helper"}, c.args...))
		if code != exitCannot || err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("run %v --via-helper answered %d, %v; want %q", c.args, code, err, c.want)
		}
	}
	for _, args := range [][]string{{"--cancel-on-exit"}, {"--max-running", "2"}} {
		code, err := cmdMatrix(ctx, append([]string{"-c", "true", "--via-helper"}, args...))
		if code != exitCannot || err == nil || !strings.Contains(err.Error(), args[0]) {
			t.Errorf("matrix %v --via-helper answered %d, %v", args, code, err)
		}
	}
	capped := ownerFlags{maxRunning: 2, maxRunningWait: time.Minute, capFromConfig: true}
	if err := capped.helperRefusal(false); err != nil || capped.maxRunning != 0 || capped.maxRunningWait != 0 {
		t.Errorf("a project's max_running through the helper answered %v and left a cap of %d", err, capped.maxRunning)
	}
}

// ⛔ The owner flags refuse a value that cannot mean what it says, and the
// command line wins over the project's file, 0 included.
func TestOwnerFlagsRefuseWhatCannotMeanWhatItSays(t *testing.T) {
	cfg := toolkit.DefaultConfig()
	project := toolkit.DefaultConfig()
	project.Jobs.MaxRunning, project.Jobs.MaxRunningWait = 2, "10m"
	o := parsedOwner(t)
	if err := o.settle(project, toolkit.ContainerPersistent); err != nil {
		t.Fatal(err)
	}
	if o.maxRunning != 2 || o.maxRunningWait != 10*time.Minute || !o.capFromConfig {
		t.Errorf("the project's cap reads %d and %s", o.maxRunning, o.maxRunningWait)
	}
	if o.lifecycle != toolkit.ContainerPersistent {
		t.Errorf("the job's lifecycle reads %q", o.lifecycle)
	}
	o = parsedOwner(t, "--max-running", "0")
	if err := o.settle(project, toolkit.ContainerEphemeral); err != nil || o.maxRunning != 0 {
		t.Errorf("--max-running 0 left a cap of %d: %v", o.maxRunning, err)
	}

	for _, args := range [][]string{{"--max-running", "-1"}, {"--max-running-wait", "-1s", "--max-running", "1"}, {"--max-running-wait", "1m"}} {
		o := parsedOwner(t, args...)
		if err := o.settle(cfg, toolkit.ContainerEphemeral); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
}

// ⭐ A detached owner watches the chain its client handed it, never its own
// parent, which is that client.
func TestADetachedOwnerWatchesTheChainItWasHanded(t *testing.T) {
	handed := []toolkit.ProcessIdentity{
		{PID: 42, Start: 7, Name: "bash.exe", Role: toolkit.RoleCaller},
	}
	value, err := toolkit.EncodeWatched(handed)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(toolkit.CallersEnv, value)
	t.Setenv(toolkit.DetachedEnv, "1")
	got, err := callersOf()
	if err != nil || len(got) != 1 || got[0] != handed[0] {
		t.Fatalf("a detached owner watches %+v, %v; want the chain it was handed", got, err)
	}
	t.Setenv(toolkit.DetachedEnv, "")
	if got, err := callersOf(); err == nil && len(got) > 0 && got[0].PID == 42 {
		t.Errorf("an attached owner watches the handed chain %+v", got)
	}
}

// ⭐ A detached answer names what --cancel-on-exit watches, and says nothing of
// it otherwise.
func TestADetachedAnswerNamesWhatItWatches(t *testing.T) {
	home := t.TempDir()
	caller := toolkit.ProcessIdentity{PID: 42, Start: 7, Name: "bash.exe", Role: toolkit.RoleCaller}
	out := captureStdout(t, func() { _, _ = reportDetached(home, cliJobID, true, true, caller) })
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if list, ok := doc["cancel_on_exit"].([]any); !ok || len(list) != 1 {
		t.Errorf("the answer carries cancel_on_exit %v", doc["cancel_on_exit"])
	}
	out = captureStdout(t, func() { _, _ = reportDetached(home, cliJobID, true, true) })
	if strings.Contains(out, "cancel_on_exit") {
		t.Errorf("an answer without --cancel-on-exit names it: %s", out)
	}
}

// ⛔ The answer says exactly what an ended job leaves.
func TestAnAnswerSaysWhatTheJobLeaves(t *testing.T) {
	base := toolkit.JobResult{ID: cliJobID, ContainerLifecycle: toolkit.ContainerEphemeral, Transcript: `C:\state\jobs\` + cliJobID}
	if line := keptLine(base); !strings.Contains(line, "Its host record stays until") || !strings.Contains(line, "gc --job "+cliJobID) {
		t.Errorf("an ephemeral job's line is %q", line)
	}
	removed := base
	removed.RecordRemoved, removed.Transcript = true, ""
	if line := keptLine(removed); !strings.Contains(line, "left nothing") {
		t.Errorf("a removed record's line is %q", line)
	}
	kept := base
	kept.RecordKept = "the answer's copy of the output was cut"
	if line := keptLine(kept); !strings.Contains(line, "is kept: the answer's copy") {
		t.Errorf("a kept record's line is %q", line)
	}
	persistent := base
	persistent.ContainerLifecycle, persistent.Container = toolkit.ContainerPersistent, "wtk-"+cliJobID
	if line := keptLine(persistent); line != "" {
		t.Errorf("a persistent job, whose container line says what it keeps, has a second line %q", line)
	}
}

// ⭐ A job held by the cap and refused at once records that end and exits 2.
func TestAJobTheCapRefusesRecordsItsEnd(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", root)
	selectInstance(t, toolkit.DefaultInstance)
	busy, err := toolkit.ClaimJob(root, "aaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Release()
	if err := os.WriteFile(filepath.Join(root, "jobs", "aaaaaaaaaaaaaaaa", "admitted"), []byte("now\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	me, err := toolkit.ClaimJob(root, cliJobID)
	if err != nil {
		t.Fatal(err)
	}
	o := ownerFlags{maxRunning: 1}
	_, ended, code, err := o.admit(context.Background(), me, "alpine", "docker.io/library/alpine:latest", false)
	if !ended || code != exitCannot || err == nil || !strings.Contains(err.Error(), "refused at once") {
		t.Fatalf("a job over the cap answered %v, %d, %v", ended, code, err)
	}
	st, err := toolkit.ReadJobState(root, cliJobID)
	if err != nil || st.Owned || st.Result == nil || st.Result.Verdict() != exitCannot {
		t.Fatalf("the refused job reads as %+v, %v", st, err)
	}
	if _, ended, _, _ := (&ownerFlags{}).admit(context.Background(), nil, "", "", false); ended {
		t.Error("a job with no cap was held")
	}
}

// ⛔ An ended ephemeral job leaves nothing: its host record goes once its end
// is recorded, and the answer says so. A persistent job keeps its record, and a
// detached ephemeral one keeps it and says why, because wait reads it. WSL-121.
func TestAnEphemeralJobLeavesNothing(t *testing.T) {
	for _, c := range []struct {
		name      string
		lifecycle string
		detached  bool
		removed   bool
	}{
		{"attached ephemeral", toolkit.ContainerEphemeral, false, true},
		{"persistent", toolkit.ContainerPersistent, false, false},
		{"detached ephemeral", toolkit.ContainerEphemeral, true, false},
	} {
		home := t.TempDir()
		t.Setenv("WSL_TOOLKIT_HOME", home)
		detached := ""
		if c.detached {
			detached = "1"
		}
		t.Setenv(toolkit.DetachedEnv, detached)
		owner, err := toolkit.ClaimJob(home, cliJobID)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(home, "jobs", cliJobID)
		o := ownerFlags{lifecycle: c.lifecycle}
		res := toolkit.JobResult{ID: cliJobID, ContainerLifecycle: c.lifecycle, Transcript: dir}
		res.Seal()
		o.finish(owner, &res)
		_, statErr := os.Stat(dir)
		if gone := statErr != nil; gone != c.removed || res.RecordRemoved != c.removed {
			t.Errorf("%s: the record is gone %v and reads removed %v; want %v", c.name, gone, res.RecordRemoved, c.removed)
		}
		if c.removed && !strings.Contains(keptLine(res), "left nothing") {
			t.Errorf("%s: the answer says %q", c.name, keptLine(res))
		}
		if c.detached {
			st, err := toolkit.ReadJobState(home, cliJobID)
			if err != nil || st.Result == nil || st.Result.RecordKept != toolkit.DetachedRecordKept {
				t.Errorf("%s: the recorded end does not say why the record stays: %+v, %v", c.name, st.Result, err)
			}
		}
	}
}

// ⛔ A fleet is held by the cap a project sets: matrix reads jobs.max_running
// and holds each row, as run holds its job. WSL-136.
func TestAFleetIsHeldByTheProjectsCap(t *testing.T) {
	project := toolkit.DefaultConfig()
	project.Jobs.MaxRunning = 1
	o := parsedOwner(t)
	if err := o.settle(project, toolkit.ContainerEphemeral); err != nil {
		t.Fatal(err)
	}
	if o.rowAdmit() == nil {
		t.Fatal("a fleet under a project's cap holds no row")
	}
	root := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", root)
	selectInstance(t, toolkit.DefaultInstance)
	busy, err := toolkit.ClaimJob(root, "aaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Release()
	if err := os.WriteFile(filepath.Join(root, "jobs", "aaaaaaaaaaaaaaaa", "admitted"), []byte("now\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	row, err := toolkit.ClaimJob(root, cliJobID)
	if err != nil {
		t.Fatal(err)
	}
	defer row.Release()
	if _, err := o.rowAdmit()(context.Background(), row); !errors.Is(err, toolkit.ErrOverRunningCap) {
		t.Errorf("a row over the project's cap answered %v", err)
	}
	if (&ownerFlags{}).rowAdmit() != nil {
		t.Error("a fleet with no cap holds its rows")
	}
}

// ⭐ The answer says how long the cap held a job, and nothing where it did not.
func TestAnAnswerSaysHowLongTheCapHeldTheJob(t *testing.T) {
	res := toolkit.JobResult{ID: cliJobID, WaitedToRun: 90 * time.Second}
	if line := waitedLine(res); !strings.Contains(line, "waited 1m30s for a running slot") {
		t.Errorf("a held job's line is %q", line)
	}
	out := captureStdout(t, func() { _, _ = reportJob(res, true) })
	if !strings.Contains(out, `"waited_to_run_ns": 90000000000`) {
		t.Errorf("the answer does not carry the wait: %s", out)
	}
	if line := waitedLine(toolkit.JobResult{ID: cliJobID}); line != "" {
		t.Errorf("a job let in at once has the line %q", line)
	}
}

// ⭐ A record whose owner is lost is closed by the reader that finds it: the
// job list, `logs ID`, and `wait`.
func TestAReaderClosesARecordWhoseOwnerIsLost(t *testing.T) {
	for _, read := range []struct {
		name string
		run  func(home string)
	}{
		{"the job list", func(home string) { _ = captureStdout(t, func() { _, _ = listTranscripts(home, true) }) }},
		{"logs ID", func(home string) {
			_ = captureStdout(t, func() { _, _ = cmdLogs(context.Background(), []string{cliJobID}) })
		}},
		{"wait", func(string) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, _ = jobEnd(ctx, cliJobID, 0, nil, nil)
		}},
	} {
		home := t.TempDir()
		t.Setenv("WSL_TOOLKIT_HOME", home)
		led, err := toolkit.OpenLedger()
		if err != nil {
			t.Fatal(err)
		}
		if err := led.Append(toolkit.LedgerEntry{Event: "open", Kind: "job", ID: cliJobID}); err != nil {
			t.Fatal(err)
		}
		killed, err := toolkit.ClaimJob(home, cliJobID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "jobs", cliJobID, "stdout.log"), []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		killed.Release()
		read.run(home)
		open, err := led.Open()
		if err != nil || len(open) != 0 {
			t.Errorf("%s left the record open: %v, %v", read.name, open, err)
		}
	}
}
