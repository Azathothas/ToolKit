// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	home := quietCLI(t)
	was := toolkit.ExplicitConfigPath
	t.Cleanup(func() { toolkit.ExplicitConfigPath = was })
	ctx := context.Background()
	forwards := [][]string{
		{"run", "--no-such-flag"},
		{"wait"},
		{"logs", cliJobID, "--follow", "--no-such-flag"},
		{"base", "exec", "--no-such-flag"},
		{"base", "shell", "--no-such-flag"},
		{"base", "agent"},
		{"distro", "new", "-c", "true", "--no-such-flag"},
		// ⛔ -c=false is the command `false`, not a boolean. WSL-127.
		{"distro", "new", "-c=false", "--no-such-flag"},
		{"distro", "run", "--no-such-flag"},
		{"distro", "enter", "--no-such-flag"},
		{"bsd", "run", "--disk", "0"},
		// ⛔ herdr answers 2 itself, so its channel's own refusal must not.
		{"base", "herdr"},
		// ⚠ LAST, because --config stays in force for the process.
		{"--config", filepath.Join(home, "no-such.json"), "base", "bootstrap", "--", "--toolset", "agent"},
	}
	for _, args := range forwards {
		if code := run(ctx, args); code != toolkit.ExitTool {
			t.Errorf("%v answered %d, want %d: it answers a job's own code, so its own refusal must not read as one", args, code, toolkit.ExitTool)
		}
	}
	toolkit.ExplicitConfigPath = ""
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
		// ⛔ AN EMPTY CODE IS NOT A CODE, from the flag. WSL-127.
		{[]string{"--tool-exit=", "config"}, exitCannot},
		{[]string{"--tool-exit", "", "config"}, exitCannot},
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
	// ⚠ AN EMPTY VARIABLE IS UNSET, as a shell leaves one.
	t.Setenv(toolkit.ToolExitEnv, "")
	if code := run(ctx, []string{"config", "--no-such-flag"}); code != exitCannot {
		t.Errorf("an empty %s answered config's refusal with %d", toolkit.ToolExitEnv, code)
	}
	if err := readToolExit(false); err != nil {
		t.Errorf("an empty %s was refused: %v", toolkit.ToolExitEnv, err)
	}
}

// ⛔ A REFUSED run --json ANSWERS A DOCUMENT THAT SAYS NO JOB STARTED. It wrote
// nothing, and a caller parsing the answer had nothing to parse.
func TestARefusedRunUnderJSONSaysNoJobStarted(t *testing.T) {
	quietCLI(t)
	ctx := context.Background()
	for _, c := range []struct {
		args    []string
		command string
		says    string
	}{
		{[]string{"run", "--json", "--no-such-flag"}, "run", "no-such-flag"},
		// ⭐ THE SUBCOMMAND IS NAMED, so a refused base exec is not just "base".
		{[]string{"base", "exec", "--json", "--no-such-flag"}, "base exec", "no-such-flag"},
		{[]string{"distro", "run", "--json", "--no-such-flag"}, "distro run", "no-such-flag"},
		{[]string{"distro", "new", "-c", "true", "--json", "--no-such-flag"}, "distro new", "no-such-flag"},
		{[]string{"bsd", "run", "--json", "--disk", "0"}, "bsd run", "--disk 0"},
	} {
		var code int
		out := captureStdout(t, func() { code = run(ctx, c.args) })
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("%v answered no document: %q", c.args, out)
		}
		if doc["schema"] != RefusalSchema || doc["command"] != c.command || doc["job_started"] != false || doc["effective_exit"] != float64(toolkit.ExitTool) ||
			!strings.Contains(doc["error"].(string), c.says) || code != toolkit.ExitTool {
			t.Errorf("%v answered %d and %v", c.args, code, doc)
		}
	}
	// ⚠ wait starts nothing, so it answers no document, and a command that
	// forwards nothing keeps its silence. distro new with no command forwards
	// nothing.
	for _, args := range [][]string{{"wait", "--json"}, {"matrix", "--json", "--no-such-flag"}, {"distro", "new", "--json", "--no-such-flag"}} {
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
}

