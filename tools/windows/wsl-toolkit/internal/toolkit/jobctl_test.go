// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// -- WSL-94: one owner per job, and job control from a second process --------

const testJobID = "0123456789abcdef"

func TestAJobHasOneOwnerAndASecondClaimIsRefused(t *testing.T) {
	home := t.TempDir()
	owner, err := ClaimJob(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release()
	st, err := ReadJobState(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Known || !st.Owned || st.Word() != "running" {
		t.Fatalf("a claimed job reads as %+v, word %q", st, st.Word())
	}
	if _, err := ClaimJob(home, testJobID); !errors.Is(err, ErrJobOwned) {
		t.Fatalf("a second claim on a running job answered %v", err)
	}
}

func TestAnOwnersEndIsRecordedAndItsLockReleased(t *testing.T) {
	home := t.TempDir()
	owner, err := ClaimJob(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Finish(JobResult{ID: testJobID, Exit: 3, EffectiveExit: 3}); err != nil {
		t.Fatal(err)
	}
	st, err := ReadJobState(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	if st.Owned || st.Result == nil || st.Result.Exit != 3 || st.Word() != "ended" {
		t.Fatalf("a finished job reads as %+v, word %q", st, st.Word())
	}
	// ⚠ A SECOND RELEASE IS HARMLESS, because the deferred one in Run follows a
	// Finish on every path.
	owner.Release()
	again, err := ClaimJob(home, testJobID)
	if err != nil {
		t.Fatalf("the lock was not let go: %v", err)
	}
	again.Release()
}

func TestAnOwnerThatEndsWithoutSayingLeavesNoOwner(t *testing.T) {
	home := t.TempDir()
	owner, err := ClaimJob(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	owner.Release()
	st, err := ReadJobState(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	if st.Owned || st.Result != nil || st.Word() != "no owner" {
		t.Fatalf("an owner gone without a result reads as %+v, word %q", st, st.Word())
	}
}

func TestEachJobStateHasItsOwnWord(t *testing.T) {
	res := &JobResult{}
	rec := &SessionRecord{}
	for _, c := range []struct {
		st   JobState
		want string
	}{
		{JobState{}, "unknown"},
		{JobState{Known: true}, "no owner"},
		{JobState{Known: true, Owned: true}, "running"},
		{JobState{Known: true, Session: rec}, "detached"},
		{JobState{Known: true, Owned: true, Result: res}, "ended"},
		{JobState{Known: true, Result: res}, "ended"},
	} {
		if got := c.st.Word(); got != c.want {
			t.Errorf("%+v reads as %q, want %q", c.st, got, c.want)
		}
	}
}

func TestAJobIDIsSixteenLowercaseHexCharacters(t *testing.T) {
	for _, ok := range []string{testJobID, "ffffffffffffffff"} {
		if !ValidJobID(ok) {
			t.Errorf("%q is refused", ok)
		}
	}
	for _, bad := range []string{"", "0123456789abcde", "0123456789abcdef0", "0123456789ABCDEF", "../../../../etc/x", "0123456789abcdeg"} {
		if ValidJobID(bad) {
			t.Errorf("%q is accepted", bad)
		}
	}
	t.Setenv(JobIDEnv, "../escape")
	if _, err := PreassignedJobID(); err == nil {
		t.Error("a preassigned id that is not an id is accepted")
	}
	t.Setenv(JobIDEnv, testJobID)
	if id, err := PreassignedJobID(); err != nil || id != testJobID {
		t.Errorf("a preassigned id answered %q, %v", id, err)
	}
	t.Setenv(JobIDEnv, "")
	if id, err := PreassignedJobID(); err != nil || id != "" {
		t.Errorf("no preassigned id answered %q, %v", id, err)
	}
}

func TestOnlyTheEnginesTermOrKillIsAStop(t *testing.T) {
	for code, want := range map[int]bool{143: true, 137: true, 130: false, 0: false, 1: false, 124: false} {
		if got := stoppedBySignal(code); got != want {
			t.Errorf("exit %d read as a stop: %v, want %v", code, got, want)
		}
	}
}

func TestTheStopScriptsAnswerIsRead(t *testing.T) {
	for _, c := range []struct {
		out     string
		before  string
		after   string
		exit    int // -1 for none
		stopped bool
	}{
		{"BEFORE|running\r\nAFTER|exited|143\r\n", "running", "exited", 143, true},
		{"BEFORE|created\nAFTER|exited|0\n", "created", "exited", 0, true},
		{"BEFORE|exited\nAFTER|exited|7\n", "exited", "exited", 7, false},
		{"BEFORE|running\nAFTER|gone|\n", "running", "gone", -1, true},
		{"BEFORE|gone\n", "gone", "", -1, false},
	} {
		var rep StopReport
		readStopOutput(&rep, c.out)
		exit := -1
		if rep.Exit != nil {
			exit = *rep.Exit
		}
		if rep.Before != c.before || rep.After != c.after || exit != c.exit || rep.Stopped != c.stopped {
			t.Errorf("%q read as before=%q after=%q exit=%d stopped=%v", c.out, rep.Before, rep.After, exit, rep.Stopped)
		}
	}
}

func TestAStopIsFoundInTheLedger(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	led, err := OpenLedger()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{ledger: led}
	if r.stopRequested(testJobID) {
		t.Fatal("an empty ledger holds a stop")
	}
	for _, e := range []LedgerEntry{
		{Event: "open", Kind: "job", ID: testJobID},
		{Event: "stop", Kind: "job", ID: "fedcba9876543210"},
	} {
		if err := led.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	if r.stopRequested(testJobID) {
		t.Fatal("another job's stop was read as this one's")
	}
	if err := led.Append(LedgerEntry{Event: "stop", Kind: "job", ID: testJobID}); err != nil {
		t.Fatal(err)
	}
	if !r.stopRequested(testJobID) {
		t.Fatal("the stop in the ledger was not found")
	}
	// ⭐ THE ONE RULE both the owner and `wait` read: a stop's signal, and a stop.
	for exit, want := range map[int]bool{143: true, 137: true, 0: false, 3: false, 130: false} {
		if got := r.EndedByStop(testJobID, exit); got != want {
			t.Errorf("exit %d with a stop recorded read as stopped: %v, want %v", exit, got, want)
		}
	}
	if r.EndedByStop("00000000000000aa", 143) {
		t.Error("exit 143 for a job with no stop of its own read as stopped")
	}
}

// followed is one follower running in its own goroutine. Its buffers are read
// only after done answers.
type followed struct {
	out, err bytes.Buffer
	done     chan error
}

func follow(ctx context.Context, dir string, tail int) *followed {
	f := &followed{done: make(chan error, 1)}
	go func() { f.done <- FollowTranscript(ctx, dir, tail, &f.out, &f.err) }()
	return f
}

func appendFile(t *testing.T, path, text string) {
	t.Helper()
	h, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.WriteString(text); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFollowingATranscriptReadsUntilTheOwnerLetsGo(t *testing.T) {
	home := t.TempDir()
	owner, err := ClaimJob(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "jobs", testJobID)
	appendFile(t, filepath.Join(dir, "stdout.log"), "one\n")
	f := follow(context.Background(), dir, 0)
	time.Sleep(2 * FollowPoll)
	appendFile(t, filepath.Join(dir, "stdout.log"), "two\n")
	appendFile(t, filepath.Join(dir, "stderr.log"), "err\n")
	select {
	case err := <-f.done:
		t.Fatalf("the follower returned while the owner held the job: %v", err)
	case <-time.After(3 * FollowPoll):
	}
	// ⛔ THE LAST BYTES ARE WRITTEN AS THE OWNER LETS GO, and a follower that
	// stopped on the release without one more read would lose them. The lock is
	// released at once after the write, so no poll can fall between the two.
	appendFile(t, filepath.Join(dir, "stdout.log"), "last\n")
	owner.Release()
	select {
	case err := <-f.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the follower did not return once the owner let go")
	}
	if got := f.out.String(); got != "one\ntwo\nlast\n" {
		t.Errorf("stdout followed as %q", got)
	}
	if got := f.err.String(); got != "err\n" {
		t.Errorf("stderr followed as %q", got)
	}
}

func TestFollowingFromTheTailStartsAtTheLastLines(t *testing.T) {
	home := t.TempDir()
	owner, err := ClaimJob(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "jobs", testJobID)
	appendFile(t, filepath.Join(dir, "stdout.log"), "1\n2\n3\n")
	f := follow(context.Background(), dir, 2)
	time.Sleep(2 * FollowPoll)
	appendFile(t, filepath.Join(dir, "stdout.log"), "4\n")
	owner.Release()
	if err := <-f.done; err != nil {
		t.Fatal(err)
	}
	if got := f.out.String(); got != "2\n3\n4\n" {
		t.Errorf("the last two lines and what followed came back as %q", got)
	}
}

func TestAFollowerStopsWhenItIsCancelled(t *testing.T) {
	home := t.TempDir()
	owner, err := ClaimJob(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release()
	ctx, cancel := context.WithCancel(context.Background())
	f := follow(ctx, filepath.Join(home, "jobs", testJobID), 0)
	cancel()
	select {
	case err := <-f.done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("a cancelled follower answered %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled follower kept following")
	}
}

func TestTheMarkerFilterDropsOnlyTheWrappersTwoLines(t *testing.T) {
	token := "wtk-started-" + strings.Repeat("ab", 16)
	for _, c := range []struct {
		name   string
		writes []string
		want   string
	}{
		{"the marker then output", []string{"\n" + token + "\nhello\n"}, "hello\n"},
		{"the marker in pieces", []string{"\nwtk-sta", "rted-" + strings.Repeat("ab", 16), "\nhello\n"}, "hello\n"},
		{"no marker", []string{"hello\n"}, "hello\n"},
		{"a line that only looks like one", []string{"\nwtk-started-short\nx\n"}, "\nwtk-started-short\nx\n"},
		{"a stream that ends inside the marker", []string{"\nwtk-sta"}, "\nwtk-sta"},
		{"a marker later in the stream is output", []string{"a\n", "\n" + token + "\n"}, "a\n\n" + token + "\n"},
	} {
		var out bytes.Buffer
		m := &markerFilter{to: &out}
		for _, w := range c.writes {
			if _, err := m.Write([]byte(w)); err != nil {
				t.Fatal(err)
			}
		}
		if err := m.Flush(); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// TestTheContainerAnnouncesItselfOnce proves the hook the late stop hangs on:
// it runs once, when the token first arrives, however the token is split.
func TestTheContainerAnnouncesItselfOnce(t *testing.T) {
	var out bytes.Buffer
	m := newMarkerStripper(&out, "tok")
	seen := 0
	m.onSeen = func() { seen++ }
	for _, w := range []string{"before\n\nt", "ok\nafter\n", "\ntok\n"} {
		if _, err := m.Write([]byte(w)); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Flush(); err != nil {
		t.Fatal(err)
	}
	if seen != 1 {
		t.Fatalf("the container announced itself %d time(s)", seen)
	}
	if got := out.String(); got != "before\nafter\n" {
		t.Errorf("the stream came through as %q", got)
	}
}

// TestAnOpenRecordWhoseOwnerIsGoneIsNotLiveWork: the owner lock decides where
// it exists, and the record alone decides where it does not.
func TestAnOpenRecordWhoseOwnerIsGoneIsNotLiveWork(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	led, err := OpenLedger()
	if err != nil {
		t.Fatal(err)
	}
	running, gone, lockless := testJobID, "fedcba9876543210", "00000000000000aa"
	for _, id := range []string{running, gone, lockless} {
		if err := led.Append(LedgerEntry{Event: "open", Kind: "job", ID: id, Deadline: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	held, err := ClaimJob(home, running)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	killed, err := ClaimJob(home, gone)
	if err != nil {
		t.Fatal(err)
	}
	killed.Release()
	r := &Runner{home: home, ledger: led}
	ids, err := r.openJobIDs()
	if err != nil {
		t.Fatal(err)
	}
	if !ids[running] || ids[gone] || !ids[lockless] {
		t.Fatalf("open work read as %v: want %s and %s, and not %s", ids, running, lockless, gone)
	}
}

func TestATargetWhoseStateIsUnknownIsKeptEvenUnderIncludeLive(t *testing.T) {
	targets := []CleanupTarget{
		{Kind: "session", Name: "a", JobID: testJobID, Unknown: "the guest could not be asked"},
		{Kind: "session", Name: "b", JobID: "fedcba9876543210", Live: true},
	}
	remove, spared := CleanupPolicy{IncludeLive: true}.Select(targets, time.Now())
	if len(remove) != 1 || remove[0].Name != "b" {
		t.Fatalf("--include-live removed %+v", remove)
	}
	if len(spared) != 1 || spared[0].Target.Name != "a" || !strings.Contains(spared[0].Reason, "could not be read") {
		t.Fatalf("the unreadable session was not kept with its reason: %+v", spared)
	}
}
