// SPDX-License-Identifier: 0BSD

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-32, issue 7: the route is chosen on an access answer ----------------

func TestRouting(t *testing.T) {
	denied := fmt.Errorf("%w: Wsl/EnumerateDistros/Service/E_ACCESSDENIED", toolkit.ErrWslDenied)
	missing := fmt.Errorf("%w: no wsl.exe", toolkit.ErrWslMissing)
	other := errors.New("the probe timed out")

	cases := []struct {
		name    string
		probe   error
		helper  bool
		want    route
		because string
	}{
		{"the probe succeeded", nil, true, routeDirect,
			"a process that can reach wsl.exe pays nothing for the helper existing"},
		{"the probe succeeded and no helper is running", nil, false, routeDirect, ""},
		{"denied, with a helper listening", denied, true, routeHelper,
			"this is the whole defect: the tool took the direct route, failed, and advised starting the helper that was already answering"},
		{"denied, with no helper", denied, false, routeDirect,
			"the direct path's own refusal names both ways forward; a refusal invented here would name neither"},
		{"no wsl.exe, with a helper", missing, true, routeHelper, ""},
		{"no wsl.exe and no helper", missing, false, routeDirect, ""},
		{"the probe failed for some other reason", other, true, routeRefuse,
			"a helper does not fix a broken install, and hiding it behind a route change is how one reads as a sandbox"},
		{"some other reason, no helper", other, false, routeRefuse, ""},
	}
	for _, c := range cases {
		got := decideRoute(c.probe, c.helper)
		if got != c.want {
			t.Errorf("%s: decideRoute = %v, want %v. %s", c.name, got, c.want, c.because)
		}
	}
}

// TestRoutingDoesNotDecideOnAPathLookup is the mutation this defect needs.
//
// ⛔ The old rule was "FindWsl() succeeded", which is true on every Windows host
// with WSL installed, denied or not. A test that only checked the denied case
// would have passed against the broken code, because the broken code never saw a
// denial at all. This asserts the input is the PROBE's answer by feeding it one
// the path lookup could not have produced.
func TestRoutingDoesNotDecideOnAPathLookup(t *testing.T) {
	denied := fmt.Errorf("%w: E_ACCESSDENIED", toolkit.ErrWslDenied)
	if decideRoute(denied, true) != routeHelper {
		t.Fatal("a denial with a live helper did not route to it")
	}
	if decideRoute(nil, true) != routeDirect {
		t.Fatal("a successful probe routed away from the direct path, so the helper would carry work it does not need to")
	}
}

// -- WSL-38, issue 13: the refusals the CLI was not making -------------------

func TestFlagRefusalsRejectPositionals(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"one stray word", []string{"stray"}, `"stray"`},
		{"a stray word before a flag", []string{"stray", "--via-helper"}, "never parsed"},
		{"a stray word after every flag", []string{"--json", "stray"}, `"stray"`},
	}
	for _, c := range cases {
		fs := newFlagSet("run")
		fs.Bool("json", false, "")
		fs.Bool("via-helper", false, "")
		err := parseArgs(fs, c.args)
		if err == nil {
			t.Errorf("%s: accepted %v. The payload would have run and the flags after the word would have been ignored", c.name, c.args)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: the refusal does not contain %q: %v", c.name, c.want, err)
		}
	}
}

func TestFlagRefusalsAcceptOrdinaryFlags(t *testing.T) {
	fs := newFlagSet("run")
	j := fs.Bool("json", false, "")
	v := fs.Bool("via-helper", false, "")
	if err := parseArgs(fs, []string{"--json", "--via-helper"}); err != nil {
		t.Fatalf("a correct command line was refused: %v", err)
	}
	if !*j || !*v {
		t.Fatal("the flags did not bind")
	}
}

// TestEveryFlagSetRefusesPositionals is the guard against the shape that caused
// this. ⛔ `doctor` had the check and the other seven did not, because it was
// written at one call site instead of in the parse.
func TestEveryFlagSetRefusesPositionals(t *testing.T) {
	for _, name := range []string{"doctor", "images", "resources", "gc", "config", "run", "matrix", "base ensure", "helper serve", "version"} {
		fs := newFlagSet(name)
		if err := parseArgs(fs, []string{"unexpected"}); err == nil {
			t.Errorf("%s accepted a positional argument", name)
		}
	}
}

