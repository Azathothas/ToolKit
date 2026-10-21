// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-121 and WSL-136: what the owning process does beyond the job ----------

// parsedOwner binds and parses the owner flags as run binds them.
func parsedOwner(t *testing.T, run bool, args ...string) ownerFlags {
	t.Helper()
	var o ownerFlags
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	o.bind(fs, run)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return o
}

// ⛔ Each owner flag the helper cannot honour is refused on that route, and a
// value the project's file set names the flag that lifts it.
func TestOwnerFlagsAreRefusedThroughTheHelper(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	ctx := context.Background()
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--cancel-on-exit"}, "--cancel-on-exit starts a watcher"},
		{[]string{"--remove-record", "--container-lifecycle", "ephemeral"}, "--remove-record removes the record"},
		{[]string{"--max-running", "2"}, "--max-running is held by the process that owns the job"},
	} {
		code, err := cmdRun(ctx, append([]string{"--image", "alpine", "-c", "true", "--via-helper"}, c.args...))
		if code != exitCannot || err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("run %v --via-helper answered %d, %v; want %q", c.args, code, err, c.want)
		}
	}
	code, err := cmdMatrix(ctx, []string{"-c", "true", "--via-helper", "--cancel-on-exit"})
	if code != exitCannot || err == nil || !strings.Contains(err.Error(), "--cancel-on-exit starts a watcher") {
		t.Errorf("matrix --cancel-on-exit --via-helper answered %d, %v", code, err)
	}
	fromFile := ownerFlags{removeRecord: true, removeFromConfig: true}
	if err := fromFile.helperRefusal(false); err == nil || !strings.Contains(err.Error(), "--remove-record=false") {
		t.Errorf("a project's remove_record through the helper answered %v", err)
	}
	capped := ownerFlags{maxRunning: 2, capFromConfig: true}
	if err := capped.helperRefusal(false); err == nil || !strings.Contains(err.Error(), "--max-running 0") {
		t.Errorf("a project's max_running through the helper answered %v", err)
	}
}

// ⛔ The owner flags refuse a value that cannot mean what it says, and the
// command line wins over the project's file, false and 0 included.
func TestOwnerFlagsRefuseWhatCannotMeanWhatItSays(t *testing.T) {
	cfg := toolkit.DefaultConfig()
	o := parsedOwner(t, true, "--remove-record")
	if err := o.settle(cfg, toolkit.ContainerPersistent, false); err == nil {
		t.Error("--remove-record beside a persistent container was accepted")
	}
	o = parsedOwner(t, true, "--remove-record")
	if err := o.settle(cfg, toolkit.ContainerEphemeral, true); err == nil {
		t.Error("--remove-record beside --detach was accepted")
	}

	project := toolkit.DefaultConfig()
	project.Jobs.RemoveRecord, project.Jobs.MaxRunning, project.Jobs.MaxRunningWait = true, 2, "10m"
	o = parsedOwner(t, true)
	if err := o.settle(project, toolkit.ContainerPersistent, false); err != nil || o.removeRecord {
		t.Errorf("a project's remove_record on a persistent job answered %v and stays %v; it applies to ephemeral jobs", err, o.removeRecord)
	}
	if o.maxRunning != 2 || o.maxRunningWait != 10*time.Minute || !o.capFromConfig {
		t.Errorf("the project's cap reads %d and %s", o.maxRunning, o.maxRunningWait)
	}
	o = parsedOwner(t, true, "--max-running", "0", "--remove-record=false")
	if err := o.settle(project, toolkit.ContainerEphemeral, false); err != nil || o.maxRunning != 0 || o.removeRecord {
		t.Errorf("--max-running 0 and --remove-record=false left a cap of %d and removal %v: %v", o.maxRunning, o.removeRecord, err)
	}
	o = parsedOwner(t, true)
	if err := o.settle(project, toolkit.ContainerEphemeral, true); err != nil || o.removeRecord {
		t.Errorf("a project's remove_record on a detached job answered %v and stays %v", err, o.removeRecord)
	}

	for _, args := range [][]string{{"--max-running", "-1"}, {"--max-running-wait", "-1s", "--max-running", "1"}, {"--max-running-wait", "1m"}} {
		o := parsedOwner(t, true, args...)
		if err := o.settle(cfg, toolkit.ContainerEphemeral, false); err == nil {
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
	ended, code, err := o.admit(context.Background(), me, "alpine", "docker.io/library/alpine:latest", false)
	if !ended || code != exitCannot || err == nil || !strings.Contains(err.Error(), "refused at once") {
		t.Fatalf("a job over the cap answered %v, %d, %v", ended, code, err)
	}
	st, err := toolkit.ReadJobState(root, cliJobID)
	if err != nil || st.Owned || st.Result == nil || st.Result.Verdict() != exitCannot {
		t.Fatalf("the refused job reads as %+v, %v", st, err)
	}
	if ended, _, _ := (&ownerFlags{}).admit(context.Background(), nil, "", "", false); ended {
		t.Error("a job with no cap was held")
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
