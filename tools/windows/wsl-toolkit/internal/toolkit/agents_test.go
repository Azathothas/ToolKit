// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// museAgentBase is a configuration carrying the muse agent and one grant of a
// directory named project, with a sibling whose name starts the same way.
func museAgentBase(t *testing.T) (Config, string, string) {
	t.Helper()
	cfg := grantBase(t)
	cfg.Base.Image = "ghcr.io/pkgforge-dev/archlinux:latest"
	museAgent := BaseAdapter{Name: "muse"}
	cfg.Base.Adapters = []BaseAdapter{museAgent}
	root := t.TempDir()
	project, sibling := filepath.Join(root, "project"), filepath.Join(root, "projector")
	for _, dir := range []string{filepath.Join(project, "src", "deep"), sibling} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	grant := BaseMount{Source: project, Target: "/workspaces/project", Mode: BaseMountReadWrite}
	cfg.Base.Mounts = []BaseMount{grant}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the fixture's configuration is refused: %v", err)
	}
	return cfg, project, sibling
}

// TestAGrantCoversItsDirectoryAndWhatIsBeneathItAlone is WSL-78's mapping: the granted
// directory and every directory beneath it reach their guest paths, and a sibling whose
// name begins the same way, or the parent, is refused with the command that grants it.
func TestAGrantCoversItsDirectoryAndWhatIsBeneathItAlone(t *testing.T) {
	cfg, project, sibling := museAgentBase(t)
	for dir, want := range map[string]string{
		project:                               "/workspaces/project",
		filepath.Join(project, "src", "deep"): "/workspaces/project/src/deep",
	} {
		got, err := GrantedGuestDir(cfg, dir)
		if err != nil || got != want {
			t.Errorf("%s answered %q (%v), want %q", dir, got, err, want)
		}
	}
	if runtime.GOOS == "windows" {
		if got, err := GrantedGuestDir(cfg, strings.ToUpper(project)); err != nil || got != "/workspaces/project" {
			t.Errorf("the project in capitals answered %q (%v)", got, err)
		}
	}
	for _, dir := range []string{sibling, filepath.Dir(project)} {
		got, err := GrantedGuestDir(cfg, dir)
		if !errors.Is(err, ErrNotGranted) || got != "" || !strings.Contains(err.Error(), "base grant --source ") {
			t.Errorf("%s, which no grant covers, answered %q (%v)", dir, got, err)
		}
	}
}

// TestAnAgentRunsOnlyForAnAdapterThatIsOne holds the name check: muse is an agent,
// herdr is an adapter and not an agent, and a base with no agent says how to add one.
func TestAnAgentRunsOnlyForAnAdapterThatIsOne(t *testing.T) {
	cfg, _, _ := museAgentBase(t)
	if got, err := AgentCommand(cfg, "muse"); err != nil || got != "muse" {
		t.Fatalf("the muse agent answered %q (%v)", got, err)
	}
	cfg.Base.Adapters = append(cfg.Base.Adapters, herdrAdapter)
	if _, err := AgentCommand(cfg, "herdr"); err == nil || !strings.Contains(err.Error(), "It carries: muse") {
		t.Fatalf("herdr was run as an agent: %v", err)
	}
	cfg.Base.Adapters = []BaseAdapter{herdrAdapter}
	if _, err := AgentCommand(cfg, "muse"); err == nil || !strings.Contains(err.Error(), "names no agent adapter") {
		t.Fatalf("a base with no agent adapter answered %v", err)
	}
}

// TestAnAgentsOwnScreenIsNotStartedWithoutATerminal holds which calls need the agent's
// screen: none at all and resume do, and exec and a version question do not.
func TestAnAgentsOwnScreenIsNotStartedWithoutATerminal(t *testing.T) {
	screens := [][]string{nil, {"resume"}, {"resume", "--last"}}
	for _, args := range screens {
		if !AgentInteractive(args) {
			t.Errorf("%q was run with no terminal", args)
		}
	}
	headless := [][]string{{"exec", "--json", "PROMPT"}, {"--version"}, {"export", "SESSION"}}
	for _, args := range headless {
		if AgentInteractive(args) {
			t.Errorf("%q was sent to a herdr pane", args)
		}
	}
}

