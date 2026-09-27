// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// -- WSL-95: a base session that outlives its client --------------------------

func TestManySessionsAreReadFromOneAnswer(t *testing.T) {
	out := "DIR|/home/toolkit/.wsl-toolkit/sessions/0123456789abcdef\r\n" +
		"ALIVE|no\nEXIT|4\nTIMEDOUT|no\nSTOPPED|no\n" +
		"STARTED|2026-01-02T03:04:05Z\nENDED|2026-01-02T03:04:17Z\nBYTES|   39|6\n" +
		"DIR|/home/toolkit/.wsl-toolkit/sessions/fedcba9876543210\nGONE|yes\n" +
		"DIR|/root/.wsl-toolkit/sessions/00000000000000aa\nALIVE|yes\nEXIT|\nTIMEDOUT|no\nSTOPPED|no\nBYTES||\n"
	got := parseSessionStates(out)
	if len(got) != 3 {
		t.Fatalf("three sections read as %d: %+v", len(got), got)
	}
	ended := got["/home/toolkit/.wsl-toolkit/sessions/0123456789abcdef"]
	if ended.Alive || ended.Exit == nil || *ended.Exit != 4 || ended.Stdout != 39 || ended.Stderr != 6 || ended.Ended != "2026-01-02T03:04:17Z" {
		t.Errorf("the ended session read as %+v", ended)
	}
	if gone := got["/home/toolkit/.wsl-toolkit/sessions/fedcba9876543210"]; !gone.Gone || gone.Alive {
		t.Errorf("the gone session read as %+v", gone)
	}
	if live := got["/root/.wsl-toolkit/sessions/00000000000000aa"]; !live.Alive || live.Exit != nil {
		t.Errorf("the running session read as %+v", live)
	}
}

// TestGCAndResourcesReadOneSurvey: the same survey row is a cleanup target and
// a held session, and the two cannot disagree about whether it runs.
func TestGCAndResourcesReadOneSurvey(t *testing.T) {
	id := "0123456789abcdef"
	rec := SessionRecord{ID: id, Distro: "wsl-toolkit", User: "toolkit", GuestDir: "/home/user/" + GuestRoot + "/sessions/" + id,
		Started: time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)}
	four := 4
	for _, c := range []struct {
		name  string
		s     sessionSurvey
		state string
		live  bool
		kept  bool
	}{
		{"running", sessionSurvey{rec: rec, id: id, st: SessionState{Alive: true}}, "running", true, false},
		{"ended", sessionSurvey{rec: rec, id: id, st: SessionState{Exit: &four, Ended: "2000-01-02T03:05:00Z"}}, "ended", false, false},
		{"gone with its distribution", sessionSurvey{rec: rec, id: id, st: SessionState{Gone: true}}, "gone", false, false},
		{"not readable", sessionSurvey{rec: rec, id: id, unknown: "the guest could not be asked"}, "unknown", false, true},
		{"a record that does not read", sessionSurvey{id: id, unknown: "does not parse"}, "unknown", false, true},
	} {
		h, tg := c.s.held(), c.s.target()
		if h.State != c.state || tg.Live != c.live || (tg.Unknown != "") != c.kept || tg.JobID != id {
			t.Errorf("%s: held %+v, target %+v", c.name, h, tg)
		}
		if c.name == "ended" {
			if h.Exit == nil || *h.Exit != 4 {
				t.Errorf("an ended session holds exit %v", h.Exit)
			}
			if !tg.ModTime.Equal(time.Date(2000, 1, 2, 3, 5, 0, 0, time.UTC)) {
				t.Errorf("an ended session is aged from %s, not from its end", tg.ModTime)
			}
		}
	}
}

