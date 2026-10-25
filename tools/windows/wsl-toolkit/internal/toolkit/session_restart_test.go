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
	"strconv"
	"strings"
	"testing"
	"time"
)

// -- WSL-184: session ids are held to the processes their launch recorded ------

// TestALostSessionIsReadAsGoneWithoutAnExit: the state script's LOST line makes a
// session that is not running, with a reason, and nothing else reads as lost.
// WSL-184.
func TestALostSessionIsReadAsGoneWithoutAnExit(t *testing.T) {
	out := "DIR|/h/.wsl-toolkit/sessions/0123456789abcdef\nALIVE|no\nLOST|restart\nEXIT|\nTIMEDOUT|no\nSTOPPED|no\n" +
		"DIR|/h/.wsl-toolkit/sessions/fedcba9876543210\nALIVE|no\nLOST|reused\nEXIT|\n" +
		"DIR|/h/.wsl-toolkit/sessions/00000000000000aa\nALIVE|no\nLOST|somewhere\nEXIT|\n"
	got := parseSessionStates(out)
	restarted := got["/h/.wsl-toolkit/sessions/0123456789abcdef"]
	if restarted.Alive || restarted.Lost != SessionLostRestart || restarted.Exit != nil || restarted.Verdict() != ExitFailed {
		t.Errorf("a session the base restarted under reads as %+v", restarted)
	}
	if !strings.Contains(restarted.LostReason(), "started again") || !strings.Contains(restarted.LostReason(), "without an exit") {
		t.Errorf("a restart reads %q", restarted.LostReason())
	}
	reused := got["/h/.wsl-toolkit/sessions/fedcba9876543210"]
	if reused.Lost != SessionLostReused || !strings.Contains(reused.LostReason(), "another process") {
		t.Errorf("a reused supervisor id reads as %+v, %q", reused, reused.LostReason())
	}
	if odd := got["/h/.wsl-toolkit/sessions/00000000000000aa"]; odd.Lost != "" || odd.LostReason() != "" {
		t.Errorf("a LOST word the script never prints was taken: %+v", odd)
	}
}

// sessionShell runs the session scripts in a real POSIX shell, or skips.
type sessionShell struct {
	sh, home, noScope string
}

