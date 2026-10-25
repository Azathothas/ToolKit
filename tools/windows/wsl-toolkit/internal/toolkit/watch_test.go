// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// -- WSL-121: --cancel-on-exit ties a job to the process that asked for it ----

// fakeWatch answers gone once it has been asked after times.
type fakeWatch struct {
	gone  *ProcessIdentity
	after int
	asked int
}

func (f *fakeWatch) Exited(time.Duration) (*ProcessIdentity, error) {
	f.asked++
	if f.asked > f.after {
		return f.gone, nil
	}
	return nil, nil
}

func (f *fakeWatch) Close() {}

var (
	aCaller = ProcessIdentity{PID: 42, Start: 1, Name: "bash.exe", Role: RoleCaller}
	anOwner = ProcessIdentity{PID: 43, Start: 1, Name: "wsl-toolkit.exe", Role: RoleOwner}
)

// stopsRecorded is a Stop that remembers what it was asked, and runs then.
type stopsRecorded struct {
	mu    sync.Mutex
	ids   []string
	why   []string
	found bool
	then  func(id string)
}

func (s *stopsRecorded) stop(_ context.Context, id, why string) (bool, error) {
	s.mu.Lock()
	s.ids, s.why = append(s.ids, id), append(s.why, why)
	s.mu.Unlock()
	if s.then != nil {
		s.then(id)
	}
	return s.found, nil
}