func TestJobFlagsRefuseValuesThatMeanSomethingElse(t *testing.T) {
	base := func() jobFlags {
		var j jobFlags
		fs := flag.NewFlagSet("run", flag.ContinueOnError)
		j.bind(fs)
		return j
	}

	j := base()
	j.timeout = -time.Second
	err := j.check()
	if err == nil {
		t.Error("--timeout -1s was accepted. A negative deadline was never applied, so the job ran unbounded")
	} else if !strings.Contains(err.Error(), "negative") {
		t.Errorf("the refusal does not say what is wrong: %v", err)
	}

	j = base()
	j.timeout = 0
	if err := j.check(); err != nil {
		t.Errorf("--timeout 0 was refused, and it is the documented way to ask for no deadline: %v", err)
	}

	j = base()
	j.maxBytes = 0
	if err := j.check(); err == nil {
		t.Error("--max-bytes 0 was accepted, which would refuse every workspace including an empty one")
	}

	j = base()
	j.maxEntries = -1
	if err := j.check(); err == nil {
		t.Error("--max-entries -1 was accepted")
	}

	j = base()
	if err := j.check(); err != nil {
		t.Errorf("the default flag values were refused: %v", err)
	}
}

func TestEnvNamesAreRefusedWhereTheyAreTyped(t *testing.T) {
	var j jobFlags
	j.env = stringList{"BAD-NAME=must-not-drop"}
	if _, err := j.envMap(); err == nil {
		t.Fatal("--env BAD-NAME=x was accepted. It was then dropped when the container was invoked, so the job ran without it and nothing said so")
	} else if !strings.Contains(err.Error(), "BAD-NAME") {
		t.Errorf("the refusal does not name the variable the caller typed: %v", err)
	}

	j.env = stringList{"NOT_A_PAIR"}
	if _, err := j.envMap(); err == nil {
		t.Error("--env NOT_A_PAIR was accepted")
	}

	j.env = stringList{"GOOD_NAME=1", "_also_good=", "A1=x"}
	got, err := j.envMap()
	if err != nil {
		t.Fatalf("legitimate names were refused: %v", err)
	}
	if len(got) != 3 || got["GOOD_NAME"] != "1" || got["A1"] != "x" {
		t.Fatalf("envMap = %v", got)
	}
}

