// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A fake wsl.exe: this test binary, started again with execFakeEnv set. It answers
// the calls the base exec, session and sync paths make, on any host, with no shell
// in it. WSL-123, WSL-124, WSL-125, WSL-133.
//
// ⭐ IT KEEPS A GUEST ON DISK. An archive unpacked into a guest path lands under
// execFakeRootEnv, and `tar -cf` packs execFakeOutEnv, so a fetch reads what a case
// put there. Every call is one line of execFakeLogEnv.

const (
	execFakeEnv        = "WTK_EXEC_FAKE_WSL"
	execFakeLogEnv     = "WTK_EXEC_FAKE_WSL_LOG"
	execFakeRootEnv    = "WTK_EXEC_FAKE_WSL_ROOT"
	execFakeOutEnv     = "WTK_EXEC_FAKE_WSL_OUT"
	execFakeMissingEnv = "WTK_EXEC_FAKE_WSL_MISSING"
	execFakeExitEnv    = "WTK_EXEC_FAKE_WSL_EXIT"
	execFakeUsersEnv   = "WTK_EXEC_FAKE_WSL_USERS"
	execFakeSurveyEnv  = "WTK_EXEC_FAKE_WSL_SURVEY"
	execFakeHome       = "/home/user"
)

func TestMain(m *testing.M) {
	if os.Getenv(execFakeEnv) != "" {
		os.Exit(execFakeWsl(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// execFakeCall is one call the fake answered.
type execFakeCall struct {
	User  string            `json:"user"`
	Cd    string            `json:"cd"`
	Argv  []string          `json:"argv"`
	Stdin string            `json:"stdin,omitempty"`
	Files map[string]string `json:"files,omitempty"`
}

var (
	execFakeStarted   = regexp.MustCompile(`printf '%s' '(wtk-exec-started-[0-9a-f]+)'`)
	execFakeRefused   = regexp.MustCompile(`printf '%s' '(wtk-exec-no-dir-[0-9a-f]+)'`)
	execFakeGateDir   = regexp.MustCompile(`(?m)^cd '([^']*)' \|\| \{`)
	execFakeIDName    = regexp.MustCompile(`id -u '([^']*)'`)
	execFakeJobMarker = regexp.MustCompile(`wtk-started-[0-9a-f]{32}`)
)

func execFakeWsl(args []string) int {
	var call execFakeCall
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-d":
			i++
		case "-u":
			i++
			call.User = args[i]
		case "--cd":
			i++
			call.Cd = args[i]
		case "--":
			call.Argv = args[i+1:]
			i = len(args)
		}
	}
	defer func() {
		if p := os.Getenv(execFakeLogEnv); p != "" {
			f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err == nil {
				b, _ := json.Marshal(call)
				_, _ = f.Write(append(b, '\n'))
				_ = f.Close()
			}
		}
	}()
	root := os.Getenv(execFakeRootEnv)
	guest := func(p string) string { return filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(p, "/"))) }
	argv := call.Argv
	switch {
	case len(argv) >= 1 && argv[0] == "/bin/mkdir":
		for _, d := range argv[1:] {
			if d != "-p" {
				_ = os.MkdirAll(guest(d), 0o700)
			}
		}
		return 0
	case len(argv) == 5 && argv[0] == "/bin/tar" && argv[1] == "-xf":
		call.Files = map[string]string{}
		tr := tar.NewReader(os.Stdin)
		for {
			h, err := tr.Next()
			if err != nil {
				break
			}
			body, _ := io.ReadAll(tr)
			call.Files[h.Name] = string(body)
			at := filepath.Join(guest(argv[4]), filepath.FromSlash(h.Name))
			if h.Typeflag == tar.TypeDir {
				_ = os.MkdirAll(at, 0o700)
				continue
			}
			_ = os.MkdirAll(filepath.Dir(at), 0o700)
			_ = os.WriteFile(at, body, 0o600)
		}
		return 0
	case len(argv) >= 5 && argv[0] == "/bin/tar" && argv[1] == "-cf":
		tw := tar.NewWriter(os.Stdout)
		_ = tw.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755})
		out := os.Getenv(execFakeOutEnv)
		_ = filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
			if err != nil || p == out {
				return err
			}
			rel, _ := filepath.Rel(out, p)
			name := "./" + filepath.ToSlash(rel)
			if info.IsDir() {
				return tw.WriteHeader(&tar.Header{Name: name + "/", Typeflag: tar.TypeDir, Mode: 0o755})
			}
			body, _ := os.ReadFile(p)
			_ = tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))})
			_, err = tw.Write(body)
			return err
		})
		_ = tw.Close()
		return 0
	case len(argv) == 2 && argv[0] == "/bin/sh":
		// A file the caller placed, run with the caller's stdin, which `cat` echoes.
		body, _ := os.ReadFile(guest(argv[1]))
		call.Stdin = string(body)
		return execFakeShell(string(body), true)
	case len(argv) == 1 && argv[0] == "/bin/sh":
		raw, _ := io.ReadAll(bufio.NewReader(os.Stdin))
		call.Stdin = string(raw)
		return execFakeShell(string(raw), false)
	}
	return 127
}

