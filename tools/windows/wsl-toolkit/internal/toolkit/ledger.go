package toolkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LedgerSchema is what every line carries. ⛔ Versioned because the reader is a
// program, and because cleanup acts on what it reads: a record misparsed as a
// different shape is a removal aimed at the wrong thing.
const LedgerSchema = "wsl-toolkit-ledger/1"

// Ledger records what this executable created on this machine, so cleanup can
// find it after the process that created it is gone.
//
// ⭐ IT IS APPENDED TO BEFORE THE RESOURCE EXISTS, NOT AFTER. A record written
// after a successful create cannot describe the create that was killed half way,
// and that is precisely the run whose leftovers nobody can find. An open with no
// close is the signal cleanup looks for.
//
// ⚠ It is a log and not a database. A duplicate open, a close with no open and a
// record for something a person already removed by hand are all normal, and
// every reader is written to tolerate them.
type Ledger struct {
	path string
	mu   sync.Mutex
}

// ledgerLockName is the file beside the ledger whose lock every write holds.
const ledgerLockName = "ledger.lock"

// ledgerLockWait is how long one write waits for another process's write. A
// write holds the lock for one append or one rewrite of a small file, so a
// wait this long is a lock something is not letting go.
const ledgerLockWait = 30 * time.Second

// LedgerEntry is one line.
type LedgerEntry struct {
	Schema   string    `json:"schema"`
	Event    string    `json:"event"` // open, close or stop
	Kind     string    `json:"kind"`  // job, container, base
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	Distro   string    `json:"distro,omitempty"`
	Image    string    `json:"image,omitempty"`
	GuestDir string    `json:"guest_dir,omitempty"`
	HostDir  string    `json:"host_dir,omitempty"`
	Deadline time.Time `json:"deadline,omitempty"`
	Note     string    `json:"note,omitempty"`
}

// OpenLedger binds to the state directory's ledger file.
//
// ⚠ BINDING IS NOT CREATING. It used to create the state directory, so every
// command that opened a ledger to READ it created one. Append is what writes and
// Append is what ensures. WSL-55.
func OpenLedger() (*Ledger, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	return &Ledger{path: filepath.Join(home, "ledger.jsonl")}, nil
}

// Append adds one line. A failure to record is reported to the caller rather
// than swallowed: a resource created with no record is one cleanup cannot find,
// which is the whole failure this file exists to prevent.
func (l *Ledger) Append(e LedgerEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	release, err := l.lockAcross()
	if err != nil {
		return err
	}
	defer release()
	e.Schema = LedgerSchema
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	// ⛔ A LINE A KILLED WRITE LEFT UNFINISHED IS ENDED FIRST. A record appended
	// onto it joined it, so the open record of the next job was unreadable too
	// and cleanup could not see that job. WSL-173.
	line := append(b, '\n')
	torn, err := endsTorn(f)
	if err != nil {
		_ = f.Close()
		return err
	}
	if torn {
		line = append([]byte{'\n'}, line...)
	}
	_, writeErr := f.Write(line)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// endsTorn says whether the ledger's last line has no newline: a write killed
// part way.
func endsTorn(f *os.File) (bool, error) {
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return false, err
	}
	last := make([]byte, 1)
	if _, err := f.ReadAt(last, info.Size()-1); err != nil {
		return false, err
	}
	return last[0] != '\n', nil
}

// Open reports every resource with an open record and no close.
func (l *Ledger) Open() ([]LedgerEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries, err := l.allLocked()
	if err != nil {
		return nil, err
	}
	return openOf(entries), nil
}

// lockAcross takes the lock every process's write holds, for the length of one
// write.
//
// ⛔ THE MUTEX SERIALIZES ONE PROCESS, AND THE LEDGER IS WRITTEN BY SEVERAL. A
// `run`, the helper and a spool each append, and `gc --apply` rewrites the file
// from another process. A rewrite reads, then renames a new file over the old,
// and a line appended between the two lands in the file the rename replaces.
// That is a lost OPEN record, and the next `gc --apply` then removes a live
// upload or artifact set. Append and Compact both hold this lock, so a rewrite
// and an append cannot interleave. WSL-185.
//
// ⚠ A READ TAKES NO LOCK, AND IT RETRIES THE SWAP. The rewrite is a rename, so
// a reader never sees half of either file. On Windows the rename can still
// refuse a reader's open for a moment, which readLedgerFile waits out.
func (l *Ledger) lockAcross() (func(), error) {
	lock := filepath.Join(filepath.Dir(l.path), ledgerLockName)
	deadline := time.Now().Add(ledgerLockWait)
	for {
		release, err := lockExclusive(lock)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, errLockHeld) {
			return nil, fmt.Errorf("the ledger's lock %s: %w", lock, err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("the ledger's lock %s was held by another process for %s", lock, ledgerLockWait)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// openOf is every entry with an open record and no close, in a fixed order.
func openOf(entries []LedgerEntry) []LedgerEntry {
	open := map[string]LedgerEntry{}
	for _, e := range entries {
		key := e.Kind + "/" + e.ID
		switch e.Event {
		case "open":
			open[key] = e
		case "close":
			delete(open, key)
		}
	}
	out := make([]LedgerEntry, 0, len(open))
	for _, e := range open {
		out = append(out, e)
	}
	// ⚠ SORTED, because a map's order is not one. Compact writes what this
	// returns, so without a fixed order the same set of records produces a
	// different file every time and nothing about the ledger is diffable.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.Before(out[j].At)
		}
		return out[i].Kind+"/"+out[i].ID < out[j].Kind+"/"+out[j].ID
	})
	return out
}