// ⛔ AN ORPHAN WHOSE END NO OWNER RECORDED IS THIS TOOL'S FAILURE. It answered 1,
// "ran and disagreed", with job_started false, even for a container the engine
// still holds. It answers 125 with tool_failed and exit 2, as the direct route
// answers a lost end, and reads started, because it may have run. WSL-127.
func TestAnOrphanWhoseEndWasNotRecordedIsTheToolsFailure(t *testing.T) {
	for _, present := range []bool{true, false} {
		res, verdict := orphanEnd(cliJobID, nil, present, "")
		if !res.JobStarted || !res.ToolFailed || res.Exit != exitCannot || verdict != toolkit.ExitTool || res.EffectiveExit != toolkit.ExitTool {
			t.Errorf("an orphan with no exit, present %v, answered %+v, %d", present, res, verdict)
		}
	}
}

// ⛔ wait --json WRITES THE effective_exit IT ANSWERS. The owner sealed the
// record under its own --tool-exit, and a 7.x owner sealed an unreached job at
// 2, so the field disagreed with wait's own code. The record is not changed.
// WSL-127.
func TestAWaitAnswersTheEffectiveExitItReturns(t *testing.T) {
	recorded := toolkit.JobResult{ID: cliJobID, Exit: 2, Unreached: true, EffectiveExit: 2}
	end := jobEndOutcome{Schema: "wsl-toolkit-job-end/1", ID: cliJobID, Kind: "job", Result: &recorded, verdict: recorded.Verdict()}
	var code int
	out := captureStdout(t, func() { code, _ = renderEnd(end, true) })
	var doc struct {
		Result toolkit.JobResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("wait --json wrote %q", out)
	}
	if code != toolkit.ExitTool || doc.Result.EffectiveExit != code {
		t.Fatalf("wait answered %d and wrote effective_exit %d", code, doc.Result.EffectiveExit)
	}
	if recorded.EffectiveExit != 2 {
		t.Errorf("the record itself was resealed to %d", recorded.EffectiveExit)
	}
}

// ⛔ A JOB THAT NEVER STARTED, OR WHOSE END WAS LOST, IS NOT SAID TO HAVE EXITED.
// `run` printed "exited 2" beside a process that answered 125, and `ready
// --smoke` printed "exited 125" for a container that never started. WSL-127.
func TestAJobThatNeverRanIsNotSaidToHaveExited(t *testing.T) {
	was := quiet
	quiet = false
	t.Cleanup(func() { quiet = was })
	for _, c := range []struct {
		res  toolkit.JobResult
		says string
	}{
		{toolkit.JobResult{Label: "alpine", Exit: 2, Unreached: true, Error: "the container never started"}, "alpine did not start in"},
		{toolkit.JobResult{Label: "alpine", Exit: 2, JobStarted: true, ToolFailed: true}, "alpine started, and this tool lost how it ended in"},
		{toolkit.JobResult{Label: "alpine", Exit: 2, JobStarted: true}, "alpine exited 2 in"},
	} {
		got := captureStderr(t, func() { _, _ = reportJob(c.res, false) })
		if !strings.Contains(got, c.says) {
			t.Errorf("run said %q, want %q", got, c.says)
		}
	}
	never := smokeFailure(toolkit.JobResult{Exit: 2, Unreached: true, Error: "the container never started"})
	if !strings.HasPrefix(never, "the smoke job did not start: ") || strings.Contains(never, "exited") {
		t.Errorf("ready --smoke said %q for a container that never started", never)
	}
	if got := smokeFailure(toolkit.JobResult{Exit: 3, JobStarted: true}); !strings.HasPrefix(got, "the smoke job exited 3") {
		t.Errorf("ready --smoke said %q for a job that exited 3", got)
	}
	if got := smokeFailure(toolkit.JobResult{JobStarted: true}); got != "" {
		t.Errorf("ready --smoke said %q for a job that passed", got)
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
	// ⛔ A FLAG THAT TAKES A VALUE IS SET BY ANY VALUE, a word that reads as a
	// boolean included. WSL-127.
	for _, c := range []struct {
		args []string
		name string
		want bool
	}{
		{[]string{"-c", "true"}, "c", true},
		{[]string{"-c=false"}, "c", true},
		{[]string{"--script=0"}, "script", true},
		{[]string{"--", "-c=false"}, "c", false},
		{[]string{"--json"}, "c", false},
	} {
		if got := hasValueFlag(c.args, c.name); got != c.want {
			t.Errorf("%v read -%s as %v", c.args, c.name, got)
		}
	}
}

// ⭐ A CANCELLED CALL ANSWERS 130, and any other failure of the tool's own 2,
// which the dispatcher moves. WSL-127.
func TestACancelledCallIsAStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	if got := cannotOrStopped(ctx); got != exitCannot {
		t.Errorf("a call that was not cancelled answered %d", got)
	}
	cancel()
	if got := cannotOrStopped(ctx); got != 130 {
		t.Errorf("a cancelled call answered %d", got)
	}
}

