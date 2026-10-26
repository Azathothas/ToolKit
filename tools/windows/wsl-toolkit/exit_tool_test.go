// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-127: the tool's own failure is told apart from a job's exit 2 -------

// quietCLI gives a case an empty state directory, the default instance and the
// default tool code, and puts each back afterwards.
//
// ⚠ --tool-exit SETS THE ENVIRONMENT, as --home does, so the variable is
// registered here and a case's flag never reaches the next case.
func quietCLI(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	t.Setenv(toolkit.InstanceEnv, "")
	t.Setenv(toolkit.ToolExitEnv, "")
	t.Cleanup(func() { toolkit.SetToolExit(toolkit.ExitTool) })
	selectInstance(t, toolkit.DefaultInstance)
	return home
}

// ⛔ EVERY COMMAND THAT ANSWERS WITH A JOB'S OWN CODE ANSWERS 125 FOR ITS OWN
// REFUSAL, AND EVERY OTHER COMMAND KEEPS 2. Each refusal below is made before
// anything reaches WSL.
func TestAJobCommandsOwnRefusalIsNotAJobsTwo(t *testing.T) {
	quietCLI(t)
	ctx := context.Background()
	forwards := [][]string{
		{"run", "--no-such-flag"},
		{"wait"},
		{"logs", cliJobID, "--follow", "--no-such-flag"},
		{"base", "exec", "--no-such-flag"},
		{"base", "shell", "--no-such-flag"},
		{"base", "agent"},
		{"distro", "new", "-c", "true", "--no-such-flag"},
		{"distro", "run", "--no-such-flag"},
		{"distro", "enter", "--no-such-flag"},
		{"bsd", "run", "--disk", "0"},
	}
	for _, args := range forwards {
		if code := run(ctx, args); code != toolkit.ExitTool {
			t.Errorf("%v answered %d, want %d: it answers a job's own code, so its own refusal must not read as one", args, code, toolkit.ExitTool)
		}
	}
	keeps := [][]string{
		{"logs", "--no-such-flag"},
		{"logs", cliJobID, "--follow=false", "--no-such-flag"},
		{"distro", "new", "--no-such-flag"},
		{"matrix", "--no-such-flag"},
		{"config", "--no-such-flag"},
		{"gc", "--no-such-flag"},
		{"base", "status", "--no-such-flag"},
	}
	for _, args := range keeps {
		if code := run(ctx, args); code != exitCannot {
			t.Errorf("%v answered %d, want 2: it never answers a job's code, so its refusal stays 2", args, code)
		}
	}
}

// ⭐ A LAUNCHER ANSWERS THE AGENT'S CODE, so its own failure answers the tool's:
// a directory no grant covers, and a tool code that is not one.
func TestALaunchersOwnFailureAnswersTheToolsCode(t *testing.T) {
	quietCLI(t)
	was := quiet
	t.Cleanup(func() { quiet = was })
	ctx := context.Background()
	if code := runLauncher(ctx, "muse", toolkit.DefaultInstance, []string{"exec", "PROMPT"}); code != toolkit.ExitTool {
		t.Errorf("a launcher refused before the agent ran answered %d", code)
	}
	t.Setenv(toolkit.ToolExitEnv, "1")
	if code := runLauncher(ctx, "muse", toolkit.DefaultInstance, nil); code != toolkit.ExitTool {
		t.Errorf("a launcher under a tool code that is not one answered %d", code)
	}
}

