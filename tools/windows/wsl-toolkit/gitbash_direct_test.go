// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-111: a Git Bash caller calls this tool directly ----------------------

// ⭐ With MSYS_NO_PATHCONV=1 a guest path survives, and a host path arrives in
// MSYS's own spelling. podssh bridged every call through PowerShell to get a
// Windows spelling, which is what this makes unnecessary.
func TestAnMsysHostPathIsReadTheWayMsysMapsIt(t *testing.T) {
	t.Setenv("MSYSTEM", "MINGW64")
	t.Setenv("EXEPATH", `C:\Program Files\Git\bin`)
	t.Setenv("TEMP", `C:\Temp\op`)
	for in, want := range map[string]string{
		"/c/src/podssh":      "C:/src/podssh",
		"/C/x":               "C:/x",
		"/d":                 "D:/",
		"/tmp/podssh.sh":     "C:/Temp/op/podssh.sh",
		"/tmp":               "C:/Temp/op",
		"/usr/bin/bash":      "C:/Program Files/Git/usr/bin/bash",
		"//server/share/x":   "//server/share/x",
		"relative/x":         "relative/x",
		`C:\already\windows`: `C:\already\windows`,
		"-":                  "-",
	} {
		if got := msysHostPath(in); got != want {
			t.Errorf("msysHostPath(%q) = %q, want %q", in, got, want)
		}
	}
	// ⭐ AND THE ONE RESOLVER EVERY JOB FLAG GOES THROUGH APPLIES IT. A drive path
	// is absolute only on Windows, so elsewhere it resolves under the working
	// directory: the translated spelling, not /c/src/podssh, is what proves it.
	// ⛔ READ ON EVERY HOST. Read on Windows alone, a resolver that skipped the
	// translation stayed green in CI's Linux mutation run.
	want := `C:\src\podssh`
	if runtime.GOOS != "windows" {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		want = filepath.Join(cwd, "C:/src/podssh")
	}
	if got, err := pathFromProject(toolkit.Config{}, "/c/src/podssh"); err != nil || got != want {
		t.Errorf("pathFromProject read /c/src/podssh as %q, %v; want %q", got, err, want)
	}
	t.Setenv("MSYSTEM", "")
	if got := msysHostPath("/c/src/op"); got != "/c/src/op" {
		t.Errorf("outside MSYS a path was translated to %q", got)
	}
}

// ⛔ WSL-187: `--home` and `WSL_TOOLKIT_HOME` skipped msysHostPath, so Git Bash's
// `/d/wsl-toolkit` named `C:\d\wsl-toolkit`, a second state root. Both spellings
// of the flag and the variable are read the way MSYS maps them, and the answer
// is exported absolute, so the helper and a detached child inherit it.
func TestAnMsysHomeNamesTheDirectoryMsysMapsAndIsHandedOnAbsolute(t *testing.T) {
	prefixMainCase(t)
	wasQuiet := quiet
	t.Cleanup(func() { quiet = wasQuiet })
	t.Setenv("MSYSTEM", "MINGW64")
	t.Setenv("EXEPATH", `C:\Program Files\Git\bin`)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// ⛔ READ ON EVERY HOST. A drive path is absolute only on Windows, so
	// elsewhere it resolves under the working directory: the translated spelling,
	// not /d/wsl-toolkit, is what proves it.
	want := `D:\wsl-toolkit`
	if runtime.GOOS != "windows" {
		want = filepath.Join(cwd, "D:/wsl-toolkit")
	}
	for _, c := range []struct {
		how, env string
		args     []string
	}{
		{"--home DIR", "", []string{"--home", "/d/wsl-toolkit"}},
		{"--home=DIR", "", []string{"--home=/d/wsl-toolkit"}},
		{"WSL_TOOLKIT_HOME", "/d/wsl-toolkit", nil},
	} {
		t.Setenv("WSL_TOOLKIT_HOME", c.env)
		args := append(append([]string{"--quiet"}, c.args...), "version")
		captureStdout(t, func() {
			if code := run(context.Background(), args); code != exitOK {
				t.Errorf("%s: answered %d", c.how, code)
			}
		})
		if got := os.Getenv("WSL_TOOLKIT_HOME"); got != want {
			t.Errorf("%s: the state directory handed on is %q, want %q", c.how, got, want)
		}
		if got, err := toolkit.Home(); err != nil || got != want {
			t.Errorf("%s: the state directory read is %q, %v; want %q", c.how, got, err, want)
		}
	}
	// ⭐ A RELATIVE ONE IS HANDED ON ABSOLUTE, so a child started elsewhere reads
	// the same directory.
	t.Setenv("MSYSTEM", "")
	t.Setenv("WSL_TOOLKIT_HOME", "state")
	captureStdout(t, func() {
		if code := run(context.Background(), []string{"--quiet", "version"}); code != exitOK {
			t.Errorf("a relative home answered %d", code)
		}
	})
	if got, want := os.Getenv("WSL_TOOLKIT_HOME"), filepath.Join(cwd, "state"); got != want {
		t.Errorf("a relative home was handed on as %q, want %q", got, want)
	}
}