func newSessionShell(t *testing.T) sessionShell {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the session scripts are POSIX shell, and this host has none to run them in")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("no setsid on this host")
	}
	if _, err := os.Stat("/proc/sys/kernel/random/boot_id"); err != nil {
		t.Skip("this host has no boot id to hold a session to")
	}
	s := sessionShell{sh: sh, home: t.TempDir(), noScope: t.TempDir()}
	// ⛔ No scope, as the process-group case says: a scope's own signal would
	// end what this case proves the group kill does not reach.
	if err := os.WriteFile(filepath.Join(s.noScope, "systemd-run"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return s
}

// run is one script, bounded: ⛔ a follow that never ends is the defect, so it
// has to fail the case and not hang it.
func (s sessionShell) run(t *testing.T, script []byte, limit time.Duration) (string, string, int, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.sh)
	cmd.Stdin = bytes.NewReader(script)
	cmd.Env = append(os.Environ(), "HOME="+s.home, "PATH="+s.noScope+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ctx.Err() != nil {
		return out.String(), errb.String(), -1, true
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.String(), errb.String(), ee.ExitCode(), false
	}
	if err != nil {
		t.Fatal(err)
	}
	return out.String(), errb.String(), 0, false
}

func (s sessionShell) state(t *testing.T, dir string) SessionState {
	t.Helper()
	out, stderr, code, _ := s.run(t, sessionStateScript(dir), 30*time.Second)
	if code != 0 {
		t.Fatalf("the state script exited %d: %s", code, stderr)
	}
	return parseSessionStates(out)[dir]
}

// bystander is another of the account's processes, in a process group of its
// own, as an agent's would be. Its pid is what a reused id names.
//
// ⚠ setsid makes the group, and the shell that started it has already exited,
// so nothing here is this test's child to wait for.
//
// ⚠ IT OUTLIVES ANY CASE, and the cleanup ends it. One that ended on its own
// under a loaded host read as a signal the case never sent.
func (s sessionShell) bystander(t *testing.T) int {
	t.Helper()
	out, stderr, code, _ := s.run(t, []byte("setsid sleep 600 </dev/null >/dev/null 2>&1 &\necho $!\n"), 30*time.Second)
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if code != 0 || err != nil {
		t.Fatalf("the bystander did not start (exit %d): %q %s", code, out, stderr)
	}
	s.killOnCleanup(t, "-"+strconv.Itoa(pid))
	return pid
}

// killOnCleanup KILLs a process or, with a leading dash, a group, when the case
// ends. ⛔ It sends the signal and waits for nothing.
func (s sessionShell) killOnCleanup(t *testing.T, target string) {
	t.Helper()
	t.Cleanup(func() {
		_ = exec.Command(s.sh, "-c", `kill -KILL "$1" 2>/dev/null; exit 0`, "kill", target).Run()
	})
}

// running says whether a process exists and is not a zombie. ⚠ Not kill -0: a
// process signalled to death and not yet reaped still answers it.
func running(pid int) bool {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	s := string(raw)
	rest := strings.Fields(s[strings.LastIndex(s, ")")+1:])
	return len(rest) > 0 && rest[0] != "Z" && rest[0] != "X"
}

// procStart is field 22 of /proc/PID/stat, read after the name.
func procStart(t *testing.T, pid int) string {
	t.Helper()
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	fields := strings.Fields(s[strings.LastIndex(s, ")")+1:])
	if len(fields) < 20 {
		t.Fatalf("/proc/%d/stat has %d fields after its name", pid, len(fields))
	}
	return fields[19]
}

func readTrim(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

// TestASessionFromAnotherBootIsGoneAndNotSignalled starts a real session, then
// makes its record name another boot, as a restart leaves it. ⛔ Its state says
// gone without an exit, a stop sends nothing, and a follow ends. WSL-184.
func TestASessionFromAnotherBootIsGoneAndNotSignalled(t *testing.T) {
	s := newSessionShell(t)
	dir := filepath.Join(s.home, GuestRoot, "sessions", "0123456789abcdef")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// ⚠ The payload outlives the case, as the bystander does, and the cleanup
	// ends its group.
	if err := os.WriteFile(filepath.Join(dir, "payload.sh"), []byte("sleep 600\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, stderr, code, _ := s.run(t, sessionLaunchScript(dir, "~", 0, time.Second), 30*time.Second)
	if code != 0 || !strings.HasPrefix(out, "SESSION|") {
		t.Fatalf("the session did not start (exit %d): %s %s", code, out, stderr)
	}
	payload, err := strconv.Atoi(readTrim(t, filepath.Join(dir, "payload.pid")))
	if err != nil {
		t.Fatal(err)
	}
	s.killOnCleanup(t, "-"+strconv.Itoa(payload))
	supervisor, err := strconv.Atoi(readTrim(t, filepath.Join(dir, "supervisor.pid")))
	if err != nil {
		t.Fatal(err)
	}
	s.killOnCleanup(t, strconv.Itoa(supervisor))

	// ⭐ THE LAUNCH RECORDS WHAT A REUSED ID CANNOT CARRY: this start of the base,
	// as the kernel's boot id and init's start, and when the supervisor and the
	// payload started.
	boot := readTrim(t, "/proc/sys/kernel/random/boot_id")
	initStart := procStart(t, 1)
	if got, want := readTrim(t, filepath.Join(dir, "boot")), boot+" "+initStart; got != want {
		t.Errorf("the launch recorded the start of the base as %q, and this one is %q", got, want)
	}
	if got, want := readTrim(t, filepath.Join(dir, "supervisor.start")), procStart(t, supervisor); got != want {
		t.Errorf("the launch recorded the supervisor's start as %q, and it started at %q", got, want)
	}
	if got, want := readTrim(t, filepath.Join(dir, "payload.start")), procStart(t, payload); got != want {
		t.Errorf("the launch recorded the payload's start as %q, and it started at %q", got, want)
	}
	if st := s.state(t, dir); !st.Alive || st.Lost != "" {
		t.Fatalf("a session of this boot reads as %+v", st)
	}

	// ⛔ A RESTART OF THE DISTRIBUTION ALONE, the common one: the VM and its boot
	// id stay, and init started again, later.
	n, err := strconv.ParseUint(initStart, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "boot"), []byte(boot+" "+strconv.FormatUint(n+1, 10)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if st := s.state(t, dir); st.Alive || st.Lost != SessionLostRestart || st.Exit != nil {
		t.Fatalf("a session from an earlier start of the distribution, on this boot, reads as %+v", st)
	}
	if err := os.WriteFile(filepath.Join(dir, "boot"), []byte("00000000-0000-0000-0000-000000000000 "+initStart+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if st := s.state(t, dir); st.Alive || st.Lost != SessionLostRestart || st.Exit != nil {
		t.Fatalf("a session from another boot reads as %+v", st)
	}
	out, stderr, code, _ = s.run(t, stopSessionScript(dir, time.Second), 30*time.Second)
	if code != 0 || !strings.Contains(out, "LOST|restart") {
		t.Fatalf("the stop of a session from another boot: exit %d, out %q, stderr %q", code, out, stderr)
	}
	if !running(payload) {
		t.Fatal("the stop of a session from another boot signalled the group its old id names")
	}
	if _, err := os.Stat(filepath.Join(dir, "stopped")); err == nil {
		t.Error("the stop marked a session it never reached as stopped")
	}
	if _, _, code, hung := s.run(t, followSessionScript(dir, 0), 30*time.Second); hung || code != 0 {
		t.Fatalf("following a session from another boot: exit %d, hung %v", code, hung)
	}
}

// TestASessionIdThatNamesAnotherProcessIsNotItsOwn stages what a restart leaves on
// the same boot: the ids name the account's other processes, which started
// later. ⛔ The state is gone without an exit, a stop leaves them, and a follow
// ends. WSL-184.
func TestASessionIdThatNamesAnotherProcessIsNotItsOwn(t *testing.T) {
	s := newSessionShell(t)
	other := s.bystander(t)
	pid := strconv.Itoa(other)
	boot := readTrim(t, "/proc/sys/kernel/random/boot_id") + " " + procStart(t, 1)
	stage := func(name string, files map[string]string) string {
		dir := filepath.Join(s.home, GuestRoot, "sessions", name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for f, body := range files {
			if err := os.WriteFile(filepath.Join(dir, f), []byte(body+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}

	// The supervisor's id names the bystander, which started later than the
	// supervisor the launch recorded.
	reused := stage("00000000000000a1", map[string]string{"boot": boot, "supervisor.pid": pid, "supervisor.start": "1",
		"payload.pid": pid, "payload.start": "1", "stdout": "", "stderr": ""})
	if st := s.state(t, reused); st.Alive || st.Lost != SessionLostReused {
		t.Fatalf("a supervisor id naming another process reads as %+v", st)
	}
	out, stderr, code, _ := s.run(t, stopSessionScript(reused, time.Second), 30*time.Second)
	if code != 0 || !strings.Contains(out, "LOST|reused") || !running(other) {
		t.Fatalf("the stop of a reused supervisor id: exit %d, out %q, stderr %q, bystander running %v", code, out, stderr, running(other))
	}
	if _, _, code, hung := s.run(t, followSessionScript(reused, 0), 30*time.Second); hung || code != 0 {
		t.Fatalf("following a session whose supervisor id names another process: exit %d, hung %v", code, hung)
	}

	// ⛔ THE GROUP IS HELD TO ITS LEADER TOO. The supervisor is gone on this boot,
	// which a stop still answers by signalling the payload's group, and the id
	// that group was named by now leads another process's.
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	group := stage("00000000000000a2", map[string]string{"boot": boot, "supervisor.pid": strconv.Itoa(dead.Process.Pid),
		"supervisor.start": "1", "payload.pid": pid, "payload.start": "1", "stdout": "", "stderr": ""})
	out, stderr, code, _ = s.run(t, stopSessionScript(group, time.Second), 30*time.Second)
	if code != 0 || !strings.Contains(out, "LOST|reused") || !running(other) {
		t.Fatalf("the stop of a group whose leader is another process: exit %d, out %q, stderr %q, bystander running %v", code, out, stderr, running(other))
	}
}
