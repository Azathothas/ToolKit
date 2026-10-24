// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