// withStdin runs fn with this process's stdin reading body.
func withStdin(t *testing.T, body string, fn func()) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(body); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old; _ = f.Close() }()
	fn()
}

// ⭐ A pipe has no path, so the scratch file a Git Bash caller wrote per script
// is not needed.
func TestScriptDashReadsTheCommandFromStdin(t *testing.T) {
	withStdin(t, "set -u\r\necho from-stdin\r\n", func() {
		got, err := guestScript("", "", "-")
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "set -u\necho from-stdin\n" {
			t.Fatalf("the command read from stdin is %q; want the bytes with CRLF repaired", got)
		}
	})
	withStdin(t, "  \n", func() {
		if _, err := guestScript("", "", "-"); err == nil || !strings.Contains(err.Error(), "read nothing from stdin") {
			t.Fatalf("an empty stdin answered %v; want a refusal, never an empty command that exits 0", err)
		}
	})
}

// ⛔ The detached owner is a copy of this program with no stdin, so a second
// `--script -` would read nothing there.
func TestADetachedOwnerGetsTheStdinCommandAsAFile(t *testing.T) {
	for _, args := range [][]string{
		{"run", "--image", "alpine", "--script", "-", "--detach"},
		{"run", "--image", "alpine", "--script=-", "--detach"},
		{"run", "--image", "alpine", "-script", "-", "--detach"},
	} {
		got, err := replaceStdinScript(args, `C:\state\jobs\id\stdin-script.sh`)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		joined := strings.Join(got, " ")
		if strings.Contains(joined, " - ") || strings.HasSuffix(joined, "=-") || !strings.Contains(joined, `stdin-script.sh`) || len(got) != len(args) {
			t.Errorf("%v became %v; want the file in place of the dash and nothing else changed", args, got)
		}
	}
	if _, err := replaceStdinScript([]string{"run", "-c", "true"}, "x"); err == nil {
		t.Error("a command line with no --script - was rewritten without a word")
	}
}

