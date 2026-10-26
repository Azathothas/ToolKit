// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// -- WSL-127: the tool's own failure is told apart from a job's exit 2 -------

const (
	// execFakeNoMarkerEnv makes the fake's container write no marker, as an
	// image that never ran writes none.
	execFakeNoMarkerEnv = "WTK_EXEC_FAKE_WSL_NO_MARKER"
	// execFakeLingerEnv makes the fake's container leave a child holding its
	// stdout, so wsl.exe ends and its streams do not.
	execFakeLingerEnv = "WTK_EXEC_FAKE_WSL_LINGER"
	// execFakeLingerChildEnv makes this binary that child.
	execFakeLingerChildEnv = "WTK_EXEC_FAKE_WSL_LINGER_CHILD"
)

func init() {
	if os.Getenv(execFakeLingerChildEnv) != "" {
		os.Exit(execFakeLingerChild())
	}
}

// execFakeLinger starts the child that outlives the fake with its stdout.
func execFakeLinger() {
	self, err := os.Executable()
	if err != nil {
		return
	}
	c := exec.Command(self)
	c.Env = append(os.Environ(), execFakeLingerChildEnv+"=1")
	c.Stdout = os.Stdout
	_ = c.Start()
}

// execFakeLingerChild writes to its stdout until a write fails, which is when
// the reader let the pipe go. ⚠ Bounded, so a reader that never lets go leaves
// no process behind for long.
func execFakeLingerChild() int {
	for end := time.Now().Add(time.Minute); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if _, err := os.Stdout.Write([]byte(".")); err != nil {
			return 0
		}
	}
	return 0
}

// useToolExit puts code in force for one case.
func useToolExit(t *testing.T, code int) {
	t.Helper()
	SetToolExit(code)
	t.Cleanup(func() { SetToolExit(ExitTool) })
}

// ⛔ A JOB THIS TOOL COULD NOT RUN ANSWERS THE TOOL'S CODE, AND A PAYLOAD'S 2
// STAYS 2. They were one number, so a caller without --json could not tell a
// refusal from a job that ran and said 2.
func TestAJobThisToolCouldNotRunIsNotAJobThatExitedTwo(t *testing.T) {
	cases := []struct {
		name string
		res  JobResult
		want int
	}{
		{"an image that never ran", JobResult{Exit: 2, Unreached: true}, ExitTool},
		{"a job whose end was lost", JobResult{Exit: 2, JobStarted: true, ToolFailed: true}, ExitTool},
		{"a payload that exited 2", JobResult{Exit: 2, JobStarted: true}, 2},
		{"a payload that exited 125", JobResult{Exit: 125, JobStarted: true}, 125},
	}
	for _, c := range cases {
		if got := c.res.Verdict(); got != c.want {
			t.Errorf("%s answered %d, want %d", c.name, got, c.want)
		}
	}
	// ⭐ THE CODE MOVES WITH THE SETTING, and a payload's own code does not.
	useToolExit(t, 77)
	if got := (JobResult{Exit: 2, Unreached: true}).Verdict(); got != 77 {
		t.Errorf("an unreached job under a tool code of 77 answered %d", got)
	}
	if got := (JobResult{Exit: 2, JobStarted: true, ToolFailed: true}).Verdict(); got != 77 {
		t.Errorf("a lost end under a tool code of 77 answered %d", got)
	}
	if got := (JobResult{Exit: 2, JobStarted: true}).Verdict(); got != 2 {
		t.Errorf("a payload's 2 under a tool code of 77 answered %d", got)
	}
}

// ⛔ A CODE THIS TOOL ALREADY ANSWERS WITH ANOTHER MEANING IS REFUSED, and 2,
// which 7.x answered, is the way back.
func TestToolExitTakesOnlyACodeWithNoOtherMeaning(t *testing.T) {
	for v, want := range map[string]int{"125": 125, "2": 2, " 9 ": 9, "255": 255, "126": 126} {
		got, err := ParseToolExit(v)
		if err != nil || got != want {
			t.Errorf("%q read as %d, %v; want %d", v, got, err, want)
		}
	}
	for _, v := range []string{"", "x", "0", "1", "124", "130", "256", "-3"} {
		if got, err := ParseToolExit(v); err == nil {
			t.Errorf("%q was taken as %d", v, got)
		}
	}
	if _, err := ParseToolExit("124"); err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Errorf("124 was refused without saying it is the deadline's: %v", err)
	}
	if _, err := ParseToolExit("130"); err == nil || !strings.Contains(err.Error(), "stop") {
		t.Errorf("130 was refused without saying it is a stop's: %v", err)
	}
}