// execFakeShell answers a script by what it asks, the way the guest's shell would.
func execFakeShell(script string, catStdin bool) int {
	switch {
	case strings.Contains(script, `printf '%s\n' "$HOME"`):
		os.Stdout.WriteString(execFakeHome + "\n")
		return 0
	case execFakeIDName.MatchString(script):
		name := execFakeIDName.FindStringSubmatch(script)[1]
		for _, pair := range strings.Split(os.Getenv(execFakeUsersEnv), ",") {
			if n, uid, ok := strings.Cut(pair, ":"); ok && n == name {
				os.Stdout.WriteString(uid + "\n")
				return 0
			}
		}
		return 3
	case strings.Contains(script, "tk_dir="):
		os.Stdout.WriteString("removed\n")
		return 0
	case strings.Contains(script, "REAL|%s"):
		b, _ := os.ReadFile(os.Getenv(execFakeSurveyEnv))
		os.Stdout.Write(b)
		return 0
	case strings.Contains(script, "printf 'REMOVED\\n'"):
		os.Stdout.WriteString("REMOVED\n")
		return 0
	case execFakeJobMarker.MatchString(script):
		// A job's container: its wrapper's marker, and its own exit. ⭐ An
		// image that never ran writes no marker, and a job whose streams outlive
		// it leaves a child holding its stdout. WSL-127.
		if os.Getenv(execFakeNoMarkerEnv) == "" {
			os.Stderr.WriteString("\n" + execFakeJobMarker.FindString(script) + "\n")
		}
		if os.Getenv(execFakeLingerEnv) != "" {
			execFakeLinger()
		}
		code, _ := strconv.Atoi(os.Getenv(execFakeExitEnv))
		return code
	}
	if m := execFakeGateDir.FindStringSubmatch(script); m != nil && m[1] == os.Getenv(execFakeMissingEnv) {
		os.Stderr.WriteString("sh: cd: " + m[1] + ": No such file or directory\n")
		if r := execFakeRefused.FindStringSubmatch(script); r != nil {
			os.Stderr.WriteString(r[1])
		}
		return 2
	}
	if s := execFakeStarted.FindStringSubmatch(script); s != nil {
		os.Stderr.WriteString(s[1])
	}
	os.Stderr.WriteString("the command's own stderr\n")
	if catStdin {
		_, _ = io.Copy(os.Stdout, os.Stdin)
	}
	// ⭐ A command whose streams outlive it, as a job's can. WSL-127.
	if os.Getenv(execFakeLingerEnv) != "" {
		execFakeLinger()
	}
	code, _ := strconv.Atoi(os.Getenv(execFakeExitEnv))
	return code
}

// execFakeGuest is one case's fake wsl.exe and its guest on disk.
type execFakeGuest struct {
	wsl       *Wsl
	root, log string
}

// newExecFakeGuest points a Wsl at this test binary as wsl.exe.
func newExecFakeGuest(t *testing.T) *execFakeGuest {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	g := &execFakeGuest{wsl: &Wsl{Path: exe}, root: filepath.Join(dir, "guest"), log: filepath.Join(dir, "calls.jsonl")}
	t.Setenv(execFakeEnv, "1")
	t.Setenv(execFakeRootEnv, g.root)
	t.Setenv(execFakeLogEnv, g.log)
	t.Setenv(execFakeOutEnv, filepath.Join(dir, "out-empty"))
	t.Setenv(execFakeMissingEnv, "")
	t.Setenv(execFakeExitEnv, "0")
	t.Setenv(execFakeUsersEnv, "")
	t.Setenv(execFakeSurveyEnv, "")
	// The ledger and the session records go to a state directory of the case's own.
	t.Setenv("WSL_TOOLKIT_HOME", filepath.Join(dir, "home"))
	return g
}

// calls reads back every call the fake answered, in order.
func (g *execFakeGuest) calls(t *testing.T) []execFakeCall {
	t.Helper()
	b, err := os.ReadFile(g.log)
	if err != nil {
		t.Fatalf("the fake wsl.exe recorded nothing: %v", err)
	}
	var out []execFakeCall
	for _, line := range bytes.Split(bytes.TrimSpace(b), []byte("\n")) {
		var c execFakeCall
		if err := json.Unmarshal(line, &c); err != nil {
			t.Fatalf("a recorded call does not parse: %v: %s", err, line)
		}
		out = append(out, c)
	}
	return out
}
