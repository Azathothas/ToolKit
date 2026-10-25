// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-123: base exec --user ---------------------------------------------------

func TestBaseExecRunsAsTheAccountItNames(t *testing.T) {
	cfg := toolkit.DefaultConfig()
	for _, c := range []struct {
		opts baseExecFlags
		want string
		bad  string
	}{
		{baseExecFlags{command: "id", dir: "~"}, cfg.Base.User, ""},
		{baseExecFlags{command: "id", dir: "~", asRoot: true}, "root", ""},
		{baseExecFlags{command: "id", dir: "~", user: "alice"}, "alice", ""},
		{baseExecFlags{command: "id", dir: "~", user: "alice", asRoot: true}, "", "--root and --user both"},
		{baseExecFlags{command: "id", dir: "~", user: "a b"}, "", "not an account name"},
		{baseExecFlags{command: "id", dir: "~", user: "root", privateNet: true}, "", "--private-net and --root are contradictory"},
	} {
		req, err := c.opts.request(cfg)
		switch {
		case c.bad == "" && (err != nil || req.User != c.want):
			t.Errorf("%+v ran as %q, %v; want %q", c.opts, req.User, err, c.want)
		case c.bad != "" && (err == nil || !strings.Contains(err.Error(), c.bad)):
			t.Errorf("%+v answered %v, want a refusal naming %q", c.opts, err, c.bad)
		}
	}
	// What the base answers about the account.
	if err := accountRefusal("wsl-toolkit", "bob", 0, false, false); err == nil || !strings.Contains(err.Error(), "has no account \"bob\"") || !strings.Contains(err.Error(), "useradd -m bob") {
		t.Errorf("an account the base lacks answered %v", err)
	}
	if err := accountRefusal("wsl-toolkit", "toor", 0, true, true); err == nil || !strings.Contains(err.Error(), "is uid 0") {
		t.Errorf("a private namespace for an account that is uid 0 answered %v", err)
	}
	if err := accountRefusal("wsl-toolkit", "toor", 0, true, false); err != nil {
		t.Errorf("an account that is uid 0, with no namespace, was refused: %v", err)
	}
	if err := accountRefusal("wsl-toolkit", "alice", 1001, true, true); err != nil {
		t.Errorf("alice in a private namespace was refused: %v", err)
	}
}

// -- WSL-124: --dir and --here through a grant -----------------------------------