func TestProjectConfigurationAnchorsRelativeHostPaths(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	root := t.TempDir()
	inner := filepath.Join(root, "nested")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit","image":"docker.io/library/alpine:latest","user":"toolkit"},"jobs":{"container_lifecycle":"persistent","workspace":"."}}`
	if err := os.WriteFile(filepath.Join(root, toolkit.WorkingConfigName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(inner); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()

	cfg, err := toolkit.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	var job jobFlags
	if err := job.applyConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if !sameHostPath(job.workspace, root) {
		t.Fatalf("configured workspace = %q, want configuration root %q", job.workspace, root)
	}
	if got, err := pathFromProject(cfg, "."); err != nil || !sameHostPath(got, root) {
		t.Fatalf("--workspace . = %q, %v; want project root %q", got, err, root)
	}
}

func sameHostPath(a, b string) bool {
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// -- WSL-33, issue 8: the verdict reads the transfer -------------------------

func TestJobVerdict(t *testing.T) {
	cases := []struct {
		name string
		res  toolkit.JobResult
		want int
	}{
		{"a clean run", toolkit.JobResult{Exit: 0}, exitOK},
		{"the container's own code is forwarded", toolkit.JobResult{Exit: 37}, 37},
		{"a deadline", toolkit.JobResult{Exit: 124, TimedOut: true}, exitTimeout},
		{"an image that never ran", toolkit.JobResult{Exit: 125, Unreached: true}, exitCannot},
		{"the command passed and its output did not arrive", toolkit.JobResult{Exit: 0, ArtifactError: "refused"}, exitFailed},
		{"the command failed AND its output did not arrive", toolkit.JobResult{Exit: 7, ArtifactError: "refused"}, 7},
	}
	for _, c := range cases {
		if got := jobVerdict(c.res); got != c.want {
			t.Errorf("%s: jobVerdict = %d, want %d", c.name, got, c.want)
		}
	}
}

// -- WSL-39, issue 14: unreached beats a status a payload could also return --

func TestUnreachedIsNotInferredFromAStatus(t *testing.T) {
	// ⛔ 125, 126 and 127 are podman's conventions AND legal values for a real
	// payload. The verdict must not read them.
	for _, code := range []int{125, 126, 127} {
		res := toolkit.JobResult{Exit: code}
		if got := jobVerdict(res); got != code {
			t.Errorf("a payload that exited %d was reported as %d; a classifier keyed to these numbers calls a working job unreached", code, got)
		}
	}
	if got := jobVerdict(toolkit.JobResult{Exit: 125, Unreached: true}); got != exitCannot {
		t.Errorf("an image that could not be acquired reported %d, want %d", got, exitCannot)
	}
}

// TestEveryJobFlagCrossesTheWire is the structural guard for the defect this
// tree has now had TWICE.
//
// ⛔ `--user` was accepted by the direct path and dropped by the helper, so a
// job ran as root after a caller asked for another account. That was fixed, and
// `--max-output` was then added and dropped the same way, found by a door sweep
// before it shipped. A guard applied by remembering is a guard that will be
// forgotten, so this one is applied by the compiler and the test together: every
// field of jobFlags must be named below, and one that describes the JOB must
// name the request field carrying it.
func TestEveryJobFlagCrossesTheWire(t *testing.T) {
	// Fields that describe the job, and what carries them over the protocol.
	carried := map[string]string{
		"command":     "ScriptB64",
		"scriptFile":  "ScriptB64",
		"workspace":   "StagingID",
		"artifactDir": "Artifacts",
		"env":         "Env",
		"timeout":     "TimeoutMS",
		"noNetwork":   "Network",
		"user":        "User",
		"platform":    "Platform",
		"lifecycle":   "ContainerLifecycle",
		"maxBytes":    "MaxBytes",
		"maxEntries":  "MaxEntries",
		"maxOutput":   "MaxOutput",
		"tick":        "TickMS",
	}
	// Fields that are the CLIENT's own business and correctly never sent.
	local := map[string]string{
		"excludes":  "applied by the client while it builds the upload, so the helper never sees a glob",
		"asJSON":    "how this process renders its own answer",
		"ensure":    "the client calls BaseEnsure itself before it sends the job",
		"viaHelper": "the routing decision, which is what chose this path",
	}

	spec := reflect.TypeOf(jobFlags{})
	req := reflect.TypeOf(toolkit.HelperRunRequest{})
	for i := 0; i < spec.NumField(); i++ {
		name := spec.Field(i).Name
		if _, ok := local[name]; ok {
			continue
		}
		field, ok := carried[name]
		if !ok {
			t.Errorf("jobFlags.%s is in neither table. Add it to `carried` with the request field that sends it, or to `local` with the reason it stays here", name)
			continue
		}
		if _, found := req.FieldByName(field); !found {
			t.Errorf("jobFlags.%s says it travels as HelperRunRequest.%s and there is no such field, so the helper route drops it", name, field)
		}
	}
	// And the tables must not name a field that no longer exists.
	for name := range carried {
		if _, ok := spec.FieldByName(name); !ok {
			t.Errorf("`carried` names jobFlags.%s and there is no such field", name)
		}
	}
	for name := range local {
		if _, ok := spec.FieldByName(name); !ok {
			t.Errorf("`local` names jobFlags.%s and there is no such field", name)
		}
	}
}

// -- the logs command, and the parse mistake a door sweep caught -------------

// TestLogsTakesTheIdOnlyFromTheFront is the case for a bug this file's own
// review found before it shipped.
//
// ⛔ Scanning the argument list for the first word without a leading dash reads
// `logs --tail 5` as a request for job "5", because a flag's VALUE has no dash
// either. The id is the first argument or there is none.
func TestLogsTakesTheIdOnlyFromTheFront(t *testing.T) {
	cases := []struct {
		args []string
		id   string
		rest []string
	}{
		{[]string{}, "", []string{}},
		{[]string{"abc123"}, "abc123", []string{}},
		{[]string{"abc123", "--tail", "5"}, "abc123", []string{"--tail", "5"}},
		{[]string{"--tail", "5"}, "", []string{"--tail", "5"}},
		{[]string{"--json"}, "", []string{"--json"}},
	}
	for _, c := range cases {
		id, rest := splitLogsArgs(c.args)
		if id != c.id {
			t.Errorf("%v: id = %q, want %q", c.args, id, c.id)
		}
		if strings.Join(rest, " ") != strings.Join(c.rest, " ") {
			t.Errorf("%v: rest = %v, want %v", c.args, rest, c.rest)
		}
	}
}

func TestLastLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.log")
	var b strings.Builder
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := lastLines(f, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"line 4997", "line 4998", "line 4999"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lastLines = %v, want %v", got, want)
	}
	// ⚠ Asking for more lines than the file has is not an error. It is the
	// whole file, which is what a caller passing a big --tail means.
	all, err := lastLines(f, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5000 {
		t.Fatalf("asking for more lines than exist gave %d, want 5000", len(all))
	}
}

// TestLastLinesGrowsItsWindow is the case the mutation pass showed was missing.
//
// ⛔ The file above is smaller than the first window, so it returned on the
// first read and the growth loop never ran: a mutation deleting that loop left
// the test green. These lines are 50 KiB each, so the last three cannot fit in
// the 64 KiB the reader starts with and the window HAS to grow.
func TestLastLinesGrowsItsWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wide.log")
	var b strings.Builder
	for i := 0; i < 8; i++ {
		b.WriteString(fmt.Sprintf("%d-", i))
		b.WriteString(strings.Repeat("w", 50<<10))
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := lastLines(f, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d line(s), want 3: the window did not grow past its first read", len(got))
	}
	for i, want := range []string{"5-", "6-", "7-"} {
		if !strings.HasPrefix(got[i], want) {
			t.Errorf("line %d starts %q, want the one beginning %q", i, got[i][:min(4, len(got[i]))], want)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestListTranscriptsSeparatesAnEmptyMachineFromAnUnreadableOne is the fifth
// review's fourth finding.
//
// ⛔ EVERY READ FAILURE USED TO BE REPORTED AS AN EMPTY MACHINE. A `jobs`
// path that was unreadable, or that was a file rather than a directory, produced
// the sentence "no transcripts on this machine yet" and exit 0: a true-sounding
// answer to a question this process could not answer, which is the defect class
// of issue #10 in a place the issue did not name. A missing directory really does
// mean "not yet" and still says so.
func TestListTranscriptsSeparatesAnEmptyMachineFromAnUnreadableOne(t *testing.T) {
	t.Run("a machine that has not run a job yet", func(t *testing.T) {
		code, err := listTranscripts(t.TempDir(), false)
		if err != nil {
			t.Fatalf("a machine with no jobs directory is not an error: %v", err)
		}
		if code != exitOK {
			t.Fatalf("exit %d for a machine that has simply not run a job", code)
		}
	})

	t.Run("a jobs path the operating system refuses", func(t *testing.T) {
		// ⚠ A NUL IN THE PATH, and the reason is measured rather than chosen
		// for convenience. The obvious case, a FILE where `jobs` has to be a
		// directory, is NOT usable: on Windows ReadDir answers that with
		// ERROR_PATH_NOT_FOUND, which os.IsNotExist reports as true, so it takes
		// the "not yet" branch and proves nothing. A NUL is refused as
		// `invalid argument` on both platforms, which is the shape of the failures
		// this branch exists for: unreadable, not absent.
		home := t.TempDir() + "\x00"
		code, err := listTranscripts(home, false)
		if err == nil {
			t.Fatal("an unreadable jobs path was reported as a machine with no transcripts")
		}
		if code != exitCannot {
			t.Fatalf("exit %d, want exitCannot (%d)", code, exitCannot)
		}
		if !strings.Contains(err.Error(), "jobs") {
			t.Fatalf("the refusal does not name the path: %v", err)
		}
	})
}

// TestTranscriptHintMakesLogsReachable covers the line that carries the job id.
//
// ⛔ WITHOUT IT THE ID IS NOWHERE IN THE HUMAN OUTPUT. `run` printed the
// image label and the exit code, so `wsl-toolkit logs JOB-...` could not be typed
// without first running `wsl-toolkit logs` bare to go hunting for the id.
func TestTranscriptHintMakesLogsReachable(t *testing.T) {
	cases := []struct {
		name string
		res  toolkit.JobResult
		want string
	}{
		{"a job with output kept",
			toolkit.JobResult{ID: "JOB-7", Transcript: "C:" + string(filepath.Separator) + "state"},
			"the complete output is kept: wsl-toolkit logs JOB-7"},
		{"a job whose output was not kept",
			toolkit.JobResult{ID: "JOB-7"}, ""},
		{"a transcript with no id to name it",
			toolkit.JobResult{Transcript: "somewhere"}, ""},
		// ⚠ The truncation line already names the path, and saying it twice
		// in two different shapes reads as two different facts.
		{"output that was cut at the capture limit",
			toolkit.JobResult{ID: "JOB-7", Transcript: "somewhere", StdoutTruncated: true}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := transcriptHint(c.res); got != c.want {
				t.Fatalf("transcriptHint = %q, want %q", got, c.want)
			}
		})
	}
}

// TestAJobWithNoPlatformStillCarriesOne holds the fix at the seam a caller
// reaches, because the resolution happens in applyConfig and a later edit there
// would restore the empty value without any package test noticing.
func TestAJobWithNoPlatformStillCarriesOne(t *testing.T) {
	var j jobFlags
	if err := j.applyConfig(toolkit.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	if j.platform != toolkit.NativePlatform() {
		t.Errorf("a job that asked for no platform carries %q, want the native %q", j.platform, toolkit.NativePlatform())
	}
	// A platform the caller DID ask for is never overwritten.
	asked := jobFlags{platform: "arm64"}
	if err := asked.applyConfig(toolkit.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	if asked.platform != "linux/arm64" {
		t.Errorf("--platform arm64 became %q", asked.platform)
	}
}