func TestASessionKeepsTwoStreamsAndReadsThemInOrder(t *testing.T) {
	s, err := sessionStreamsScript("/home/user/"+GuestRoot+"/sessions/0123456789abcdef", []string{"stderr", "stdout"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	body := string(s)
	errAt, outAt := strings.Index(body, `tail -n 3 "$d/stderr"`), strings.Index(body, `tail -n 3 "$d/stdout"`)
	if errAt < 0 || outAt < 0 || errAt > outAt {
		t.Fatalf("the streams are not read in the order asked:\n%s", body)
	}
	for _, bad := range []string{"exit", "payload.sh", "../stdout"} {
		if _, err := sessionStreamsScript("/d", []string{bad}, 0); err == nil {
			t.Errorf("%q is read as a stream", bad)
		}
	}
}

func TestASessionEndsOnTheRulesAJobKeeps(t *testing.T) {
	four := 4
	killed := 137
	for _, c := range []struct {
		st   SessionState
		want int
	}{
		{SessionState{Exit: &four}, 4},
		{SessionState{Exit: &killed, TimedOut: true}, ExitTimeout},
		{SessionState{Exit: &killed, Stopped: true}, 130},
		{SessionState{}, ExitFailed},
	} {
		if got := c.st.Verdict(); got != c.want {
			t.Errorf("%+v answered %d, want %d", c.st, got, c.want)
		}
	}
}

func TestASessionRecordNamesOnlyItsOwnDirectory(t *testing.T) {
	id := "0123456789abcdef"
	good := SessionRecord{Schema: SessionSchema, ID: id, Distro: "wsl-toolkit", User: "toolkit",
		GuestDir: "/home/toolkit/" + GuestRoot + "/sessions/" + id}
	if err := checkSessionRecord(id, good); err != nil {
		t.Fatalf("a record in the one layout is refused: %v", err)
	}
	for name, mutate := range map[string]func(*SessionRecord){
		"another id":          func(r *SessionRecord) { r.ID = "fedcba9876543210" },
		"the account's home":  func(r *SessionRecord) { r.GuestDir = "/home/toolkit" },
		"another session":     func(r *SessionRecord) { r.GuestDir = "/home/toolkit/" + GuestRoot + "/sessions/fedcba9876543210" },
		"a climb":             func(r *SessionRecord) { r.GuestDir = "/home/toolkit/../../" + GuestRoot + "/sessions/" + id },
		"a relative path":     func(r *SessionRecord) { r.GuestDir = GuestRoot + "/sessions/" + id },
		"no home in front":    func(r *SessionRecord) { r.GuestDir = "/" + GuestRoot + "/sessions/" + id },
		"a newline":           func(r *SessionRecord) { r.GuestDir = "/x\n/" + GuestRoot + "/sessions/" + id },
		"no account":          func(r *SessionRecord) { r.User = "" },
		"no distribution":     func(r *SessionRecord) { r.Distro = "" },
		"a dot segment":       func(r *SessionRecord) { r.GuestDir = "/home/./toolkit/" + GuestRoot + "/sessions/" + id },
		"the sessions parent": func(r *SessionRecord) { r.GuestDir = "/home/toolkit/" + GuestRoot + "/sessions" },
	} {
		rec := good
		mutate(&rec)
		if err := checkSessionRecord(id, rec); err == nil {
			t.Errorf("%s: %q is accepted", name, rec.GuestDir)
		}
	}
}

func TestADamagedSessionRecordIsRefusedOnRead(t *testing.T) {
	home := t.TempDir()
	id := "0123456789abcdef"
	if err := writeSessionRecord(home, SessionRecord{Schema: SessionSchema, ID: id, Distro: "d", User: "user", GuestDir: "/home/user"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := ReadSessionRecord(home, id); err == nil || ok {
		t.Fatalf("a record naming the account's home read as %v, %v", ok, err)
	}
	if got := sessionsRoot(SessionRecord{ID: id, GuestDir: "/home/user/" + GuestRoot + "/sessions/" + id}); got != "/home/user/"+GuestRoot+"/sessions" {
		t.Errorf("the containment root is %q", got)
	}
}

func TestASessionsPayloadIsItsEnvironmentThenItsScript(t *testing.T) {
	script := []byte("echo \"$B\"\n")
	req := ExecRequest{Env: map[string]string{"B": "two words", "A": "it's"}, Script: script}
	got := string(SessionPayload(req))
	want := `A='it'\''s'; export A` + "\n" + `B='two words'; export B` + "\n" + `echo "$B"` + "\n"
	if got != want {
		t.Fatalf("the payload is\n%q\nwant\n%q", got, want)
	}
	if string(req.Script) != "echo \"$B\"\n" {
		t.Fatalf("building the payload changed the caller's script to %q", req.Script)
	}
}

func TestTheSupervisorIsGivenWholeSecondsAndItsDirectory(t *testing.T) {
	for _, c := range []struct {
		dir            string
		timeout, grace time.Duration
		args, cd       string
	}{
		{"~", 0, 0, " 0 1 ", `cd "$HOME"`},
		{"~", 400 * time.Millisecond, 10 * time.Second, " 1 10 ", `cd "$HOME"`},
		{"/work dir", 1500 * time.Millisecond, 2 * time.Second, " 2 2 ", `cd '/work dir'`},
		{"", 90 * time.Second, 0, " 90 1 ", `cd "$HOME"`},
	} {
		s := string(sessionLaunchScript("/home/user/.wsl-toolkit/sessions/0123456789abcdef", c.dir, c.timeout, c.grace))
		if !strings.Contains(s, "wtk-session \"$d\""+c.args) {
			t.Errorf("timeout %s, grace %s: the supervisor is not given%q", c.timeout, c.grace, c.args)
		}
		if !strings.Contains(s, c.cd+" ||") {
			t.Errorf("dir %q: no %s", c.dir, c.cd)
		}
	}
}

// TestASessionRunsStopsTimesOutAndIsFollowedInARealShell runs the four scripts
// a session is made of, where this host has a POSIX shell and setsid.
func TestASessionRunsStopsTimesOutAndIsFollowedInARealShell(t *testing.T) {
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
	home := t.TempDir()
	run := func(script []byte) (string, string, int) {
		cmd := exec.Command(sh)
		cmd.Stdin = bytes.NewReader(script)
		cmd.Env = append(os.Environ(), "HOME="+home)
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
	start := func(name, payload string, timeout, grace time.Duration) string {
		dir := filepath.Join(home, GuestRoot, "sessions", name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "payload.sh"), []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		out, stderr, code := run(sessionLaunchScript(dir, "~", timeout, grace))
		if code != 0 || !strings.HasPrefix(out, "SESSION|") {
			t.Fatalf("%s did not start (exit %d): %s %s", name, code, out, stderr)
		}
		return dir
	}
	state := func(dir string) SessionState {
		out, stderr, code := run(sessionStateScript(dir))
		if code != 0 {
			t.Fatalf("the state script exited %d: %s", code, stderr)
		}
		return parseSessionStates(out)[dir]
	}
	waitEnd := func(dir string, limit time.Duration) SessionState {
		deadline := time.Now().Add(limit)
		for {
			st := state(dir)
			if st.Exit != nil || time.Now().After(deadline) {
				return st
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	done := start("ends", "pwd\necho to-err >&2\nexit 4\n", 0, time.Second)
	if st := waitEnd(done, 10*time.Second); st.Alive || st.Verdict() != 4 {
		t.Fatalf("a payload that exits 4 ended as %+v", st)
	}
	if out, errs, code := run(followSessionScript(done, 0)); code != 0 || out != home+"\n" || errs != "to-err\n" {
		t.Errorf("following an ended session wrote %q and %q, exit %d", out, errs, code)
	}
	both, err := sessionStreamsScript(done, []string{"stdout", "stderr"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out, _, code := run(both); code != 0 || out != home+"\nto-err\n" {
		t.Errorf("reading both streams wrote %q, exit %d", out, code)
	}
	if _, err := sessionStreamsScript(done, []string{"exit"}, 0); err == nil {
		t.Error("a stream that is not stdout or stderr is accepted")
	}

	// The payload leaves a child in the background. ⛔ A stop that ends only the
	// payload's shell ends the session and leaves the child running, so the
	// child is what says the whole group was reached.
	stopped := start("stopped", "sleep 30 &\necho $! > child.pid\nsleep 30\n", 0, time.Second)
	if _, stderr, code := run(stopSessionScript(stopped, time.Second)); code != 0 {
		t.Fatalf("the stop script exited %d: %s", code, stderr)
	}
	if st := state(stopped); st.Alive || !st.Stopped || st.Verdict() != 130 {
		t.Fatalf("a stopped session reads as %+v", st)
	}
	raw, err := os.ReadFile(filepath.Join(home, "child.pid"))
	if err != nil {
		t.Fatal(err)
	}
	child := strings.TrimSpace(string(raw))
	gone := false
	for i := 0; i < 40 && !gone; i++ {
		_, _, code := run([]byte("kill -0 " + child + " 2>/dev/null\n"))
		gone = code != 0
		if !gone {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if !gone {
		_, _, _ = run([]byte("kill -KILL " + child + " 2>/dev/null\n"))
		t.Fatalf("the stop ended the session and left its child %s running", child)
	}

	late := start("deadline", "trap '' TERM\nsleep 30\n", time.Second, time.Second)
	if st := waitEnd(late, 15*time.Second); !st.TimedOut || st.Verdict() != ExitTimeout {
		t.Fatalf("a session past its deadline reads as %+v", st)
	}
}
