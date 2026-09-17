// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
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
	for _, name := range []string{"doctor", "images", "resources", "gc", "config", "run", "matrix", "base ensure", "base exec", "helper serve", "version"} {
		fs := newFlagSet(name)
		if err := parseArgs(fs, []string{"unexpected"}); err == nil {
			t.Errorf("%s accepted a positional argument", name)
		}
	}
}

// TestBaseExecDefaultsToGuestHome is the guard against inheriting the Windows
// working directory. WSL does that when --cd is absent, which silently grants a
// base access to the caller's checkout whenever automount is enabled.
func TestBaseExecDefaultsToGuestHome(t *testing.T) {
	var opts baseExecFlags
	fs := flag.NewFlagSet("base exec", flag.ContinueOnError)
	opts.bind(fs)
	if err := parseArgs(fs, []string{"-c", "pwd"}); err != nil {
		t.Fatal(err)
	}
	cfg := toolkit.DefaultConfig()
	req, err := opts.request(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if req.Dir != "~" {
		t.Fatalf("base exec starts in %q, want the guest home; an empty directory inherits the Windows working directory", req.Dir)
	}
	if req.User != cfg.Base.User {
		t.Fatalf("base exec runs as %q, want the configured account %q", req.User, cfg.Base.User)
	}
	// ⛔ THE SECOND DOOR TO THE CALLER'S-COMMAND CHANNEL. Unframed, a command
	// reading stdin eats the lines of the script after it.
	if !req.Payload {
		t.Fatal("base exec sends the caller's command unframed")
	}
	opts.dir = "workspaces/project"
	if _, err := opts.request(cfg); err == nil {
		t.Fatal("a relative --dir was accepted, so its meaning depends on where WSL happened to start")
	}
}

// TestTheAutomountRowCarriesTheGuestsDrives is WSL-84's report: `base status --probe`
// printed the configured `ro` over a guest whose /mnt/c was writable. A probe that
// read the drives prints them beside the setting, and one that did not stays the
// setting alone rather than claiming a reading.
func TestTheAutomountRowCarriesTheGuestsDrives(t *testing.T) {
	unprobed := toolkit.BaseAccessState{Automount: toolkit.AutomountReadOnly}
	drifted := toolkit.BaseAccessState{Automount: toolkit.AutomountReadOnly, AutomountGuest: toolkit.AutomountReadWrite}
	agreed := toolkit.BaseAccessState{Automount: toolkit.AutomountOff, AutomountGuest: toolkit.AutomountOff}
	for _, c := range []struct {
		access toolkit.BaseAccessState
		want   string
	}{
		{unprobed, "ro"},
		{drifted, "ro, and the guest's drives read rw"},
		{agreed, "off, and the guest's drives read off"},
	} {
		if got := automountRow(c.access); got != c.want {
			t.Errorf("%+v printed %q, want %q", c.access, got, c.want)
		}
	}
}

// TestARootShellNamesEachDriveByItsMode is the root shell's warning. It called every
// drive "mounted and writable" in a base whose drives were read-only, where root's
// touch on /mnt/c answered "Read-only file system".
func TestARootShellNamesEachDriveByItsMode(t *testing.T) {
	readOnlyC := toolkit.WindowsDriveMount{Target: "/mnt/c", ReadOnly: true}
	readOnlyD := toolkit.WindowsDriveMount{Target: "/mnt/d", ReadOnly: true}
	writableE := toolkit.WindowsDriveMount{Target: "/mnt/e"}
	unread := errors.New("the mounts of wsl-toolkit could not be read (exit 1)")
	for _, c := range []struct {
		drives []toolkit.WindowsDriveMount
		err    error
		want   string
	}{
		{nil, nil, "root here is root INSIDE wsl-toolkit and not on this machine. No Windows drive is mounted"},
		{nil, unread, "root here is root INSIDE wsl-toolkit and not on this machine. Which Windows drives are mounted could not be read: " + unread.Error()},
		{[]toolkit.WindowsDriveMount{readOnlyC, readOnlyD}, nil,
			"root here is root INSIDE wsl-toolkit and not on this machine. These Windows drives are mounted read-only: /mnt/c /mnt/d"},
		{[]toolkit.WindowsDriveMount{readOnlyC, writableE}, nil,
			"root here is root INSIDE wsl-toolkit and not on this machine, AND these Windows drives are mounted writable: /mnt/e. These Windows drives are mounted read-only: /mnt/c"},
	} {
		if got := rootDrivesNote("wsl-toolkit", c.drives, c.err); got != c.want {
			t.Errorf("%+v noted %q, want %q", c.drives, got, c.want)
		}
	}
}

// TestAShellHereIntoABaseWithNoDriveIsRefused is the door that trusted the setting.
// On a base configured `ro` whose guest had no drive mounted, `base shell --here`
// noted "starting in this Windows directory" and started in the account's home. A read
// that failed refuses nothing, so the shell starts where WSL puts it, as before.
func TestAShellHereIntoABaseWithNoDriveIsRefused(t *testing.T) {
	readOnlyC := toolkit.WindowsDriveMount{Target: "/mnt/c", ReadOnly: true}
	err := hereRefusal("wsl-toolkit-m84", toolkit.AutomountReadOnly, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "has none although base.automount is ro. Run: wsl-toolkit base ensure") {
		t.Errorf("a base with no drive answered %v, want a refusal naming base ensure", err)
	}
	if err := hereRefusal("wsl-toolkit-m84", toolkit.AutomountReadOnly, []toolkit.WindowsDriveMount{readOnlyC}, nil); err != nil {
		t.Errorf("a base with a drive was refused: %v", err)
	}
	if err := hereRefusal("wsl-toolkit-m84", toolkit.AutomountReadOnly, nil, errors.New("could not be read")); err != nil {
		t.Errorf("a read that failed was refused as a base with no drive: %v", err)
	}
}

// TestAShellHereMarksItselfThroughWslenv is the other half of WSL-71's guard.
// scripts/common/shell-profile.sh moves an interactive shell off a Windows drive,
// which is right for a shell that inherited the directory and wrong for one that
// asked for it. The mark is how the guest tells them apart, and WSLENV is the only
// way a variable crosses at all: one not named there never arrives.
func TestAShellHereMarksItselfThroughWslenv(t *testing.T) {
	for _, c := range []struct {
		name  string
		given string
		want  []string
	}{
		{"nothing set", "", []string{"WSL_TOOLKIT_HERE=1", "WSLENV=WSL_TOOLKIT_HERE/u"}},
		{"the caller's list is kept", "GOPATH/p:EDITOR", []string{"WSL_TOOLKIT_HERE=1", "WSLENV=GOPATH/p:EDITOR:WSL_TOOLKIT_HERE/u"}},
		{"a trailing separator makes no empty entry", "EDITOR:", []string{"WSL_TOOLKIT_HERE=1", "WSLENV=EDITOR:WSL_TOOLKIT_HERE/u"}},
		{"already named, so not named twice", "EDITOR:WSL_TOOLKIT_HERE/u", []string{"WSL_TOOLKIT_HERE=1"}},
		{"already named with other flags", "WSL_TOOLKIT_HERE/w:EDITOR", []string{"WSL_TOOLKIT_HERE=1"}},
	} {
		got := hereEnv(c.given)
		if len(got) != len(c.want) {
			t.Errorf("%s: WSLENV %q gave %q, want %q", c.name, c.given, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: WSLENV %q gave %q, want %q", c.name, c.given, got, c.want)
				break
			}
		}
	}
}

// TestBaseExecSurfacesAFailureToStart holds the line between a guest's answer
// and a process that never ran. The guest's own exit status is forwarded
// silently; a `wsl.exe` that could not be started used to exit 2 with its reason
// dropped, which reads exactly like a guest script that ran `exit 2`.
func TestBaseExecSurfacesAFailureToStart(t *testing.T) {
	notStarted := &toolkit.ProcessError{Code: exitCannot, Op: "process", Err: exec.ErrNotFound}
	if code, err := baseExecResult(exitCannot, notStarted); err == nil || code != exitCannot {
		t.Fatalf("a wsl.exe that never started returned (%d, %v); want exit %d and its reason", code, err, exitCannot)
	}
	deadline := &toolkit.ProcessError{Code: exitTimeout, Op: "process", Err: context.DeadlineExceeded}
	if code, err := baseExecResult(exitTimeout, deadline); err == nil || code != exitTimeout {
		t.Fatalf("a deadline returned (%d, %v); want exit %d and its reason", code, err, exitTimeout)
	}
	if code, err := baseExecResult(exitOK, nil); err != nil || code != exitOK {
		t.Fatalf("a clean run returned (%d, %v); want (%d, nil)", code, err, exitOK)
	}
}

// TestBsdRunRefusesADiskThatIsNotASize is WSL-72's flag. Zero and a negative
// number are refused, because the library would read either as the default.
func TestBsdRunRefusesADiskThatIsNotASize(t *testing.T) {
	for _, gib := range []int{0, -3} {
		err := checkBsdDisk(gib)
		if err == nil || !strings.Contains(err.Error(), "not a disk size") {
			t.Errorf("--disk %d answered %v; want a refusal that says why", gib, err)
		}
	}
	if err := checkBsdDisk(1); err != nil {
		t.Errorf("--disk 1 was refused: %v", err)
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

// WSL-74: `config` refuses a configuration naming another instance's
// distribution with exit 2, and still says which file won and where it looked.
func TestConfigRefusesAnotherInstancesDistributionAndStillReportsItsSearch(t *testing.T) {
	museHome := filepath.Join(t.TempDir(), "instances", "muse")
	t.Setenv("WSL_TOOLKIT_HOME", museHome)
	previous := toolkit.SelectedInstance
	t.Cleanup(func() { toolkit.SelectedInstance = previous })
	toolkit.SelectedInstance = toolkit.Instance{Name: "muse", Distro: "wsl-toolkit-muse", Home: museHome}
	project := t.TempDir()
	file := filepath.Join(project, toolkit.WorkingConfigName)
	if err := os.WriteFile(file, []byte(`{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	captured, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	realErr := os.Stderr
	os.Stderr = captured
	code, runErr := cmdConfig([]string{"--json"})
	os.Stderr = realErr
	if err := captured.Close(); err != nil {
		t.Fatal(err)
	}
	said, err := os.ReadFile(captured.Name())
	if err != nil {
		t.Fatal(err)
	}
	if code != exitCannot || !errors.Is(runErr, toolkit.ErrInstanceMismatch) {
		t.Fatalf("config answered %d, %v, rather than refusing with %d", code, runErr, exitCannot)
	}
	if !strings.Contains(string(said), file) || !strings.Contains(string(said), "the working directory or a parent") {
		t.Errorf("the refusal did not report the file that won and where it came from:\n%s", said)
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

// TestHerdrCommandsThatWantATerminalAreNamedRatherThanRun holds the line between a
// herdr command this channel can carry and one that would hang with nothing to draw
// on. herdr's own agent skill says a bare `herdr` launches or attaches its UI.
func TestHerdrCommandsThatWantATerminalAreNamedRatherThanRun(t *testing.T) {
	for _, c := range []struct {
		args []string
		want bool
	}{
		{nil, true},
		{[]string{"attach"}, true},
		{[]string{"agent", "attach", "reviewer"}, true},
		{[]string{"terminal", "attach", "t1"}, true},
		{[]string{"agent", "list"}, false},
		{[]string{"pane", "read", "w1:p1"}, false},
		{[]string{"agent", "prompt", "w1:p1", "attach the debugger"}, false},
		{[]string{"workspace", "list"}, false},
	} {
		if got := herdrWantsATerminal(c.args); got != c.want {
			t.Errorf("herdrWantsATerminal(%q) = %v, want %v", c.args, got, c.want)
		}
	}
}

// ⛔ --private-net AND --root ARE CONTRADICTORY. The namespace is what confines the
// ACCOUNT; guest root can unmount, re-mount and re-enter whatever it likes, so
// wrapping a root payload would put a boundary around something that can step over
// it and then report that it had been confined. WSL-68.
func TestPrivateNetAndRootAreRefusedTogether(t *testing.T) {
	cfg := toolkit.DefaultConfig()
	opts := baseExecFlags{command: "true", dir: "~", privateNet: true, asRoot: true}
	if _, err := opts.request(cfg); err == nil || !strings.Contains(err.Error(), "--private-net and --root") {
		t.Fatalf("request() = %v, want a refusal naming both flags", err)
	}
	opts.asRoot = false
	req, err := opts.request(cfg)
	if err != nil {
		t.Fatalf("request() as the account = %v, want it accepted", err)
	}
	if req.Env["TK_PRIVATE_NET_PAYLOAD_B64"] == "" {
		t.Fatal("the payload did not reach the wrapper's environment")
	}
	if req.User != cfg.Base.User {
		t.Fatalf("the wrapped request runs as %q, want the configured account %q", req.User, cfg.Base.User)
	}
}

// ⛔ `base shell` LANDED IN sh WITH NO PROFILE READ. `wsl.exe -d N -u U` runs the
// account's passwd shell, which the provisioner leaves at /bin/sh, so an
// interactive attach got no line editing, no history and no profile - on a base
// that has bash installed. Measured 2026-09-17 on the operator's own base: passwd
// shell /bin/sh, bash present, and the operator asked for it. The choice is made
// in the GUEST, in the same call, because reading the shell from this side is a
// second round trip and still a guess about that machine's PATH.
func TestBaseShellPrefersBashAsALoginShell(t *testing.T) {
	body, err := os.ReadFile("cmd_base.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	// ⛔ THREE ARMS, AND EVERY ONE A LOGIN SHELL. Driven on a real base 2026-09-17:
	// bash where there is one; else the account's OWN passwd shell, proved with a
	// PATH holding getent, cut and id and no bash, which chose /usr/bin/bash; else
	// /bin/sh. A single fallback would hand a guest running zsh a /bin/sh it never
	// configured.
	for _, arm := range []string{
		`if command -v bash >/dev/null 2>&1; then exec bash -l; fi; `,
		`s=$(getent passwd "$(id -un)" 2>/dev/null | cut -d: -f7); `,
		`if [ -n "$s" ] && [ -x "$s" ]; then exec "$s" -l; fi; `,
		`exec /bin/sh -l`,
	} {
		if !strings.Contains(src, arm) {
			t.Errorf("base shell lost an arm of its shell selection: %s", arm)
		}
	}
	// ⚠ EVERY ARM IS A LOGIN SHELL. The `-l` is the half that was missing before,
	// and dropping it from any one arm is the regression that reads as working.
	if n := strings.Count(src, " -l; fi;") + strings.Count(src, "exec /bin/sh -l`)"); n < 2 {
		t.Errorf("the shell selection has %d login arms, want every arm to carry -l", n)
	}
}