// ⛔ A JOB THAT EXITED 2 IS STILL ANSWERED 2. A forwarded code comes back with no
// error, and only a 2 that came with one is this tool's.
func TestAJobsOwnTwoIsForwardedUnchanged(t *testing.T) {
	home := quietCLI(t)
	endedJob(t, home, cliJobID, 2)
	if code := run(context.Background(), []string{"wait", cliJobID}); code != 2 {
		t.Fatalf("wait for a job that exited 2 answered %d", code)
	}
	if code := ownExit(true, exitCannot, nil); code != exitCannot {
		t.Errorf("a forwarded 2 with no error answered %d", code)
	}
	if code := ownExit(true, 130, errors.New("cancelled")); code != 130 {
		t.Errorf("a cancellation answered %d", code)
	}
	if code := ownExit(false, exitCannot, errors.New("refused")); code != exitCannot {
		t.Errorf("a refusal of a command that forwards nothing answered %d", code)
	}
	if code := ownExit(true, exitCannot, errors.New("refused")); code != toolkit.ExitTool {
		t.Errorf("a refusal of a command that forwards a code answered %d", code)
	}
}

// ⭐ --tool-exit AND WSL_TOOLKIT_TOOL_EXIT MOVE THE CODE, 2 IS THE WAY BACK TO
// 7.x, AND A CODE WITH ANOTHER MEANING IS REFUSED BEFORE ANYTHING RUNS.
func TestTheToolsOwnCodeIsConfigurable(t *testing.T) {
	quietCLI(t)
	ctx := context.Background()
	reset := func() { os.Setenv(toolkit.ToolExitEnv, "") }
	for _, c := range []struct {
		args []string
		want int
	}{
		{[]string{"--tool-exit", "3", "run", "--no-such-flag"}, 3},
		{[]string{"--tool-exit=2", "wait"}, 2},
		{[]string{"--tool-exit", "3", "config", "--no-such-flag"}, exitCannot},
		{[]string{"--tool-exit", "124", "run", "--no-such-flag"}, toolkit.ExitTool},
		{[]string{"--tool-exit", "1", "wait", cliJobID}, toolkit.ExitTool},
		{[]string{"--tool-exit", "300", "config"}, exitCannot},
		{[]string{"--tool-exit"}, exitCannot},
	} {
		reset()
		if code := run(ctx, c.args); code != c.want {
			t.Errorf("%v answered %d, want %d", c.args, code, c.want)
		}
	}
	reset()
	t.Setenv(toolkit.ToolExitEnv, "4")
	if code := run(ctx, []string{"run", "--no-such-flag"}); code != 4 {
		t.Errorf("%s=4 run answered %d", toolkit.ToolExitEnv, code)
	}
	t.Setenv(toolkit.ToolExitEnv, "130")
	if code := run(ctx, []string{"base", "exec", "--no-such-flag"}); code != toolkit.ExitTool {
		t.Errorf("%s=130 base exec answered %d, want a refusal at %d", toolkit.ToolExitEnv, code, toolkit.ExitTool)
	}
}

// ⛔ A REFUSED run --json ANSWERS A DOCUMENT THAT SAYS NO JOB STARTED. It wrote
// nothing, and a caller parsing the answer had nothing to parse.
func TestARefusedRunUnderJSONSaysNoJobStarted(t *testing.T) {
	quietCLI(t)
	ctx := context.Background()
	for _, args := range [][]string{
		{"run", "--json", "--no-such-flag"},
		{"base", "exec", "--json", "--no-such-flag"},
	} {
		var code int
		out := captureStdout(t, func() { code = run(ctx, args) })
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("%v answered no document: %q", args, out)
		}
		if doc["schema"] != RefusalSchema || doc["job_started"] != false || doc["effective_exit"] != float64(toolkit.ExitTool) ||
			!strings.Contains(doc["error"].(string), "no-such-flag") || code != toolkit.ExitTool {
			t.Errorf("%v answered %d and %v", args, code, doc)
		}
	}
	// ⚠ wait starts nothing, so it answers no document, and a command that
	// forwards nothing keeps its silence.
	for _, args := range [][]string{{"wait", "--json"}, {"matrix", "--json", "--no-such-flag"}} {
		if out := captureStdout(t, func() { run(ctx, args) }); out != "" {
			t.Errorf("%v answered a document: %q", args, out)
		}
	}
}

