// SPDX-License-Identifier: 0BSD

package main

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// gitBashHost makes this process look like one started from Git Bash.
func gitBashHost(t *testing.T) {
	t.Helper()
	t.Setenv("MSYSTEM", "MINGW64")
	t.Setenv("EXEPATH", `C:\Program Files\Git\bin`)
	t.Setenv("TEMP", `C:\Users\user\AppData\Local\Temp`)
}

// TestAGitBashRewriteIsRefusedAndNamed is WSL-99's guard, on every host: it
// reads the environment and nothing else.
func TestAGitBashRewriteIsRefusedAndNamed(t *testing.T) {
	gitBashHost(t)
	cases := []struct {
		flag, value string
		path        bool
		refused     bool
		typed       string
	}{
		{"--dir", "C:/Program Files/Git/root/z", guestPath, true, "of /root/z"},
		{"--dir", "C:/Users/user/AppData/Local/Temp/w", guestPath, true, "of /tmp/w"},
		{"--dir", "D:/somewhere", guestPath, true, ""},
		{"--dir", "/root/z", guestPath, false, ""},
		{"--dir", "~", guestPath, false, ""},
		{"-c", "C:/Program Files/Git/root/x.sh arg", guestPath, true, "of /root/x.sh arg"},
		{"-c", "ls /tmp", guestPath, false, ""},
		{"--env", "X=C:/Program Files/Git/root/y", guestText, true, "of X=/root/y"},
		{"--env", "X=C:/Users/user/AppData/Local/Temp/a", guestText, true, "of X=/tmp/a"},
		{"--env", "WIN=C:/Users/user/project", guestText, false, ""},
		{"an argument to muse", "C:/Users/user/file.txt: why does this fail?", guestText, false, ""},
		{"an argument to muse", "C:/Program Files/Git/root/x", guestText, true, "of /root/x"},
		{"an argument to muse", "C:/Program Files/GitHub/x", guestText, false, ""},
	}
	for _, c := range cases {
		err := gitBashRewrite(c.flag, c.value, c.path)
		if (err != nil) != c.refused {
			t.Errorf("%s %q: err = %v, refused should be %v", c.flag, c.value, err, c.refused)
			continue
		}
		if err != nil && c.typed != "" && !strings.Contains(err.Error(), "rewrite "+c.typed) {
			t.Errorf("%s %q: err = %v, want it to name what was typed: %q", c.flag, c.value, err, c.typed)
		}
	}
}

func TestNothingIsRefusedWhereMSYSDidNotStartTheProcess(t *testing.T) {
	t.Setenv("MSYSTEM", "")
	t.Setenv("EXEPATH", `C:\Program Files\Git\bin`)
	if err := gitBashRewrite("--dir", "C:/Program Files/Git/root/z", guestPath); err != nil {
		t.Errorf("no MSYS program started this process, and it refused: %v", err)
	}
}

func TestTheMSYSRootIsReadWhateverTheSeparators(t *testing.T) {
	for exe, want := range map[string]string{
		`C:\Program Files\Git\bin`:  "C:/Program Files/Git",
		`C:\Program Files\Git\bin\`: "C:/Program Files/Git",
		`C:/msys64/usr/bin`:         "C:/msys64/usr",
		``:                          "",
		`bin`:                       "",
	} {
		t.Setenv("EXEPATH", exe)
		if got := msysRoot(); got != want {
			t.Errorf("EXEPATH %q: root %q, want %q", exe, got, want)
		}
	}
}

// TestEveryGuestDoorRefusesARewrite reaches the guard through the code each
// command runs, so a door that stops calling it goes red here.
func TestEveryGuestDoorRefusesARewrite(t *testing.T) {
	gitBashHost(t)
	const rewritten = "C:/Program Files/Git/root/x"
	if _, err := guestScript(rewritten+" --flag", "", ""); err == nil {
		t.Error("run, matrix, base exec and bsd run: a rewritten -c was accepted")
	}
	j := jobFlags{env: stringList{"X=" + rewritten}}
	if _, err := j.envMap(); err == nil {
		t.Error("run and matrix: a rewritten --env value was accepted")
	}
	// ⚠ The absolute-path rule refuses this --dir too, so the case reads the
	// message rather than the refusal alone.
	e := baseExecFlags{command: "true", dir: rewritten}
	if _, err := e.request(toolkit.DefaultConfig()); err == nil || !strings.Contains(err.Error(), "rewrite of /root/x") {
		t.Errorf("base exec: a rewritten --dir answered %v", err)
	}
	c := commandFlags{command: rewritten}
	if _, err := c.payload(toolkit.DefaultConfig()); err == nil {
		t.Error("distro: a rewritten -c was accepted")
	}
	c = commandFlags{command: "true", env: stringList{"X=" + rewritten}}
	if _, err := c.payload(toolkit.DefaultConfig()); err == nil {
		t.Error("distro: a rewritten --env value was accepted")
	}
	// ⚠ The target rule refuses this path too, so the case reads the message:
	// only the guard names the rewrite and what was typed.
	if code, err := cmdBaseGrant(t.Context(), "revoke", []string{"--target", rewritten}); err == nil || !strings.Contains(err.Error(), "rewrite of /root/x") {
		t.Errorf("base revoke: a rewritten --target answered %d, %v", code, err)
	}
	if code, err := cmdBaseAgent(t.Context(), []string{"muse", "--", rewritten}); err == nil || !strings.Contains(err.Error(), "rewrite") {
		t.Errorf("base agent: a rewritten argument answered %d, %v", code, err)
	}
	if code, err := cmdBaseHerdr(t.Context(), []string{"--", "pane", "send-text", rewritten}); err == nil || !strings.Contains(err.Error(), "rewrite") {
		t.Errorf("base herdr: a rewritten argument answered %d, %v", code, err)
	}
}

// TestTheThreeSpellingsOfACommandReachEveryJobCommand holds --command-base64
// on the commands that lacked it: one reader, the same repair.
func TestTheThreeSpellingsOfACommandReachEveryJobCommand(t *testing.T) {
	got, err := guestScript("", base64.StdEncoding.EncodeToString([]byte("printf 'ok'\r\n")), "")
	if err != nil || string(got) != "printf 'ok'\n" {
		t.Errorf("--command-base64 read %q, %v; want the repaired copy", got, err)
	}
	if _, err := guestScript("", "not base64!", ""); err == nil || !strings.Contains(err.Error(), "--command-base64") {
		t.Errorf("an invalid --command-base64 answered %v", err)
	}
	if _, err := guestScript("true", base64.StdEncoding.EncodeToString([]byte("true")), ""); err == nil {
		t.Error("-c and --command-base64 together were accepted")
	}
	sets, err := collectManualFlagSets(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"run", "matrix", "base exec", "bsd run"} {
		fs, ok := sets[name]
		if !ok {
			t.Errorf("%s has no registered flag set", name)
			continue
		}
		if fs.Lookup("command-base64") == nil {
			t.Errorf("%s takes -c and no --command-base64", name)
		}
	}
}