func TestAWindowsDirectoryIsMappedToWhereTheBaseSeesIt(t *testing.T) {
	for _, c := range []struct {
		v    string
		want bool
	}{
		{`C:\work\project`, true}, {`d:/work`, true}, {"/workspaces/p", false}, {"~", false}, {"C:", false}, {"1:/x", false},
	} {
		if got := windowsDirArg(c.v); got != c.want {
			t.Errorf("windowsDirArg(%q) = %v, want %v", c.v, got, c.want)
		}
	}
	ro := toolkit.DefaultConfig()
	ro.Base.Automount = toolkit.AutomountReadOnly
	if got, err := windowsDirInGuest(ro, `C:\src\x\proj`, "--dir"); err != nil || got != "/mnt/c/src/x/proj" {
		t.Errorf("with the drives mounted, a Windows directory maps to %q, %v", got, err)
	}
	// ⭐ THROUGH THE REQUEST, on any host: a drive path is read by its own grammar.
	if req, err := (baseExecFlags{command: "pwd", dir: `D:\work\project`}).request(ro); err != nil || req.Dir != "/mnt/d/work/project" {
		t.Errorf("--dir D:\\work\\project starts in %q, %v", req.Dir, err)
	}

	project := t.TempDir()
	sub := filepath.Join(project, "src")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	off := toolkit.DefaultConfig()
	off.Base.Automount, off.Base.Interop = toolkit.AutomountOff, toolkit.BaseInteropOff
	off.Base.Mounts = []toolkit.BaseMount{
		{Source: project, Target: "/workspaces/project", Mode: "rw"},
	}
	if got, err := windowsDirInGuest(off, sub, "--dir"); err != nil || got != "/workspaces/project/src" {
		t.Fatalf("a directory inside a grant maps to %q, %v", got, err)
	}
	outside := t.TempDir()
	if _, err := windowsDirInGuest(off, outside, "--here"); err == nil || !strings.Contains(err.Error(), "--here "+outside+" is not granted") ||
		!strings.Contains(err.Error(), "base grant --source") {
		t.Errorf("a directory no grant covers answered %v", err)
	}

	// --here reads the working directory, on any host.
	t.Chdir(sub)
	req, err := baseExecFlags{command: "pwd", dir: "~", here: true}.request(off)
	if err != nil || req.Dir != "/workspaces/project/src" {
		t.Fatalf("--here from inside the grant starts in %q, %v", req.Dir, err)
	}
	if _, err := (baseExecFlags{command: "pwd", dir: "/tmp", here: true}).request(off); err == nil || !strings.Contains(err.Error(), "--here and --dir") {
		t.Errorf("--here beside --dir answered %v", err)
	}
	// ⭐ base shell --here takes the same grant when the drives are unmounted,
	// and leaves a mounted base to its /mnt path.
	cwd := func() (string, error) { return sub, nil }
	if got, err := grantedHere(off, toolkit.AutomountOff, true, cwd); err != nil || got != "/workspaces/project/src" {
		t.Errorf("base shell --here from inside the grant starts in %q, %v", got, err)
	}
	if got, err := grantedHere(off, toolkit.AutomountOff, false, cwd); err != nil || got != "" {
		t.Errorf("base shell without --here starts in %q, %v", got, err)
	}
	if got, err := grantedHere(ro, toolkit.AutomountReadOnly, true, cwd); err != nil || got != "" {
		t.Errorf("base shell --here on a mounted base starts in %q, %v", got, err)
	}
	if _, err := grantedHere(off, toolkit.AutomountOff, true, func() (string, error) { return outside, nil }); err == nil || !strings.Contains(err.Error(), "is not granted") {
		t.Errorf("base shell --here outside every grant answered %v", err)
	}
	if runtime.GOOS == "windows" {
		// A drive path names the directory on this machine.
		req, err := baseExecFlags{command: "pwd", dir: sub}.request(off)
		if err != nil || req.Dir != "/workspaces/project/src" {
			t.Errorf("--dir %s starts in %q, %v", sub, req.Dir, err)
		}
	}
}

// -- WSL-124: inputs, artifacts and stdin ----------------------------------------