// ⚠ A REFUSAL IS NEVER A SECOND DOCUMENT after the command's own answer.
func TestARefusalIsNotASecondDocument(t *testing.T) {
	out := captureStdout(t, func() {
		answered = false
		_ = writeJSON(map[string]any{"schema": "x"})
		writeRefusal("run", toolkit.ExitTool, errors.New("after the answer"))
	})
	if strings.Count(out, `"schema"`) != 1 || strings.Contains(out, RefusalSchema) {
		t.Fatalf("a refusal after an answer was written as well: %q", out)
	}
}

// ⭐ A JOB THE CAP REFUSES ANSWERS ITS RESULT UNDER --json: not started, and the
// tool's code.
func TestAJobTheCapRefusesAnswersThatItNeverStarted(t *testing.T) {
	root := quietCLI(t)
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
	var code int
	out := captureStdout(t, func() {
		_, _, code, err = o.admit(context.Background(), me, "alpine", "docker.io/library/alpine:latest", true)
	})
	var doc map[string]any
	if jerr := json.Unmarshal([]byte(out), &doc); jerr != nil {
		t.Fatalf("a refused job under --json answered no result: %q", out)
	}
	if code != exitCannot || err == nil || doc["job_started"] != false || doc["unreached"] != true || doc["effective_exit"] != float64(toolkit.ExitTool) {
		t.Fatalf("a refused job answered %d, %v, %s", code, err, out)
	}
}

// ⭐ wait --json SAYS WHETHER THE JOB OR THE SESSION STARTED, in one place.
func TestAWaitSaysWhetherTheJobStarted(t *testing.T) {
	for _, c := range []struct {
		name string
		end  jobEndOutcome
		want bool
	}{
		{"a job that ran", jobEndOutcome{Result: &toolkit.JobResult{JobStarted: true}}, true},
		{"a job that never ran", jobEndOutcome{Result: &toolkit.JobResult{Unreached: true}}, false},
		{"a session", jobEndOutcome{Session: &toolkit.SessionState{}}, true},
	} {
		out := captureStdout(t, func() { _, _ = renderEnd(c.end, true) })
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("%s: %q", c.name, out)
		}
		if doc["job_started"] != c.want {
			t.Errorf("%s: job_started is %v, want %v", c.name, doc["job_started"], c.want)
		}
	}
}

// ⭐ AN ORPHAN'S CONTAINER THAT HAS AN EXIT RAN, and a 2 it exited with is its own.
func TestAnOrphanWithAnExitStarted(t *testing.T) {
	two := 2
	res, verdict := orphanEnd(cliJobID, &two, true, "")
	if !res.JobStarted || verdict != 2 || res.EffectiveExit != 2 {
		t.Errorf("an orphan whose container exited 2 answered %+v, %d", res, verdict)
	}
	if res, verdict := orphanEnd(cliJobID, &two, true, "wsl-toolkit stop"); !res.JobStarted || verdict != 130 || !res.Stopped {
		t.Errorf("a stopped orphan answered %+v, %d", res, verdict)
	}
	for _, present := range []bool{true, false} {
		if res, verdict := orphanEnd(cliJobID, nil, present, ""); res.JobStarted || verdict != exitFailed {
			t.Errorf("an orphan with no exit, present %v, answered %+v, %d", present, res, verdict)
		}
	}
}

// ⭐ A FLAG IS READ IN EVERY SPELLING GO'S FLAG PACKAGE TAKES, and stops at --.
func TestAFlagIsReadInEverySpelling(t *testing.T) {
	for _, c := range []struct {
		args []string
		want bool
	}{
		{[]string{"--follow"}, true},
		{[]string{"-follow"}, true},
		{[]string{"--follow=true"}, true},
		{[]string{"--follow=false"}, false},
		{[]string{"ID", "--tail", "5", "--follow"}, true},
		{[]string{"--", "--follow"}, false},
		{[]string{"--followed"}, false},
		{[]string{"echo --follow"}, false},
	} {
		if got := hasFlag(c.args, "follow"); got != c.want {
			t.Errorf("%v read --follow as %v", c.args, got)
		}
	}
}