// All reads every line, skipping any that does not parse.
//
// ⚠ A LINE THAT DOES NOT PARSE IS COUNTED AND REPORTED, NOT SILENTLY DROPPED. A
// process killed mid-write leaves a partial last line, which is normal; a file
// where most lines are unreadable is a different fact and a reader that
// swallowed both would report an empty machine over a full one.
func (l *Ledger) All() ([]LedgerEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.allLocked()
}

func (l *Ledger) allLocked() ([]LedgerEntry, error) {
	data, err := readLedgerFile(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []LedgerEntry
	bad := 0
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e LedgerEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			bad++
			continue
		}
		if e.Schema != LedgerSchema {
			bad++
			continue
		}
		out = append(out, e)
	}
	if bad > 0 && bad > len(out) {
		return out, &ledgerUnreliableError{path: l.path, bad: bad, total: bad + len(out)}
	}
	return out, nil
}

// ledgerUnreliableError is a ledger that was read but whose lines mostly do not
// parse. All still returns the lines that did.
//
// ⚠ IT IS A REASON NOT TO CLEAN UP, NOT A FAILED READ. Cleanup acts on what the
// ledger lacks, so a list that is mostly missing is refused there. A stop lookup
// acts on what the ledger holds, and the lines that parsed answer it. WSL-173.
type ledgerUnreliableError struct {
	path       string
	bad, total int
}

func (e *ledgerUnreliableError) Error() string {
	return fmt.Sprintf("%s: %d of %d lines are unreadable, so this is not a reliable list of what to clean up", e.path, e.bad, e.total)
}

// readLedgerFile reads the ledger, waiting out a rewrite that holds it.
//
// ⛔ ON WINDOWS A READ DURING A REWRITE CAN SEE NEITHER FILE. The rename opens
// the ledger with no sharing, and a read in that moment answers a sharing
// violation. A reader that took that as no ledger answered "no stop" for a job
// somebody stopped, and the job ran to completion. Measured at about one read
// in seventy against another process compacting 300 times. WSL-173.
//
// ⚠ THE RETRY IS BOUNDED AND THE LAST ERROR IS RETURNED WITH ITS ATTEMPT COUNT,
// as renameReplacing does for the write. A rewrite holds the file for one
// rename, so a refusal that outlasts the retries is a file something keeps open.
func readLedgerFile(path string) ([]byte, error) {
	const attempts = 20
	delay := 2 * time.Millisecond
	var err error
	for i := 0; i < attempts; i++ {
		var data []byte
		if data, err = ledgerReadFile(path); err == nil || !ledgerReadBusy(err) {
			return data, err
		}
		time.Sleep(delay)
		if delay < 40*time.Millisecond {
			delay *= 2
		}
	}
	return nil, fmt.Errorf("reading %s after %d attempts: %w", filepath.Base(path), attempts, err)
}

// ledgerReadFile is the read readLedgerFile retries.
//
// ⚠ A VARIABLE SO A CASE CAN REFUSE THE READ ON ANY HOST. A refused read is
// staged for real only on Windows, and the suite also runs as root on Linux,
// where no file mode refuses one.
var ledgerReadFile = os.ReadFile

// Path is where the ledger lives, for a report that names it.
func (l *Ledger) Path() string { return l.path }

// Compact rewrites the ledger keeping the still-open records and the stops a
// job can still read. It answers how many open records it kept.
//
// ⚠ IT RUNS ON EVERY `gc --apply`, scoped or not, after the stale records are
// closed. A log that trims itself can remove the line somebody needed, so what
// it keeps is decided here and nowhere else.
//
// ⛔ A STOP IS KEPT WHILE ITS JOB CAN STILL READ IT. The owner reads the stop
// before its container starts, as it starts and after it exits, and `wait`
// reads it for a job whose owner is gone. A compaction that dropped it let a
// job the operator stopped run to completion. A stop is kept while its kind
// and id have an open record, or while the job's host directory exists. The
// second covers an owner still waiting for its --exclusive name, which has no
// open record yet. A stop for a job with neither is over. WSL-173.
func (l *Ledger) Compact() (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	release, err := l.lockAcross()
	if err != nil {
		return 0, err
	}
	defer release()
	// ⛔ THE READ AND THE WRITE ARE UNDER ONE HOLD OF BOTH LOCKS. Compact used
	// to read through Open and then take the lock to write, and a record
	// appended between the two was read by neither and discarded by the write.
	// That is a lost OPEN record, which cleanup reads as no work in flight.
	entries, err := l.allLocked()
	if err != nil {
		return 0, err
	}
	open := openOf(entries)
	live := map[string]bool{}
	for _, e := range open {
		live[e.Kind+"/"+e.ID] = true
	}
	keep := append([]LedgerEntry{}, open...)
	kept := map[string]bool{}
	for _, e := range entries {
		key := e.Kind + "/" + e.ID
		if e.Event == "stop" && !kept[key] && (live[key] || l.jobDirExists(e)) {
			kept[key] = true
			keep = append(keep, e)
		}
	}
	var b strings.Builder
	for _, e := range keep {
		line, err := json.Marshal(e)
		if err != nil {
			return 0, err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := writeFileAtomic(l.path, []byte(b.String()), 0o600); err != nil {
		return 0, err
	}
	return len(open), nil
}

// jobDirExists says whether a job's host directory is still in the state
// directory this ledger lives in.
func (l *Ledger) jobDirExists(e LedgerEntry) bool {
	if e.Kind != "job" || !ValidJobID(e.ID) {
		return false
	}
	info, err := os.Stat(filepath.Join(filepath.Dir(l.path), "jobs", e.ID))
	return err == nil && info.IsDir()
}
