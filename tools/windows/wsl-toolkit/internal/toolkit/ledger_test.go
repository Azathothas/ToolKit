// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// -- WSL-173: a compaction keeps the stop a job can still read ----------------

func TestAStopSurvivesCompactionWhileItsJobIsOpen(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	l, err := OpenLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(LedgerEntry{Event: "open", Kind: "job", ID: testJobID}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(LedgerEntry{Event: "stop", Kind: "job", ID: testJobID}); err != nil {
		t.Fatal(err)
	}
	n, err := l.Compact()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("compaction kept %d open record(s), expected 1", n)
	}
	if !StopRecorded(testJobID) {
		t.Fatal("a compaction dropped the stop of a job that is still open, so the job runs as if nobody stopped it")
	}
}

func TestAStopSurvivesCompactionWhileItsJobDirectoryExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	l, err := OpenLedger()
	if err != nil {
		t.Fatal(err)
	}
	// ⚠ AN OWNER WAITING FOR ITS --exclusive NAME HAS ITS DIRECTORY AND NO OPEN
	// RECORD YET, and it reads the stop as it waits.
	dir := filepath.Join(home, "jobs", testJobID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(LedgerEntry{Event: "stop", Kind: "job", ID: testJobID}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Compact(); err != nil {
		t.Fatal(err)
	}
	if !StopRecorded(testJobID) {
		t.Fatal("a compaction dropped the stop of a job whose directory is still here")
	}
	// ⭐ A STOP FOR A JOB THAT IS OVER IS STILL DROPPED, so the ledger does not
	// grow by one line for every stop ever asked.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Compact(); err != nil {
		t.Fatal(err)
	}
	if StopRecorded(testJobID) {
		t.Fatal("a stop for a job with no open record and no directory survived compaction")
	}
}

// -- WSL-173: a read waits out the moment a rewrite holds the ledger ----------

// ⚠ THE HOLD IS AN OPEN WITH NO SHARING, which is what a rename does to the file
// it replaces on Windows. Elsewhere the hold is an advisory lock no read sees,
// so these cases skip there rather than pass without proving anything.
func TestAStopIsReadWhileARewriteHoldsTheLedger(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only a Windows open refuses a read")
	}
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	l, err := OpenLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(LedgerEntry{Event: "open", Kind: "job", ID: testJobID}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(LedgerEntry{Event: "stop", Kind: "job", ID: testJobID}); err != nil {
		t.Fatal(err)
	}
	release, err := lockExclusive(l.Path())
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		release()
	}()
	if got, err := stopRecordedIn(l, testJobID); err != nil || !got {
		t.Fatal("a read during a rewrite answered no stop for a job that has one, so the job runs as if nobody stopped it")
	}
}

func TestALedgerReadHeldPastItsRetriesFails(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	l, err := OpenLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(LedgerEntry{Event: "open", Kind: "job", ID: testJobID}); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		t.Skip("only a Windows open refuses a read")
	}
	release, err := lockExclusive(l.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	// ⛔ A FILE SOMETHING KEEPS OPEN IS AN ERROR, NOT AN EMPTY LEDGER. A reader
	// that answered no records would let cleanup remove live work.
	entries, err := l.All()
	if err == nil {
		t.Fatalf("a ledger held past every retry read as %d record(s) and no error", len(entries))
	}
	if !strings.Contains(err.Error(), "after 20 attempts") {
		t.Fatalf("the error does not say how often the read was tried: %v", err)
	}
}