// TestALauncherNameCarriesItsInstance holds the launcher's identity in both directions:
// muse.exe reaches the base instance and muse-NAME.exe the instance NAME, the default
// instance has no launcher, and a name that is not an agent's is not a launcher.
func TestALauncherNameCarriesItsInstance(t *testing.T) {
	if got := LauncherFile("muse", "base"); got != "muse.exe" {
		t.Errorf("the base instance's launcher is %q", got)
	}
	if got := LauncherFile("muse", "m78"); got != "muse-m78.exe" {
		t.Errorf("a named instance's launcher is %q", got)
	}
	if got := LauncherFile("muse", DefaultInstance); got != "" {
		t.Errorf("the default instance got a launcher, %q", got)
	}
	for argv0, want := range map[string][2]string{
		`C:\Users\runner\bin\MUSE.EXE`: {"muse", "base"},
		"muse-m78.exe":                 {"muse", "m78"},
		"/usr/local/bin/muse":          {"muse", "base"},
	} {
		agent, instance, ok := LauncherIdentity(argv0)
		if !ok || agent != want[0] || instance != want[1] {
			t.Errorf("%s was read as (%q, %q, %v), want %v", argv0, agent, instance, ok, want)
		}
	}
	for _, argv0 := range []string{"wsl-toolkit.exe", "muse-.exe", "muse-Not A Name.exe", "herdr.exe", "musecode.exe"} {
		if agent, instance, ok := LauncherIdentity(argv0); ok {
			t.Errorf("%s was read as the launcher (%q, %q)", argv0, agent, instance)
		}
	}
}

// TestALauncherIsWrittenOnlyOverOneOfThisToolsBuilds holds the machine half: it writes a
// copy of this executable, checks it, leaves a file that is not a build of this tool
// untouched both ways, and removes only its own.
func TestALauncherIsWrittenOnlyOverOneOfThisToolsBuilds(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !isToolBuild(self) {
		t.Skipf("%s carries no build information for %s", self, toolModulePath)
	}
	previous := SelectedInstance
	t.Cleanup(func() { SelectedInstance = previous })
	SelectedInstance = Instance{Name: "m78", Distro: "wsl-toolkit-m78"}
	dir := t.TempDir()
	h := &agentLauncherHost{agent: "muse", dir: dir, self: self}
	b := &Base{cfg: DefaultConfig(), log: func(string) {}}
	launcher := filepath.Join(dir, "muse-m78.exe")

	if problems := h.check(context.Background(), b, nil); len(problems) != 1 || !strings.Contains(problems[0], "no muse launcher") {
		t.Fatalf("a missing launcher was checked as %v", problems)
	}
	if err := h.apply(context.Background(), b, nil); err != nil {
		t.Fatal(err)
	}
	if !sameFileBytes(launcher, self) {
		t.Fatalf("%s is not a copy of this executable", launcher)
	}
	if problems := h.check(context.Background(), b, nil); len(problems) != 0 {
		t.Fatalf("a written launcher was checked as %v", problems)
	}
	if err := h.remove(b); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(launcher); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the launcher is still there after removal: %v", err)
	}

	foreign := []byte("somebody else's muse\n")
	if err := os.WriteFile(launcher, foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := h.apply(context.Background(), b, nil); err == nil || !strings.Contains(err.Error(), "not a build of wsl-toolkit") {
		t.Fatalf("a file that is not this tool's was overwritten, or not refused: %v", err)
	}
	if err := h.remove(b); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(launcher); err != nil || !bytes.Equal(got, foreign) {
		t.Fatalf("a file that is not this tool's was changed or removed: %q (%v)", got, err)
	}
}

// TestAnAgentsArgumentsReachItAsWritten runs the command AgentScript writes under a POSIX
// shell, with a stand-in for the agent that prints each argument it receives: quotes, a
// dollar sign, a backtick, a glob, spaces and an empty argument arrive as written.
func TestAnAgentsArgumentsReachItAsWritten(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the command runs under a POSIX shell, which the Linux job has")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not on PATH")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "muse"), []byte("#!/bin/sh\nfor a in \"$@\"; do printf '[%s]\\n' \"$a\"; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"exec", "it's", `say "hi"`, "$HOME", "`id`", "*", "two  spaces", ""}
	cmd := exec.Command("sh")
	cmd.Stdin = bytes.NewReader(AgentScript("muse", args))
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Dir = t.TempDir()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the agent's command failed: %v\n%s", err, out)
	}
	var want strings.Builder
	for _, a := range args {
		want.WriteString("[" + a + "]\n")
	}
	if string(out) != want.String() {
		t.Fatalf("the agent received\n%s\nwant\n%s", out, want.String())
	}
}