// fakeRunHelper is a helper on this machine whose /v1/run answers with run, as
// the state directory's endpoint names it.
func fakeRunHelper(t *testing.T, home string, run http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"schema":"x","version":"8.0.0","pid":%d}`, os.Getpid())
			return
		}
		if r.URL.Path != "/v1/run" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, `{"error":"this fake answers no %s"}`, r.URL.Path)
			return
		}
		// ⚠ THE BODY IS READ FIRST. The server notices a client that went away
		// only once the request's body is consumed.
		_, _ = io.Copy(io.Discard, r.Body)
		run(w, r)
	}))
	writeHelperEndpoint(t, home, srv)
	return srv
}

// writeHelperEndpoint names srv as the helper serving home, as `helper serve`
// names itself.
func writeHelperEndpoint(t *testing.T, home string, srv *httptest.Server) {
	t.Helper()
	ep, err := json.Marshal(toolkit.HelperEndpoint{Schema: toolkit.HelperSchema, Address: strings.TrimPrefix(srv.URL, "http://"), Token: "t", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "helper.json"), ep, 0o600); err != nil {
		t.Fatal(err)
	}
}

// ⛔ run --via-helper --json ANSWERS A JOB'S RESULT FOR A JOB THE HELPER TOOK ON
// AND LOST. It answered the refusal document, job_started false, for a job that
// may still be running on the helper. WSL-127.
func TestAHelperJobWhoseAnswerWasLostIsNotARefusal(t *testing.T) {
	home := quietCLI(t)
	srv := fakeRunHelper(t, home, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"schema":%q,"kind":"stdout","b64":"aGFsZg=="}`+"\n", toolkit.HelperEventSchema)
	})
	defer srv.Close()
	var code int
	out := captureStdout(t, func() {
		code = run(context.Background(), []string{"run", "--via-helper", "--ensure-base=false", "--json", "--image", "alpine", "-c", "true"})
	})
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("a lost helper job answered %d and no document: %q", code, out)
	}
	if code != toolkit.ExitTool || doc["schema"] == RefusalSchema || doc["job_started"] != true || doc["tool_failed"] != true ||
		doc["effective_exit"] != float64(toolkit.ExitTool) || doc["label"] != "alpine" {
		t.Fatalf("a lost helper job answered %d and %v", code, doc)
	}
}

// ⛔ A run --via-helper CANCELLED BEFORE THE HELPER TOOK THE JOB ON ANSWERS 130,
// as the direct route and the manual's table answer, and its document says no
// job started. It answered the tool's code. WSL-127.
func TestAHelperRunCancelledBeforeItWasTakenOnIsAStop(t *testing.T) {
	home := quietCLI(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := fakeRunHelper(t, home, func(w http.ResponseWriter, r *http.Request) {
		// ⚠ THE CALL IS CANCELLED WHILE THE HELPER HAS ANSWERED NOTHING, so the
		// job was never taken on.
		cancel()
		<-r.Context().Done()
	})
	defer srv.Close()
	var code int
	out := captureStdout(t, func() {
		code = run(ctx, []string{"run", "--via-helper", "--ensure-base=false", "--json", "--image", "alpine", "-c", "true"})
	})
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("a cancelled helper run answered %d and no document: %q", code, out)
	}
	if code != 130 || doc["schema"] != RefusalSchema || doc["job_started"] != false || doc["effective_exit"] != float64(130) {
		t.Fatalf("a cancelled helper run answered %d and %v", code, doc)
	}
}