// refusedLedger is a ledger whose every read is refused, as a read held past its
// retries is. Only Windows can stage that for real, so the read itself answers
// the refusal here, on any host.
func refusedLedger(t *testing.T, home string) *Ledger {
	t.Helper()
	path := filepath.Join(home, "ledger.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ledgerReadFile = os.ReadFile })
	ledgerReadFile = func(string) ([]byte, error) { return nil, errors.New("the read was refused") }
	return &Ledger{path: path}
}

// mostlyUnreadableLedger is a ledger that reads, with more lines that do not
// parse than lines that do: what killed writes leave behind.
func mostlyUnreadableLedger(t *testing.T, home string, records ...LedgerEntry) *Ledger {
	t.Helper()
	l := &Ledger{path: filepath.Join(home, "ledger.jsonl")}
	if err := os.WriteFile(l.path, []byte(strings.Repeat("not a record\n", 5)), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, e := range records {
		if err := l.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

// ⛔ A LEDGER THAT CANNOT BE READ IS NOT "NO STOP". Every reader of the stop got
// false for an error, so a stopped job ran on. WSL-173.
func TestAnUnreadableLedgerIsNotReadAsNoStop(t *testing.T) {
	l := refusedLedger(t, t.TempDir())
	if _, err := l.All(); err == nil {
		t.Fatal("the staged ledger read without an error, so this case proves nothing")
	}
	if _, err := stopRecordedIn(l, testJobID); err == nil {
		t.Fatal("a ledger that could not be read answered no stop and no error")
	}
}

// ⛔ THE CHECK BEFORE THE CONTAINER STARTS REFUSES THE JOB, and nothing runs. The
// job is driven through the fake WSL, so the container's call is what is
// counted. WSL-173.
func TestAJobWhoseLedgerCannotBeReadIsNotStarted(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(staleFakeWslEnv, dir)
	t.Setenv("WSL_TOOLKIT_HOME", home)
	cfg := Config{Base: BaseConfig{Name: "wsl-toolkit-stale", Image: "docker.io/library/alpine:latest", User: "agent"}}
	r := &Runner{cfg: cfg, wsl: &Wsl{Path: self}, home: home, ledger: refusedLedger(t, home), log: func(string) {}}
	res := r.Run(t.Context(), JobSpec{
		Image: "docker.io/library/alpine:latest", Script: []byte("true\n"),
		ContainerLifecycle: ContainerEphemeral,
	})
	if res.Exit != 2 || !res.Unreached || res.Stopped {
		t.Fatalf("a job whose stop could not be read answered exit %d, unreached %v, stopped %v: %s", res.Exit, res.Unreached, res.Stopped, res.Error)
	}
	if !strings.Contains(res.Error, "could not read whether the job was stopped") {
		t.Errorf("the refusal does not say why: %s", res.Error)
	}
	data, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "job@") {
		t.Fatalf("the container ran although its stop could not be read; calls %v", strings.Fields(string(data)))
	}
}

// ⚠ AN EXIT READ WITHOUT THE LEDGER KEEPS ITS OWN CODE, AND THE FAILED READ IS
// SAID. It was silently read as no stop. WSL-173.
func TestAnExitReadWithoutTheLedgerSaysWhy(t *testing.T) {
	var said []string
	r := &Runner{ledger: refusedLedger(t, t.TempDir()), log: func(s string) { said = append(said, s) }}
	if r.EndedByStop(testJobID, 143) {
		t.Fatal("an exit read without the ledger was taken as a stop")
	}
	if !strings.Contains(strings.Join(said, "\n"), "could not read whether job "+testJobID+" was stopped") {
		t.Fatalf("the failed read was not said: %q", said)
	}
}

// -- WSL-173: a ledger with mostly unreadable lines still answers a stop -------

// ⛔ A STOP THAT PARSED IS HONOURED AMONG LINES THAT DID NOT. The lines that
// parsed were thrown away with the error, and a stopped job answered exit 2.
func TestAStopAmongUnreadableLinesIsRead(t *testing.T) {
	l := mostlyUnreadableLedger(t, t.TempDir(), LedgerEntry{Event: "stop", Kind: "job", ID: testJobID})
	if _, err := l.All(); err == nil {
		t.Fatal("the staged ledger read without an error, so this case proves nothing")
	}
	if got, err := stopRecordedIn(l, testJobID); err != nil || !got {
		t.Fatalf("a stop that parsed answered stopped %v: %v", got, err)
	}
}

// ⛔ LINES THAT DO NOT PARSE ARE NOT A FAILED READ, and the job runs as it did
// before the check. A torn line refused every job until the file was edited by
// hand. WSL-173.
func TestAJobRunsOverALedgerWithMostlyUnreadableLines(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(staleFakeWslEnv, dir)
	t.Setenv("WSL_TOOLKIT_HOME", home)
	cfg := Config{Base: BaseConfig{Name: "wsl-toolkit-stale", Image: "docker.io/library/alpine:latest", User: "agent"}}
	r := &Runner{cfg: cfg, wsl: &Wsl{Path: self}, home: home, ledger: mostlyUnreadableLedger(t, home), log: func(string) {}}
	res := r.Run(t.Context(), JobSpec{
		Image: "docker.io/library/alpine:latest", Script: []byte("true\n"),
		ContainerLifecycle: ContainerEphemeral,
	})
	if res.Unreached || res.Stopped {
		t.Fatalf("a job over a ledger with unreadable lines answered exit %d, unreached %v, stopped %v: %s", res.Exit, res.Unreached, res.Stopped, res.Error)
	}
	data, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "job@") {
		t.Fatalf("the container never ran; calls %v", strings.Fields(string(data)))
	}
}

// ⛔ A RECORD APPENDED AFTER A TORN LINE IS ITS OWN LINE. It joined the torn line,
// so the next job's open record was unreadable and cleanup missed it. WSL-173.
func TestARecordAppendedAfterATornLineIsRead(t *testing.T) {
	l := &Ledger{path: filepath.Join(t.TempDir(), "ledger.jsonl")}
	torn := `{"schema":"` + LedgerSchema + `","event":"op`
	if err := os.WriteFile(l.path, []byte(torn), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(LedgerEntry{Event: "open", Kind: "job", ID: testJobID}); err != nil {
		t.Fatal(err)
	}
	open, err := l.Open()
	if err != nil {
		t.Fatalf("the ledger after one torn line and one record did not read: %v", err)
	}
	if len(open) != 1 || open[0].ID != testJobID {
		t.Fatalf("the record appended after a torn line was lost: %+v", open)
	}
}

// -- WSL-185: a compaction is serialized across processes ---------------------

// ledgerCompactEnv names a ledger a re-run of this test binary compacts, as a
// second process would, and then exits.
const ledgerCompactEnv = "WSL_TOOLKIT_TEST_LEDGER_COMPACT"

// ⚠ AN init AND NOT A CASE, because the second process is this test binary and
// it must compact before any case runs.
func init() {
	path := os.Getenv(ledgerCompactEnv)
	if path == "" {
		return
	}
	rounds, _ := strconv.Atoi(os.Getenv(ledgerCompactEnv + "_ROUNDS"))
	l := &Ledger{path: path}
	for i := 0; i < rounds; i++ {
		if _, err := l.Compact(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func TestACompactionWaitsForTheLedgerLockAnotherProcessHolds(t *testing.T) {
	dir := t.TempDir()
	l := &Ledger{path: filepath.Join(dir, "ledger.jsonl")}
	if err := l.Append(LedgerEntry{Event: "open", Kind: "job", ID: testJobID}); err != nil {
		t.Fatal(err)
	}
	release, err := lockExclusive(filepath.Join(dir, ledgerLockName))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	go func() {
		_, err := l.Compact()
		done <- err
	}()
	go func() {
		done <- l.Append(LedgerEntry{Event: "open", Kind: "upload", ID: "u1"})
	}()
	select {
	case <-done:
		release()
		t.Fatal("a ledger write went ahead while another process held the ledger's lock")
	case <-time.After(500 * time.Millisecond):
	}
	release()
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("a ledger write did not go ahead after the lock was let go")
		}
	}
}

func TestTwoProcessesCompactingLoseNoRecordAnotherAppends(t *testing.T) {
	parallelCase(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.jsonl")
	l := &Ledger{path: path}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("old%02d", i)
		if err := l.Append(LedgerEntry{Event: "open", Kind: "job", ID: id}); err != nil {
			t.Fatal(err)
		}
		if err := l.Append(LedgerEntry{Event: "close", Kind: "job", ID: id}); err != nil {
			t.Fatal(err)
		}
	}

	// ⚠ THE APPENDS RUN FOR AS LONG AS BOTH COMPACTING PROCESSES DO, so a slow
	// process start cannot leave the two with nothing to interleave with.
	var compacting sync.WaitGroup
	errs := make(chan error, 3)
	for p := 0; p < 2; p++ {
		cmd := exec.Command(self)
		cmd.Env = append(os.Environ(), ledgerCompactEnv+"="+path, ledgerCompactEnv+"_ROUNDS=100")
		compacting.Add(1)
		go func() {
			defer compacting.Done()
			if out, err := cmd.CombinedOutput(); err != nil {
				errs <- fmt.Errorf("a compacting process failed: %v: %s", err, out)
			}
		}()
	}
	finished := make(chan struct{})
	go func() {
		compacting.Wait()
		close(finished)
	}()
	appends := 0
	for running := true; running || appends < 50; appends++ {
		select {
		case <-finished:
			running = false
		default:
		}
		if err := l.Append(LedgerEntry{Event: "open", Kind: "upload", ID: fmt.Sprintf("new%05d", appends)}); err != nil {
			t.Fatal(err)
		}
	}
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	open, err := l.Open()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range open {
		got[e.ID] = true
	}
	missing := 0
	for i := 0; i < appends; i++ {
		if !got[fmt.Sprintf("new%05d", i)] {
			missing++
		}
	}
	if missing > 0 {
		t.Fatalf("%d of %d records appended while two other processes compacted were lost. Each one is a transfer gc would not see as live", missing, appends)
	}
	if got["old00"] {
		t.Error("a closed record survived compaction")
	}
}
