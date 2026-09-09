package toolkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// LedgerEntry is one line.
type LedgerEntry struct {
	Schema   string    `json:"schema"`
	Event    string    `json:"event"` // open or close
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
func OpenLedger() (*Ledger, error) {
	home, err := EnsureHome()
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
	e.Schema = LedgerSchema
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(b, '\n'))
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// Open reports every resource with an open record and no close.
func (l *Ledger) Open() ([]LedgerEntry, error) {
	entries, err := l.All()
	if err != nil {
		return nil, err
	}
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
	return out, nil
}

// All reads every line, skipping any that does not parse.
//
// ⚠ A LINE THAT DOES NOT PARSE IS COUNTED AND REPORTED, NOT SILENTLY DROPPED. A
// process killed mid-write leaves a partial last line, which is normal; a file
// where most lines are unreadable is a different fact and a reader that
// swallowed both would report an empty machine over a full one.
func (l *Ledger) All() ([]LedgerEntry, error) {
	data, err := os.ReadFile(l.path)
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
		return out, fmt.Errorf("%s: %d of %d lines are unreadable, so this is not a reliable list of what to clean up", l.path, bad, bad+len(out))
	}
	return out, nil
}

// Path is where the ledger lives, for a report that names it.
func (l *Ledger) Path() string { return l.path }

// Compact rewrites the ledger keeping only the still-open records.
//
// ⚠ It is not run automatically. A log that trims itself is a log that removed
// the line somebody needed, and this one is a few hundred bytes per job.
func (l *Ledger) Compact() (int, error) {
	open, err := l.Open()
	if err != nil {
		return 0, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var b strings.Builder
	for _, e := range open {
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