func TestAnEnvFileIsReadAndAnEnvAfterItWins(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "job.env")
	if err := os.WriteFile(file, []byte("# a comment\r\nTOKEN_PATH=/c/not/rewritten\r\nKEEP=file\r\nOVERRIDE=file\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	j := jobFlags{envFile: file, env: stringList{"OVERRIDE=flag"}}
	got, err := j.envMap(toolkit.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if got["TOKEN_PATH"] != "/c/not/rewritten" || got["KEEP"] != "file" || got["OVERRIDE"] != "flag" || len(got) != 3 {
		t.Fatalf("--env-file then --env read as %v", got)
	}
	missing := jobFlags{envFile: filepath.Join(dir, "absent.env")}
	if _, err := missing.envMap(toolkit.Config{}); err == nil {
		t.Error("a missing --env-file was accepted")
	}
}

// ⭐ base exec's variables are part of the command, so they reach it attached,
// detached and in a private namespace alike, and no value is typed into a shell.
func TestBaseExecVariablesArePartOfTheCommand(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "exec.env")
	if err := os.WriteFile(file, []byte("FROM_FILE=a b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := baseExecFlags{command: `echo "$FROM_FILE $FROM_FLAG"`, envFile: file, env: stringList{"FROM_FLAG=$(touch /tmp/x)"}, dir: "~"}
	req, err := e.request(toolkit.Config{Base: toolkit.BaseConfig{Name: "wsl-toolkit", User: "toolkit"}})
	if err != nil {
		t.Fatal(err)
	}
	script := string(req.Script)
	file1 := strings.Index(script, "FROM_FILE='a b'; export FROM_FILE\n")
	flag1 := strings.Index(script, "FROM_FLAG='$(touch /tmp/x)'; export FROM_FLAG\n")
	cmd := strings.Index(script, `echo "$FROM_FILE $FROM_FLAG"`)
	if file1 < 0 || flag1 < 0 || cmd < 0 || !(file1 < flag1 && flag1 < cmd) {
		t.Fatalf("the command is not the file's variables, then the flag's, single-quoted, then the command:\n%s", script)
	}
	if _, err := (baseExecFlags{command: "true", env: stringList{"BAD-NAME=x"}, dir: "~"}).request(toolkit.Config{}); err == nil {
		t.Error("a name no shell can assign was accepted")
	}
}

// -- WSL-112: a project states what its copy leaves out once ------------------

// ⛔ The loader copies every stored field by name, so a field it does not name is
// decoded, validated as empty and dropped. base.adapters was, once.
func TestAProjectsJobSettingsReachEveryJob(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	root := t.TempDir()
	body := `{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit"},"jobs":{"container_lifecycle":"ephemeral","exclude":["target",".git"],"gitignore":true}}`
	if err := os.WriteFile(filepath.Join(root, toolkit.WorkingConfigName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	cfg, err := toolkit.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Jobs.Gitignore || strings.Join(cfg.Jobs.Exclude, " ") != "target .git" {
		t.Fatalf("the loader kept jobs as %+v", cfg.Jobs)
	}
	j := jobFlags{excludes: stringList{"extra"}}
	if err := j.applyConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if strings.Join(j.excludes, " ") != "target .git extra" || !j.gitignore {
		t.Fatalf("a job got excludes %v and gitignore %v; want the project's, then the command line's, and gitignore on", j.excludes, j.gitignore)
	}
}

// -- WSL-113: the flags a caller can get wrong ---------------------------------

// ⛔ The route this process chose for itself, because it cannot reach wsl.exe,
// is refused as the caller's --via-helper is: the helper owns a job it runs, so
// the lock would be held by nothing. Both entry points ask before they use the
// helper, so neither is given one here. A correctness review found it.
func TestTheHelperRouteRefusesExclusiveHoweverItWasChosen(t *testing.T) {
	var j jobFlags
	j.exclusive = "build"
	ctx := context.Background()
	if _, err := helperRunJob(ctx, nil, j, "alpine", "alpine", []byte("true"), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "cannot reach wsl.exe") {
		t.Errorf("a job the helper would run kept --exclusive: %v", err)
	}
	if _, err := helperRunMatrix(ctx, nil, j, []string{"alpine"}, 1, []byte("true"), nil, nil, nil, ""); err == nil || !strings.Contains(err.Error(), "cannot reach wsl.exe") {
		t.Errorf("a fleet the helper would run kept --exclusive: %v", err)
	}
	j.viaHelper = true
	if err := j.helperRouteRefusal(); err == nil || !strings.Contains(err.Error(), "without --via-helper") {
		t.Errorf("an explicit --via-helper answered %v", err)
	}
	j.exclusive = ""
	if err := j.helperRouteRefusal(); err != nil {
		t.Errorf("a job with no --exclusive was refused the helper: %v", err)
	}
}

// ⛔ The command line wins over a project's jobs.gitignore, false included: an
// explicit --gitignore=false parsed and was then overruled. A correctness review
// found it.
func TestAnExplicitGitignoreFalseBeatsTheProjects(t *testing.T) {
	cfg := toolkit.DefaultConfig()
	cfg.Jobs.Gitignore = true
	for _, c := range []struct {
		args []string
		want bool
	}{
		{nil, true},
		{[]string{"--gitignore=false"}, false},
		{[]string{"--gitignore"}, true},
	} {
		var j jobFlags
		fs := newFlagSet("run-gitignore-check")
		j.bind(fs)
		if err := fs.Parse(c.args); err != nil {
			t.Fatal(err)
		}
		if err := j.applyConfig(cfg); err != nil {
			t.Fatal(err)
		}
		if j.gitignore != c.want {
			t.Errorf("%v beside jobs.gitignore true read as %v; want %v", c.args, j.gitignore, c.want)
		}
	}
}

func TestExclusiveFlagsRefuseWhatCannotHold(t *testing.T) {
	base := func() jobFlags {
		var j jobFlags
		fs := newFlagSet("run-exclusive-check")
		j.bind(fs)
		return j
	}
	j := base()
	j.exclusiveWait = time.Minute
	if err := j.check(); err == nil || !strings.Contains(err.Error(), "none was given") {
		t.Errorf("--exclusive-wait with no name answered %v", err)
	}
	j = base()
	j.exclusive, j.viaHelper = "build", true
	if err := j.check(); err == nil || !strings.Contains(err.Error(), "--via-helper") {
		t.Errorf("--exclusive through the helper answered %v", err)
	}
	j = base()
	j.exclusive, j.exclusiveWait = "build", -time.Second
	if err := j.check(); err == nil || !strings.Contains(err.Error(), "negative") {
		t.Errorf("a negative wait answered %v", err)
	}
	fs := newFlagSet("run-exclusive-empty")
	j = jobFlags{}
	j.bind(fs)
	if err := fs.Parse([]string{"--exclusive", ""}); err == nil || !strings.Contains(err.Error(), "empty value is refused") {
		t.Errorf("an empty --exclusive answered %v; want a refusal rather than no lock", err)
	}
	j = base()
	j.exclusive = "not a name"
	if err := j.check(); err == nil {
		t.Error("a name with spaces was accepted")
	}
	j = base()
	j.exclusive, j.exclusiveWait = "build", time.Minute
	if err := j.check(); err != nil {
		t.Errorf("a good name and wait were refused: %v", err)
	}
}
