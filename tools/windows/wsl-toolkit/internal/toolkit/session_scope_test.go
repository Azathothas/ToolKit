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
	"time"
)

// -- WSL-126: a stop reaches a whole session -----------------------------------

// TestASessionAsksForItsOwnScopeOnlyAfterAProbeScopeStarted holds the shape of
// the scripts on every host, because the real shell below has none on Windows.
func TestASessionAsksForItsOwnScopeOnlyAfterAProbeScopeStarted(t *testing.T) {
	launch := string(sessionLaunchScript("/home/toolkit/.wsl-toolkit/sessions/0123456789abcdef", "~", time.Minute, time.Second))
	for _, want := range []string{
		"systemd-run --user --scope --quiet --collect -- true",
		`u="wtk-session-${d##*/}.scope"`,
		"if [ -n \"$u\" ]; then\n  setsid systemd-run --user --scope --unit=\"$u\" --quiet --collect -- /bin/sh \"$d/payload.sh\"",
		"else\n  setsid /bin/sh \"$d/payload.sh\"",
		`if [ -n "$u" ]; then systemctl --user kill --signal=TERM "$u" >/dev/null 2>&1; fi`,
		`if [ -n "$u" ]; then systemctl --user kill --signal=KILL "$u" >/dev/null 2>&1; fi`,
		`if [ -n "$u" ]; then printf 'UNIT|%s\n' "$u"; fi`,
		// ⛔ Past a deadline in a scope the watchdog is left to KILL what ignored
		// the TERM; a process group keeps the behaviour it had.
		`if [ -n "$w" ] && { [ -z "$u" ] || [ ! -e "$d/timedout" ]; }; then kill "$w" 2>/dev/null; fi`,
	} {
		if !strings.Contains(launch, want) {
			t.Errorf("the launch script does not carry %q:\n%s", want, launch)
		}
	}
	probe := strings.Index(launch, "systemd-run --user --scope --quiet --collect -- true")
	start := strings.Index(launch, "setsid /bin/sh -c '")
	if probe < 0 || start < 0 || probe > start {
		t.Error("the scope is not probed before the supervisor starts")
	}
	stop := string(stopSessionScript("/home/toolkit/.wsl-toolkit/sessions/0123456789abcdef", 3*time.Second))
	for _, want := range []string{
		`case "$u" in wtk-session-*.scope) ;; *) u= ;; esac`,
		"if [ -n \"$u\" ]; then systemctl --user kill --signal=TERM \"$u\" >/dev/null 2>&1; fi\nkill -TERM \"-$pg\"",
		`{ [ ! -e "$d/exit" ] || live; }`,
		`if live; then systemctl --user kill --signal=KILL "$u" >/dev/null 2>&1; fi`,
	} {
		if !strings.Contains(stop, want) {
			t.Errorf("the stop script does not carry %q:\n%s", want, stop)
		}
	}
}

// TestASessionRecordsOnlyItsOwnScope: the unit a record names is the one name
// a session's scope can have, read from the launch and on every read after.
func TestASessionRecordsOnlyItsOwnScope(t *testing.T) {
	const id = "0123456789abcdef"
	if got := launchedUnit("SESSION|42\nUNIT|wtk-session-0123456789abcdef.scope\n", id); got != "wtk-session-0123456789abcdef.scope" {
		t.Errorf("the session's own scope read as %q", got)
	}
	for _, out := range []string{
		"SESSION|42\n",
		"SESSION|42\nUNIT|wtk-session-fedcba9876543210.scope\n",
		"SESSION|42\nUNIT|user.slice\n",
	} {
		if got := launchedUnit(out, id); got != "" {
			t.Errorf("%q recorded the unit %q", out, got)
		}
	}
	rec := SessionRecord{ID: id, Distro: "wsl-toolkit", User: "toolkit", GuestDir: "/home/toolkit/" + GuestRoot + "/sessions/" + id}
	if err := checkSessionRecord(id, rec); err != nil || rec.Containment() != ContainedByGroup {
		t.Fatalf("a record with no unit: %v, %s", err, rec.Containment())
	}
	rec.Unit = sessionUnit(id)
	if err := checkSessionRecord(id, rec); err != nil || rec.Containment() != ContainedByScope {
		t.Fatalf("a record naming its own scope: %v, %s", err, rec.Containment())
	}
	rec.Unit = "user@1000.service"
	if err := checkSessionRecord(id, rec); err == nil {
		t.Fatal("a record naming another unit was accepted")
	}
}