// ⭐ A caller that exits first stops the job, and an owner lost after it leaves
// an end the watcher records: 130, stopped, saying which process exited.
func TestACallerThatExitsFirstStopsTheJob(t *testing.T) {
	home := t.TempDir()
	led := &Ledger{path: home + string(os.PathSeparator) + "ledger.jsonl"}
	if err := led.Append(LedgerEntry{Event: "open", Kind: "job", ID: testJobID, Image: "docker.io/library/alpine:latest"}); err != nil {
		t.Fatal(err)
	}
	owner, err := ClaimJob(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	stops := &stopsRecorded{found: true, then: func(string) { owner.Release() }}
	w := Watchdog{Home: home, IDs: []string{testJobID}, Watch: &fakeWatch{gone: &aCaller}, Stop: stops.stop, Settle: 5 * time.Second, Poll: time.Millisecond}
	out, err := w.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Exited == nil || out.Exited.PID != 42 || len(stops.ids) != 1 || !strings.Contains(stops.why[0], "its caller, process 42 (bash.exe) exited") {
		t.Fatalf("the watch answered %+v and stopped %v for %q", out, stops.ids, stops.why)
	}
	if len(out.Recorded) != 1 {
		t.Fatalf("an owner lost after the stop left no recorded end: %+v", out)
	}
	st, err := ReadJobState(home, testJobID)
	if err != nil || st.Result == nil || st.Result.Verdict() != 130 || !st.Result.Stopped ||
		!strings.Contains(st.Result.Error, "--cancel-on-exit") || st.Result.Image != "docker.io/library/alpine:latest" {
		t.Fatalf("the recorded end is %+v, %v", st.Result, err)
	}
	open, err := led.Open()
	if err != nil || len(open) != 0 {
		t.Errorf("the job's record is still open: %v, %v", open, err)
	}
}

// liveWatch answers the first of its processes that alive says has ended.
type liveWatch struct {
	ids   []ProcessIdentity
	alive func(pid int) bool
}

func (l *liveWatch) Exited(time.Duration) (*ProcessIdentity, error) {
	for i := range l.ids {
		if !l.alive(l.ids[i].PID) {
			return &l.ids[i], nil
		}
	}
	time.Sleep(time.Millisecond)
	return nil, nil
}

func (l *liveWatch) Close() {}

// ⛔ The client that detached a job exits at once, and so does the stub Git Bash
// ran it under. Neither stops the job: the script above them does. WSL-121.
func TestAClientThatDetachedAndItsStubDoNotStopTheJob(t *testing.T) {
	home := t.TempDir()
	owner, err := ClaimJob(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	client := ProcessIdentity{PID: 44, Start: 1, Name: "wsl-toolkit.exe", Role: RoleClient}
	stub := ProcessIdentity{PID: 45, Start: 1, Name: "bash.exe", Role: RoleCaller}
	script := ProcessIdentity{PID: 46, Start: 1, Name: "bash.exe", Role: RoleCaller}
	var mu sync.Mutex
	dead := map[int]bool{44: true, 45: true}
	alive := func(pid int) bool { mu.Lock(); defer mu.Unlock(); return !dead[pid] }
	stops := &stopsRecorded{found: true, then: func(string) { owner.Release() }}
	rewatched := make(chan []ProcessIdentity, 8)
	w := Watchdog{Home: home, IDs: []string{testJobID}, Watch: &fakeWatch{gone: &client}, Stop: stops.stop,
		Watched: []ProcessIdentity{anOwner, client, stub, script},
		Rewatch: func(ps []ProcessIdentity) (ProcessWatch, error) {
			rewatched <- ps
			return &liveWatch{ids: ps, alive: alive}, nil
		},
		ClientSettle: time.Millisecond, Settle: 5 * time.Second, Poll: time.Millisecond}
	done := make(chan WatchOutcome, 1)
	go func() {
		out, err := w.Run(context.Background())
		if err != nil {
			t.Error(err)
		}
		done <- out
	}()
	// ⛔ THE SCRIPT EXITS ONLY ONCE THE WATCH HOLDS IT, the wait that keeps the
	// owner. A fixed wait let a loaded host's watch first look at a script
	// already dead, drop it as ended with the client, and watch the owner alone.
	for holds := false; !holds; {
		select {
		case out := <-done:
			t.Fatalf("the job was stopped when its client detached: %+v, %v", out, stops.why)
		case ps := <-rewatched:
			for _, p := range ps {
				holds = holds || p.Role == RoleOwner
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the watch never opened its wait on what the detached client left")
		}
	}
	select {
	case out := <-done:
		t.Fatalf("the job was stopped when its client detached: %+v, %v", out, stops.why)
	case <-time.After(300 * time.Millisecond):
	}
	mu.Lock()
	dead[46] = true
	mu.Unlock()
	select {
	case out := <-done:
		if out.Exited == nil || out.Exited.PID != 46 || len(stops.ids) != 1 {
			t.Errorf("the script's exit answered %+v and stopped %v", out, stops.ids)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the script's exit did not stop the job")
	}
}

// ⛔ A job that ended before any watched process exited is left alone.
func TestAJobThatEndedFirstIsNotStopped(t *testing.T) {
	home := t.TempDir()
	claimEnded(t, home, testJobID)
	stops := &stopsRecorded{}
	w := Watchdog{Home: home, IDs: []string{testJobID}, Watch: &fakeWatch{gone: &aCaller}, Stop: stops.stop, Poll: time.Millisecond}
	out, err := w.Run(context.Background())
	if err != nil || out.Exited != nil || len(stops.ids) != 0 {
		t.Fatalf("an ended job was watched to %+v, %v, and stopped %v", out, err, stops.ids)
	}
}

// ⛔ A fleet row nobody claimed is stopped where its fleet is alive to start it,
// and left alone where the fleet's own process is the one that exited.
func TestARowNotYetClaimedIsStoppedOnlyWhileItsFleetLives(t *testing.T) {
	home := t.TempDir()
	stops := &stopsRecorded{}
	w := Watchdog{Home: home, IDs: []string{testJobID}, Watch: &fakeWatch{gone: &anOwner}, Stop: stops.stop, Poll: time.Millisecond}
	if out, err := w.Run(context.Background()); err != nil || out.Exited == nil || len(stops.ids) != 0 {
		t.Fatalf("a row whose fleet is gone was stopped: %+v, %v, %v", out, err, stops.ids)
	}
	stops = &stopsRecorded{}
	w = Watchdog{Home: home, IDs: []string{testJobID}, Watch: &fakeWatch{gone: &aCaller}, Stop: stops.stop, Poll: time.Millisecond}
	if out, err := w.Run(context.Background()); err != nil || len(out.Stopped) != 1 || len(stops.ids) != 1 {
		t.Fatalf("a row its fleet may still start was not stopped: %+v, %v, %v", out, err, stops.ids)
	}
}

// ⚠ The container of a job whose owner was lost before it existed is looked
// for again, and stopped when it appears.
func TestAContainerThatStartsAfterItsOwnerWasLostIsStopped(t *testing.T) {
	home := t.TempDir()
	owner, err := ClaimJob(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	stops := &stopsRecorded{then: func(string) { owner.Release() }}
	swept := 0
	w := Watchdog{Home: home, IDs: []string{testJobID}, Watch: &fakeWatch{gone: &anOwner}, Stop: stops.stop,
		Sweep:    func(context.Context, string) (bool, error) { swept++; return swept >= 2, nil },
		SweepFor: 5 * time.Second, SweepEvery: time.Millisecond, Settle: 5 * time.Second, Poll: time.Millisecond}
	if _, err := w.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if swept != 2 {
		t.Errorf("the late container was looked for %d time(s); want until it was found, twice", swept)
	}
}

// ⛔ Only a value this tool wrote makes a process the watcher.
func TestOnlyThisToolsValueStartsAWatcher(t *testing.T) {
	ids, ok := WatchdogJobs(WatchdogValue([]string{testJobID, "fedcba9876543210"}))
	if !ok || len(ids) != 2 || ids[1] != "fedcba9876543210" {
		t.Errorf("the tool's own value read as %v, %v", ids, ok)
	}
	for _, v := range []string{testJobID, "1", "", watchdogMark, watchdogMark + "../x"} {
		if _, ok := WatchdogJobs(v); ok {
			t.Errorf("%q started a watcher", v)
		}
	}
	got, err := DecodeWatched(mustEncode(t, []ProcessIdentity{aCaller}))
	if err != nil || len(got) != 1 || got[0] != aCaller {
		t.Errorf("the watched processes came back as %v, %v", got, err)
	}
}

func mustEncode(t *testing.T, ps []ProcessIdentity) string {
	t.Helper()
	s, err := EncodeWatched(ps)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// ⛔ A stop --cancel-on-exit asked for says so, and a stop by `wsl-toolkit stop`
// reads exactly as it did.
func TestAStopSaysWhoAskedForIt(t *testing.T) {
	home := t.TempDir()
	r := &Runner{home: home, ledger: &Ledger{path: home + string(os.PathSeparator) + "ledger.jsonl"}}
	if err := r.ledger.Append(LedgerEntry{Event: "stop", Kind: "job", ID: testJobID, Note: "requested by wsl-toolkit stop"}); err != nil {
		t.Fatal(err)
	}
	if got := r.stopError(testJobID, ""); got != "the job was stopped by wsl-toolkit stop" {
		t.Errorf("a stop by wsl-toolkit stop reads %q", got)
	}
	if err := RecordStop(home, "fedcba9876543210", "its caller, process 42 (bash.exe) exited before the job ended"); err != nil {
		t.Fatal(err)
	}
	got := r.stopError("fedcba9876543210", " before its container started")
	if got != "the job was stopped by --cancel-on-exit before its container started: its caller, process 42 (bash.exe) exited before the job ended" {
		t.Errorf("a stop by --cancel-on-exit reads %q", got)
	}
}

// ⛔ A container whose log ended while a stop's grace ran is still running, and
// its exit is not read until it ends. A reader racing the watcher recorded 0.
func TestAContainerInItsGraceHasNoExitYet(t *testing.T) {
	for _, out := range []string{"running|0\n", "stopping|0", "paused|0"} {
		if exit, ended := containerEnd(out); ended || exit != nil {
			t.Errorf("%q read as ended, exit %v", out, exit)
		}
	}
	if exit, ended := containerEnd("exited|137\n"); !ended || exit == nil || *exit != 137 {
		t.Errorf("an exited container read as %v, %v", exit, ended)
	}
	if exit, ended := containerEnd(""); !ended || exit != nil {
		t.Errorf("a container the engine no longer has read as %v, %v", exit, ended)
	}
}

// ⛔ The end a reader records names what stopped the job, and a stop by `wsl-toolkit
// stop` reads as it did.
func TestAReaderNamesWhatStoppedAJob(t *testing.T) {
	home := t.TempDir()
	r := &Runner{home: home, ledger: &Ledger{path: home + string(os.PathSeparator) + "ledger.jsonl"}}
	if err := r.ledger.Append(LedgerEntry{Event: "stop", Kind: "job", ID: testJobID, Note: "requested by wsl-toolkit stop"}); err != nil {
		t.Fatal(err)
	}
	if got := r.StoppedBy(testJobID); got != "wsl-toolkit stop" {
		t.Errorf("a stop by wsl-toolkit stop is named %q", got)
	}
	if err := RecordStop(home, "fedcba9876543210", "its owner, process 43 (wsl-toolkit.exe) exited before the job ended"); err != nil {
		t.Fatal(err)
	}
	if got := r.StoppedBy("fedcba9876543210"); !strings.Contains(got, "--cancel-on-exit, because its owner, process 43") {
		t.Errorf("a stop by --cancel-on-exit is named %q", got)
	}
}

// -- the processes themselves, on each host ----------------------------------

// sleepingChild starts this test binary again, as a process that sleeps.
func sleepingChild(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestASleepingChild$")
	cmd.Env = append(os.Environ(), "WSL_TOOLKIT_TEST_SLEEP=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd
}

// TestASleepingChild is the process sleepingChild starts. It passes at once
// anywhere else.
func TestASleepingChild(t *testing.T) {
	if os.Getenv("WSL_TOOLKIT_TEST_SLEEP") == "" {
		return
	}
	time.Sleep(2 * time.Minute)
}

func assertWatchSeesAnExit(t *testing.T) {
	cmd := sleepingChild(t)
	id, err := processIdentity(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	w, err := WatchProcesses([]ProcessIdentity{id})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if gone, err := w.Exited(100 * time.Millisecond); gone != nil || err != nil {
		t.Fatalf("a running process read as exited: %v, %v", gone, err)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	gone, err := w.Exited(10 * time.Second)
	if err != nil || gone == nil || gone.PID != id.PID {
		t.Fatalf("a killed process was not seen to exit: %v, %v", gone, err)
	}
}

func assertAReusedIDIsNotTheProcessWatched(t *testing.T) {
	self, err := Self()
	if err != nil {
		t.Fatal(err)
	}
	self.Start--
	w, err := WatchProcesses([]ProcessIdentity{self})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if gone, err := w.Exited(10 * time.Millisecond); err != nil || gone == nil {
		t.Fatalf("a process under the same id and another start time was taken for the one watched: %v, %v", gone, err)
	}
}

func assertTheChainStartsAtTheParent(t *testing.T) {
	callers, err := Callers()
	if err != nil {
		t.Fatal(err)
	}
	self, err := Self()
	if err != nil {
		t.Fatal(err)
	}
	if len(callers) == 0 || callers[0].PID != os.Getppid() || callers[0].Role != RoleCaller || callers[0].Start > self.Start {
		t.Fatalf("the chain above pid %d (parent %d) is %+v", self.PID, os.Getppid(), callers)
	}
}

func onlyOn(t *testing.T, goos string) {
	t.Helper()
	if runtime.GOOS != goos {
		t.Skipf("this half of the watch is the %s one", goos)
	}
}

func TestTheWindowsWatchSeesAProcessExit(t *testing.T) {
	onlyOn(t, "windows")
	assertWatchSeesAnExit(t)
}
func TestTheProcWatchSeesAProcessExit(t *testing.T) { onlyOn(t, "linux"); assertWatchSeesAnExit(t) }

func TestTheWindowsWatchReadsAReusedIDAsExited(t *testing.T) {
	onlyOn(t, "windows")
	assertAReusedIDIsNotTheProcessWatched(t)
}

func TestTheProcWatchReadsAReusedIDAsExited(t *testing.T) {
	onlyOn(t, "linux")
	assertAReusedIDIsNotTheProcessWatched(t)
}

func TestTheWindowsChainStartsAtTheParent(t *testing.T) {
	onlyOn(t, "windows")
	assertTheChainStartsAtTheParent(t)
}
func TestTheProcChainStartsAtTheParent(t *testing.T) {
	onlyOn(t, "linux")
	assertTheChainStartsAtTheParent(t)
}
