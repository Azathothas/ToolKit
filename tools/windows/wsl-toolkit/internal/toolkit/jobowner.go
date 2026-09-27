// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A job has one owner: the process that runs it, attached or detached. The owner
// holds jobs/ID/owner.lock for the job's whole life and writes jobs/ID/result.json
// before it lets go. A second process reads those two files to learn whether the
// job is running, how it ended, or that its owner is gone without saying.
// WSL-94.

// JobResultSchema versions result.json.
const JobResultSchema = "wsl-toolkit-job-result/1"

const (
	ownerLockName  = "owner.lock"
	resultFileName = "result.json"
	ownerLogName   = "owner.log"
)

// errLockHeld is what lockExclusive answers when another process holds the lock.
var errLockHeld = errors.New("another process holds the lock")

// ErrJobOwned refuses a claim on a job another process runs.
var ErrJobOwned = errors.New("another process owns this job")

// claimRetry is how long a claim tolerates a probe that holds the lock for the
// length of two system calls. A real owner holds it for the whole job, so a
// claim that is still refused after this is refused for a reason.
const claimRetry = 2 * time.Second

// JobOwner is this process's claim on one job.
type JobOwner struct {
	ID      string
	Dir     string
	release func()
}

// ValidJobID is the shape every id this tool makes has: sixteen lowercase hex
// characters. It is checked before an id reaches a path or a guest command.
func ValidJobID(id string) bool {
	if len(id) != 16 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// NewJobID draws an id for a job this process is about to start.
func NewJobID() (string, error) { return newJobID() }

// ClaimJob makes the job's host directory and takes its owner lock.
func ClaimJob(home, id string) (*JobOwner, error) {
	if !ValidJobID(id) {
		return nil, fmt.Errorf("%q is not a job id: an id is 16 lowercase hex characters", id)
	}
	dir := filepath.Join(home, "jobs", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("the job's directory: %w", err)
	}
	lock := filepath.Join(dir, ownerLockName)
	deadline := time.Now().Add(claimRetry)
	for {
		release, err := lockExclusive(lock)
		if err == nil {
			return &JobOwner{ID: id, Dir: dir, release: release}, nil
		}
		if !errors.Is(err, errLockHeld) || time.Now().After(deadline) {
			if errors.Is(err, errLockHeld) {
				return nil, fmt.Errorf("%w: %s", ErrJobOwned, id)
			}
			return nil, fmt.Errorf("the job's owner lock: %w", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Finish writes the job's result and then lets the lock go, in that order, so a
// reader that sees the lock released finds the result already there.
func (o *JobOwner) Finish(res JobResult) error {
	if o == nil {
		return nil
	}
	defer o.Release()
	doc, err := json.MarshalIndent(struct {
		Schema string    `json:"schema"`
		Result JobResult `json:"result"`
	}{JobResultSchema, res}, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(o.Dir, resultFileName), append(doc, '\n'), 0o600)
}

// Release lets the lock go without a result. A reader then finds a job whose
// owner ended without saying how, which is the truth.
func (o *JobOwner) Release() {
	if o != nil && o.release != nil {
		o.release()
		o.release = nil
	}
}

// JobState is what the host side knows about one job without asking the guest.
type JobState struct {
	ID string `json:"id"`
	// Owned is true while a process holds the job's owner lock.
	Owned bool `json:"owned"`
	// Result is the recorded end, or nil where none was written.
	Result *JobResult `json:"result,omitempty"`
	// Known is false for an id this state directory holds nothing about.
	Known bool `json:"known"`
	// Session is the record of a detached base session, or nil for a job.
	Session *SessionRecord `json:"session,omitempty"`
}

// Word is the one word a listing shows for a job's state.
func (s JobState) Word() string {
	switch {
	case s.Result != nil:
		return "ended"
	case s.Owned:
		return "running"
	case s.Session != nil:
		return "detached"
	case s.Known:
		return "no owner"
	}
	return "unknown"
}

// ReadJobState reads a job's owner lock, result and session record.
func ReadJobState(home, id string) (JobState, error) {
	st := JobState{ID: id}
	if !ValidJobID(id) {
		return st, fmt.Errorf("%q is not a job id: an id is 16 lowercase hex characters", id)
	}
	dir := filepath.Join(home, "jobs", id)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return st, nil
	}
	st.Known = true
	held, _, err := lockProbe(filepath.Join(dir, ownerLockName))
	if err != nil {
		return st, fmt.Errorf("the job's owner lock: %w", err)
	}
	st.Owned = held
	if res, ok, err := readJobResult(dir); err != nil {
		return st, err
	} else if ok {
		st.Result = &res
	}
	if rec, ok, err := ReadSessionRecord(home, id); err != nil {
		return st, err
	} else if ok {
		st.Session = &rec
	}
	return st, nil
}

func readJobResult(dir string) (JobResult, bool, error) {
	b, err := os.ReadFile(filepath.Join(dir, resultFileName))
	if errors.Is(err, os.ErrNotExist) {
		return JobResult{}, false, nil
	}
	if err != nil {
		return JobResult{}, false, err
	}
	var doc struct {
		Schema string    `json:"schema"`
		Result JobResult `json:"result"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return JobResult{}, false, fmt.Errorf("%s is not a job result: %w", filepath.Join(dir, resultFileName), err)
	}
	if doc.Schema != JobResultSchema {
		return JobResult{}, false, fmt.Errorf("%s has schema %q, and this build reads %q", filepath.Join(dir, resultFileName), doc.Schema, JobResultSchema)
	}
	return doc.Result, true, nil
}

// OwnerLogPath is where a detached owner's own progress goes.
func OwnerLogPath(home, id string) string { return filepath.Join(home, "jobs", id, ownerLogName) }

// JobIDEnv carries a preassigned id from `run --detach` to the owner it starts.
const JobIDEnv = "WSL_TOOLKIT_JOB_ID"

// DetachedEnv tells that owner it runs detached, so it keeps no live stream.
const DetachedEnv = "WSL_TOOLKIT_DETACHED"

// PreassignedJobID is the id a detaching caller chose, or empty.
//
// ⛔ ONLY A DETACHED OWNER READS IT. A value left exported in a caller's own
// shell would otherwise give every run one id, and each would overwrite the
// last one's record.
func PreassignedJobID() (string, error) {
	id := strings.TrimSpace(os.Getenv(JobIDEnv))
	if id == "" || strings.TrimSpace(os.Getenv(DetachedEnv)) == "" {
		return "", nil
	}
	if !ValidJobID(id) {
		return "", fmt.Errorf("%s %q is not a job id", JobIDEnv, id)
	}
	return id, nil
}