func TestBaseExecReadsItsInputsAndArtifactsFromTheCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "app.toml"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ins, err := baseExecFlags{inputs: stringList{"conf/app.toml=app.toml"}}.execInputs()
	if err != nil || len(ins) != 1 || ins[0].Name != "conf/app.toml" || ins[0].Path != filepath.Join(dir, "app.toml") {
		t.Fatalf("the input read as %+v, %v", ins, err)
	}
	for _, bad := range []stringList{{"app.toml"}, {"../x=app.toml"}, {"a=app.toml", "a=app.toml"}, {"a=missing.toml"}} {
		if _, err := (baseExecFlags{inputs: bad}).execInputs(); err == nil {
			t.Errorf("--input %q was accepted", bad)
		}
	}
	out, err := baseExecFlags{artifacts: "out"}.artifactDir()
	if err != nil || out != filepath.Join(dir, "out") {
		t.Errorf("--artifacts out resolved to %q, %v", out, err)
	}
	root := string(filepath.Separator)
	if runtime.GOOS == "windows" {
		root = filepath.VolumeName(dir) + `\`
	}
	if _, err := (baseExecFlags{artifacts: root}).artifactDir(); err == nil {
		t.Error("--artifacts at a filesystem root was accepted")
	}
}

func TestStdinIsPassedOnlyWhereItIsAPipeOrAFile(t *testing.T) {
	if err := (baseExecFlags{stdin: true}).stdinRefusal(true); err == nil || !strings.Contains(err.Error(), "is a terminal") {
		t.Errorf("--stdin from a terminal answered %v", err)
	}
	if err := (baseExecFlags{stdin: true, scriptFile: stdinScript}).stdinRefusal(false); err == nil || !strings.Contains(err.Error(), "--script -") {
		t.Errorf("--stdin with --script - answered %v", err)
	}
	if err := (baseExecFlags{stdin: true}).stdinRefusal(false); err != nil {
		t.Errorf("--stdin from a pipe was refused: %v", err)
	}
	if err := (baseExecFlags{}).stdinRefusal(true); err != nil {
		t.Errorf("no --stdin was refused: %v", err)
	}
	got, err := readDetachedStdin(strings.NewReader(""))
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("an empty stdin read as %v, %v, and a detached session needs an empty file rather than none", got, err)
	}
}

// TestAnAttachedExecAnswersTheExitItsOutcomeMeans is cmdBaseExec's verdict after
// the command: a directory it could not enter is 2, not the guest's exit, and a
// failed artifact copy turns a success into 1 but never hides a failure. WSL-124.
func TestAnAttachedExecAnswersTheExitItsOutcomeMeans(t *testing.T) {
	refused := fmt.Errorf("%w: /srv/nope", toolkit.ErrDirNotEntered)
	// RunBaseExec reports no exit for a command that never ran, so the 2 is this verdict's.
	if code, err := attachedExecExit(toolkit.BaseExecResult{}, refused, true, ""); code != exitCannot || !errors.Is(err, toolkit.ErrDirNotEntered) {
		t.Errorf("a directory the command could not enter answered %d, %v", code, err)
	}
	kept := toolkit.BaseExecResult{ArtifactError: "the copy stopped", Retained: "/home/user/.wsl-toolkit/jobs/x", ID: "x"}
	if code, err := attachedExecExit(kept, nil, true, "out"); code != exitFailed || err != nil {
		t.Errorf("a command that succeeded and delivered nothing answered %d, %v", code, err)
	}
	kept.Exit = 9
	if code, err := attachedExecExit(kept, nil, false, "out"); code != 9 || err != nil {
		t.Errorf("the command's own failure answered %d, %v; want 9", code, err)
	}
	if code, err := attachedExecExit(toolkit.BaseExecResult{}, nil, true, ""); code != exitOK || err != nil {
		t.Errorf("a command that succeeded answered %d, %v", code, err)
	}
}

// TestADetachedSessionReadsStdinOnlyWithTheFlag: --detach --stdin hands the
// session the bytes, and without the flag the session keeps /dev/null. WSL-124.
func TestADetachedSessionReadsStdinOnlyWithTheFlag(t *testing.T) {
	got, err := (baseExecFlags{stdin: true}).detachedStdin(strings.NewReader("abc"))
	if err != nil || string(got) != "abc" {
		t.Errorf("--detach --stdin read %q, %v", got, err)
	}
	got, err = (baseExecFlags{}).detachedStdin(strings.NewReader("abc"))
	if err != nil || got != nil {
		t.Errorf("--detach without --stdin read %q, %v", got, err)
	}
}

// TestADetachedSessionsDeliveryFailureFailsASucceededCommand is a job's rule on a
// session's end.
func TestADetachedSessionsDeliveryFailureFailsASucceededCommand(t *testing.T) {
	failed := &toolkit.SessionDelivery{Error: "the artifact copy stopped"}
	if got := deliveryVerdict(exitOK, failed); got != exitFailed {
		t.Errorf("a command that succeeded and delivered nothing answered %d", got)
	}
	if got := deliveryVerdict(9, failed); got != 9 {
		t.Errorf("the command's own failure answered %d, want 9", got)
	}
	if got := deliveryVerdict(exitOK, &toolkit.SessionDelivery{Delivered: 2}); got != exitOK {
		t.Errorf("a delivered session answered %d", got)
	}
	if got := deliveryVerdict(exitOK, nil); got != exitOK {
		t.Errorf("a session that asked for nothing answered %d", got)
	}
	if line := artifactsNote(failed); !strings.HasPrefix(line, "! artifacts: ") {
		t.Errorf("a failed delivery reads %q", line)
	}
}

// -- WSL-133: base shell --private-net ---------------------------------------------

// ⛔ IT CALLS THE PARSE AND NOT cmdBase, so a mutation that removes the refusal
// cannot go on to attach a shell to whatever base this machine has.
func TestAPrivateShellIsRefusedToRoot(t *testing.T) {
	if _, err := parseBaseFlags("shell", []string{"--private-net", "--root"}); err == nil || !strings.Contains(err.Error(), "--private-net and --root are contradictory") {
		t.Fatalf("a private root shell answered %v", err)
	}
	if f, err := parseBaseFlags("shell", []string{"--private-net"}); err != nil || !f.privateNet {
		t.Errorf("a private shell as the account was refused: %v", err)
	}
}

// -- WSL-146: base agent --env ---------------------------------------------------

func TestAnAgentsEnvironmentComesBeforeItsName(t *testing.T) {
	name, passed, pairs, err := parseAgentArgs([]string{"--env", "PI_KEY=k e$y", "--env", "MODEL=one", "pi", "--", "exec", "--env", "X=1"})
	if err != nil || name != "pi" || strings.Join(passed, " ") != "exec --env X=1" || len(pairs) != 2 {
		t.Fatalf("parsed %q %q %+v, %v", name, passed, pairs, err)
	}
	got := string(agentPayload("pi", passed, pairs))
	want := "PI_KEY='k e$y'; export PI_KEY\nMODEL='one'; export MODEL\nexec pi 'exec' '--env' 'X=1'\n"
	if got != want {
		t.Fatalf("the agent's payload is\n%q\nwant\n%q", got, want)
	}
	if _, _, _, err := parseAgentArgs([]string{"--env", "1BAD=x", "pi"}); err == nil || !strings.Contains(err.Error(), "not a usable environment name") {
		t.Errorf("a name no shell can assign answered %v", err)
	}
	if _, _, _, err := parseAgentArgs([]string{"--env", "A=1"}); err == nil || !strings.Contains(err.Error(), "takes an agent's name") {
		t.Errorf("--env with no agent answered %v", err)
	}
	if _, _, _, err := parseAgentArgs([]string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("--help answered %v", err)
	}
	// ⭐ A LAUNCHER PASSES EVERYTHING AS THE AGENT'S.
	if name, passed, pairs, err := parseAgentArgs([]string{"muse", "--", "--env", "Y=2"}); err != nil || name != "muse" || len(pairs) != 0 || strings.Join(passed, " ") != "--env Y=2" {
		t.Errorf("a launcher's arguments parsed as %q %q %+v, %v", name, passed, pairs, err)
	}
}

// -- WSL-125: base sync's two names ------------------------------------------------

func TestBaseSyncTakesTwoNamesBesideItsFlags(t *testing.T) {
	fs := newFlagSet("base sync")
	del := fs.Bool("delete", false, "")
	names, err := parseNames(fs, []string{"C:\\work", "--delete", "~/src/work"}, 2)
	if err != nil || len(names) != 2 || names[0] != `C:\work` || names[1] != "~/src/work" || !*del {
		t.Fatalf("parsed %q, delete %v, %v", names, *del, err)
	}
	for _, args := range [][]string{{"one"}, {"one", "two", "three"}} {
		if _, err := parseNames(newFlagSet("base sync"), args, 2); err == nil || !strings.Contains(err.Error(), "takes 2 names") {
			t.Errorf("%q answered %v", args, err)
		}
	}
	if code, err := cmdBaseSync(context.Background(), []string{"only-one"}); code != exitCannot || err == nil {
		t.Errorf("base sync with one name answered %d, %v", code, err)
	}
}