func TestWhatAnEndedSessionLeftIsReadFromOneLine(t *testing.T) {
	for out, want := range map[string][2]bool{
		"LEFT|none\n":    {false, false},
		"LEFT|ended\n":   {true, false},
		"LEFT|running\n": {true, true},
	} {
		held, still, err := parseLeftovers(out)
		if err != nil || held != want[0] || still != want[1] {
			t.Errorf("%q read as held %v, still %v, %v", out, held, still, err)
		}
	}
	if _, _, err := parseLeftovers("nothing\n"); err == nil {
		t.Error("an answer with no LEFT line was read as one")
	}
	if held, _, err := StopSessionLeftovers(context.Background(), nil, SessionRecord{}, time.Second); held || err != nil {
		t.Errorf("a session with no scope asked the guest: %v %v", held, err)
	}
}

// TestASessionInAScopeIsReachedWholeInARealShell runs the session scripts with
// a systemd-run and a systemctl that stand in for the user manager: the first
// marks every process the scope starts, and the second signals each process so
// marked, which is what a scope's cgroup does for a real one.
//
// ⛔ THE CHILD THAT CALLED setsid IS WHAT THIS CASE IS ABOUT. It leaves the
// payload's process group, so the group alone does not reach it, and the last
// part shows that without a scope it is still running.
func TestASessionInAScopeIsReachedWholeInARealShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the session scripts are POSIX shell, and this host has none to run them in")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	for _, tool := range []string{"setsid", "tr", "grep"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool + " on this host")
		}
	}
	if _, err := os.Stat("/proc/self/environ"); err != nil {
		t.Skip("no /proc on this host")
	}
	home := t.TempDir()
	fake := func(name, body string) string {
		dir := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		run := "#!/bin/sh\n" + body
		if err := os.WriteFile(filepath.Join(dir, "systemd-run"), []byte(run), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(fakeSystemctl), 0o755); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	scoped := fake("scoped", fakeSystemdRun)
	refused := fake("refused", "exit 1\n")
	run := func(bin string, script []byte) (string, string, int) {
		cmd := exec.Command(sh)
		cmd.Stdin = bytes.NewReader(script)
		cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+":"+os.Getenv("PATH"))
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		err := cmd.Run()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out.String(), errb.String(), ee.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return out.String(), errb.String(), 0
	}
	start := func(bin, name, payload string, timeout, grace time.Duration) (string, string) {
		dir := filepath.Join(home, GuestRoot, "sessions", name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "payload.sh"), []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		out, stderr, code := run(bin, sessionLaunchScript(dir, "~", timeout, grace))
		if code != 0 || !strings.HasPrefix(out, "SESSION|") {
			t.Fatalf("%s did not start (exit %d): %s %s", name, code, out, stderr)
		}
		return dir, out
	}
	pidOf := func(name string) string {
		for i := 0; i < 100; i++ {
			if raw, err := os.ReadFile(filepath.Join(home, name+".pid")); err == nil && len(bytes.TrimSpace(raw)) > 0 {
				return strings.TrimSpace(string(raw))
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("%s never wrote its pid", name)
		return ""
	}
	gone := func(pid string, limit time.Duration) bool {
		deadline := time.Now().Add(limit)
		for time.Now().Before(deadline) {
			if _, _, code := run("/nonexistent", []byte("kill -0 "+pid+" 2>/dev/null\n")); code != 0 {
				return true
			}
			time.Sleep(100 * time.Millisecond)
		}
		return false
	}
	state := func(dir string) SessionState {
		out, stderr, code := run("/nonexistent", sessionStateScript(dir))
		if code != 0 {
			t.Fatalf("the state script exited %d: %s", code, stderr)
		}
		return parseSessionStates(out)[dir]
	}
	escaping := func(name, trap string) string {
		return "setsid sh -c '" + trap + "echo $$ > " + name + ".pid; exec sleep 30' </dev/null >/dev/null 2>&1 &\nsleep 30\n"
	}

	// A stop reaches the child that left the group.
	dir, out := start(scoped, "scoped", escaping("escaped", ""), 0, time.Second)
	if !strings.Contains(out, "UNIT|wtk-session-scoped.scope") {
		t.Fatalf("a session whose user manager answered printed no unit: %q", out)
	}
	child := pidOf("escaped")
	if _, stderr, code := run(scoped, stopSessionScript(dir, time.Second)); code != 0 {
		t.Fatalf("the stop script exited %d: %s", code, stderr)
	}
	if st := state(dir); !st.Stopped || st.Verdict() != 130 {
		t.Fatalf("a stopped session reads as %+v", st)
	}
	if !gone(child, 5*time.Second) {
		_, _, _ = run("/nonexistent", []byte("kill -KILL "+child+"\n"))
		t.Fatal("the stop ended the session and left the child that called setsid running")
	}

	// The deadline reaches a child that left the group and ignores TERM.
	dir, _ = start(scoped, "deadline", escaping("ignoring", `trap "" TERM; `), time.Second, time.Second)
	child = pidOf("ignoring")
	if !gone(child, 10*time.Second) {
		_, _, _ = run("/nonexistent", []byte("kill -KILL "+child+"\n"))
		t.Fatal("the deadline ended the session and left the child that ignores TERM running")
	}
	if st := state(dir); !st.TimedOut || st.Verdict() != ExitTimeout {
		t.Fatalf("a session past its deadline reads as %+v", st)
	}

	// What an ended session left in its scope is ended by the leftovers stop.
	dir, _ = start(scoped, "left", "setsid sh -c 'echo $$ > left.pid; exec sleep 30' </dev/null >/dev/null 2>&1 &\n", 0, time.Second)
	child = pidOf("left")
	for i := 0; i < 100 && state(dir).Exit == nil; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	out, stderr, code := run(scoped, sessionLeftoversScript(dir, time.Second))
	if code != 0 || !strings.Contains(out, "LEFT|ended") {
		t.Fatalf("the leftovers script answered %q %q, exit %d", out, stderr, code)
	}
	if !gone(child, 5*time.Second) {
		_, _, _ = run("/nonexistent", []byte("kill -KILL "+child+"\n"))
		t.Fatal("the leftovers stop said ended and the child is running")
	}

	// ⚠ WITHOUT A SCOPE THE GROUP IS ALL THERE IS, and the child that left it
	// outlives the stop. That is the behaviour the answer names.
	dir, out = start(refused, "grouped", escaping("outlived", ""), 0, time.Second)
	if strings.Contains(out, "UNIT|") {
		t.Fatalf("a session whose probe scope failed printed a unit: %q", out)
	}
	child = pidOf("outlived")
	if _, stderr, code := run(refused, stopSessionScript(dir, time.Second)); code != 0 {
		t.Fatalf("the stop script exited %d: %s", code, stderr)
	}
	survived := !gone(child, time.Second)
	_, _, _ = run("/nonexistent", []byte("kill -KILL "+child+" 2>/dev/null\n"))
	if !survived {
		t.Fatal("a process group stop reached a child that left the group, so this case cannot tell the two apart")
	}
}

// fakeSystemdRun stands in for `systemd-run --user --scope`: it marks what it
// starts with the unit's name, and a probe with no unit succeeds.
const fakeSystemdRun = `unit=probe
while [ $# -gt 0 ]; do
  case $1 in
    --unit=*) unit=${1#--unit=} ;;
    --) shift; break ;;
  esac
  shift
done
FAKE_UNIT=$unit; export FAKE_UNIT
exec "$@"
`

// fakeSystemctl stands in for `systemctl --user kill` and `is-active`: it finds
// the processes the fake systemd-run marked.
const fakeSystemctl = `#!/bin/sh
op=; sig=TERM; unit=
for a in "$@"; do
  case $a in
    --user|--quiet) ;;
    --signal=*) sig=${a#--signal=} ;;
    kill|is-active) op=$a ;;
    *) unit=$a ;;
  esac
done
pids=
for e in /proc/[0-9]*/environ; do
  p=${e#/proc/}; p=${p%/environ}
  [ "$p" = "$$" ] && continue
  if tr '\0' '\n' < "$e" 2>/dev/null | grep -qx "FAKE_UNIT=$unit"; then pids="$pids $p"; fi
done
case $op in
  kill) if [ -n "$pids" ]; then kill -s "$sig" $pids 2>/dev/null; fi; exit 0 ;;
  is-active) [ -n "$pids" ] ;;
esac
`