// ⭐ AN EXIT STATUS IS THE PAYLOAD'S ANSWER. Only an error that carries none
// is an end this tool lost.
func TestOnlyAnErrorWithNoExitStatusIsALostEnd(t *testing.T) {
	newExecFakeGuest(t)
	t.Setenv(execFakeExitEnv, "3")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	exited := runCommand(ctx, newCommand(ctx, exe, "-d", "x", "-u", "y", "--", "/bin/sh"), strings.NewReader(""), io.Discard, io.Discard)
	if ExitCode(exited) != 3 {
		t.Fatalf("the fake did not exit 3: %v", exited)
	}
	if lostEnd(exited) {
		t.Errorf("a payload's exit 3 was read as a lost end: %v", exited)
	}
	if lostEnd(nil) {
		t.Error("no error was read as a lost end")
	}
	for _, err := range []error{
		&ProcessError{Code: 2, Op: "process", Err: exec.ErrWaitDelay},
		errors.New("the stream could not be read"),
	} {
		if !lostEnd(err) {
			t.Errorf("%v was not read as a lost end", err)
		}
	}
}

// ⛔ A HELPER OF THE PROTOCOL BEFORE THIS ONE IS REFUSED. Its results carry
// neither job_started nor tool_failed, so a job it ran would read as one that
// never started.
func TestAHelperOfTheEarlierProtocolIsRefused(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	b, err := json.Marshal(HelperEndpoint{Schema: "wsl-toolkit-helper/6", Address: "127.0.0.1:1", Token: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "helper.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadHelperEndpoint(); err == nil || !strings.Contains(err.Error(), "wsl-toolkit-helper/6") {
		t.Fatalf("a helper of protocol 6 was read: %v", err)
	}
}

// ⛔ job_started IS NEVER OMITTED, so a caller reads one field on every answer.
func TestJobStartedIsOnEveryResult(t *testing.T) {
	b, err := json.Marshal(JobResult{Unreached: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"job_started":false`) {
		t.Fatalf("a job that never started omits job_started: %s", b)
	}
}

// TestAJobSaysWhetherItStartedAndWhoseCodeItAnswers drives Runner.Run, the job
// path `run` takes, through the fake wsl.exe: a payload that ran, one that
// exited 2 itself, an image that never ran, and a payload whose end was lost.
func TestAJobSaysWhetherItStartedAndWhoseCodeItAnswers(t *testing.T) {
	g := newExecFakeGuest(t)
	t.Setenv(execFakeNoMarkerEnv, "")
	t.Setenv(execFakeLingerEnv, "")
	home, err := EnsureHome()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: DefaultConfig(), wsl: g.wsl, home: home, ledger: mustLedger(t), log: func(string) {}}
	job := func() JobResult {
		res := r.Run(context.Background(), JobSpec{Image: "docker.io/library/alpine:latest", Script: []byte("true\n"),
			ContainerLifecycle: ContainerEphemeral, Platform: "linux/amd64", Limits: DefaultWorkspaceLimits()})
		res.Seal()
		return res
	}

	ran := job()
	if !ran.JobStarted || ran.Unreached || ran.ToolFailed || ran.EffectiveExit != 0 {
		t.Fatalf("a payload that ran answered %+v", ran)
	}

	t.Setenv(execFakeExitEnv, "2")
	two := job()
	if !two.JobStarted || two.Unreached || two.ToolFailed || two.EffectiveExit != 2 {
		t.Fatalf("a payload that exited 2 answered started %v, unreached %v, tool_failed %v, exit %d: %s",
			two.JobStarted, two.Unreached, two.ToolFailed, two.EffectiveExit, two.Error)
	}

	t.Setenv(execFakeExitEnv, "125")
	t.Setenv(execFakeNoMarkerEnv, "1")
	never := job()
	if never.JobStarted || !never.Unreached || never.EffectiveExit != ExitTool {
		t.Fatalf("an image that never ran answered started %v, unreached %v, exit %d: %s",
			never.JobStarted, never.Unreached, never.EffectiveExit, never.Error)
	}

	// ⛔ wsl.exe ended with 0 and a child kept its stdout open, so Go answered
	// no exit status at all, and the 2 the process layer put in its place read
	// as the payload's.
	t.Setenv(execFakeExitEnv, "0")
	t.Setenv(execFakeNoMarkerEnv, "")
	t.Setenv(execFakeLingerEnv, "1")
	lost := job()
	if !lost.JobStarted || !lost.ToolFailed || lost.Unreached || lost.EffectiveExit != ExitTool {
		t.Fatalf("a job whose end was lost answered started %v, tool_failed %v, unreached %v, exit %d: %s",
			lost.JobStarted, lost.ToolFailed, lost.Unreached, lost.EffectiveExit, lost.Error)
	}
	if !strings.Contains(lost.Error, "could not read how it ended") {
		t.Errorf("a lost end does not say so: %s", lost.Error)
	}
}
