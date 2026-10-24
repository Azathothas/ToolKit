// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ⭐ --cancel-on-exit TIES A JOB TO THE PROCESS THAT ASKED FOR IT. Killing a
// wrapper left its job running: a Git Bash script killed three minutes in left
// the process that ran its job alive, and the container ran nine more minutes
// to completion. A hard kill of the owner itself skips its own cleanup, and
// the container then ends only at the engine's deadline. WSL-121.
//
// ⛔ THE WATCHER IS A SEPARATE PROCESS, detached as an owner is, so the kill it
// exists for cannot reach it. It waits on a handle to every process it
// watches: the owner, and the chain of processes that started this command.
// When one ends before the job does, it stops the job as `stop` does, TERM and
// then KILL after the grace, and records why.

// WatchdogEnv starts this program as the watcher, and carries the jobs it
// watches after watchdogMark. WatchedEnv carries the processes it waits on.
// CallersEnv hands a detaching client's chain to the owner it starts.
const (
	WatchdogEnv  = "WSL_TOOLKIT_WATCHDOG"
	WatchedEnv   = "WSL_TOOLKIT_WATCHED"
	CallersEnv   = "WSL_TOOLKIT_CALLERS"
	watchdogMark = "wsl-toolkit-watchdog/1:"
)

// The two roles a watched process has.
const (
	RoleCaller = "caller"
	RoleOwner  = "owner"
)

// maxCallers bounds the chain of processes a job is tied to.
const maxCallers = 8

// chainEnds are the processes the chain stops below.
//
// ⚠ A DESKTOP RESTART IS NOT A CALLER LEAVING. The shell and the session's own
// services start every terminal on the machine, and one of them restarting is
// no reason to stop a build.
var chainEnds = map[string]bool{
	"explorer.exe": true, "svchost.exe": true, "services.exe": true, "wininit.exe": true,
	"winlogon.exe": true, "userinit.exe": true, "sihost.exe": true, "csrss.exe": true,
	"smss.exe": true, "init": true, "systemd": true,
}

// ProcessIdentity names one process, so that a later process given the same
// pid is not taken for it.
type ProcessIdentity struct {
	PID int `json:"pid"`
	// Start is when the process was created, in the host's own unit: 100 ns
	// ticks on Windows, clock ticks since boot elsewhere.
	Start int64  `json:"start"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

// String is the process as a stop's reason names it.
func (p ProcessIdentity) String() string {
	who := "its caller"
	if p.Role == RoleOwner {
		who = "its owner"
	}
	return fmt.Sprintf("%s, process %d (%s)", who, p.PID, p.Name)
}

// Callers is the chain of processes that started this one, nearest first: the
// process that ran this command, the one that started that, and on up to the
// desktop. A process that cannot be waited on ends the chain.
//
// ⭐ THE CHAIN AND NOT THE PARENT ALONE. Git Bash runs a Windows program under a
// stub of its own, and a killed script leaves that stub waiting on the program.
// Measured on this host: the script's process ended, and the stub and the
// program it ran stayed. The script is the stub's parent.
func Callers() ([]ProcessIdentity, error) { return callerChain(maxCallers) }

// Self is this process, as the owner of its job.
func Self() (ProcessIdentity, error) {
	p, err := selfIdentity()
	p.Role = RoleOwner
	return p, err
}

// EncodeWatched is a set of processes as one environment value.
func EncodeWatched(ps []ProcessIdentity) (string, error) {
	b, err := json.Marshal(ps)
	return string(b), err
}

// DecodeWatched reads what EncodeWatched wrote.
func DecodeWatched(s string) ([]ProcessIdentity, error) {
	var ps []ProcessIdentity
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(s), &ps); err != nil {
		return nil, fmt.Errorf("%s does not name processes: %w", WatchedEnv, err)
	}
	return ps, nil
}

// WatchdogValue is WatchdogEnv's value for a set of jobs.
func WatchdogValue(ids []string) string { return watchdogMark + strings.Join(ids, ",") }

// WatchdogJobs reads the jobs WatchdogEnv names, and false where the variable
// is not this tool's.
//
// ⛔ ONLY A VALUE THIS TOOL WROTE STARTS A WATCHER. A variable of that name left
// in somebody's shell must not turn every command into one.
func WatchdogJobs(v string) ([]string, bool) {
	rest, ok := strings.CutPrefix(v, watchdogMark)
	if !ok {
		return nil, false
	}
	var ids []string
	for _, id := range strings.Split(rest, ",") {
		if !ValidJobID(id) {
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, len(ids) > 0
}

// ProcessWatch waits for the first of a set of processes to exit.
type ProcessWatch interface {
	// Exited answers the first watched process that has exited, waiting up to
	// timeout for one to, and nil when none has.
	Exited(timeout time.Duration) (*ProcessIdentity, error)
	Close()
}

// WatchProcesses opens a wait on each process. One that is gone already, or
// that is another process under the same pid now, reads as exited at once.
func WatchProcesses(ps []ProcessIdentity) (ProcessWatch, error) { return watchProcesses(ps) }

// WatchOutcome is what one watch did.
type WatchOutcome struct {
	// Exited is the watched process that ended first, nil where every job ended
	// before any did.
	Exited *ProcessIdentity
	// Stopped are the jobs a stop was recorded for, and Recorded the ones whose
	// end this watch recorded because their owner was lost.
	Stopped  []string
	Recorded []string
}

// Watchdog stops jobs whose caller or owner ends before they do. WSL-121.
type Watchdog struct {
	Home  string
	IDs   []string
	Watch ProcessWatch
	Grace time.Duration
	// Stop records a stop with why and stops the job's container, and answers
	// whether a container was there. It is Runner.StopForCancelOnExit.
	Stop func(ctx context.Context, id, why string) (bool, error)
	// Sweep stops a job's container again and records nothing, and answers
	// whether one was there. It is Runner.StopContainerOf.
	Sweep func(ctx context.Context, id string) (bool, error)
	Log   func(string)
	// Poll is how long one wait for the processes lasts before the jobs are
	// read again.
	Poll time.Duration
	// SweepFor and SweepEvery bound the look for a container that starts after
	// its owner was lost: the owner's own wsl.exe call outlives it.
	SweepFor, SweepEvery time.Duration
	// Settle is how long past Grace an owner that is alive has to record the end
	// of a job this watch stopped. Zero means two minutes.
	Settle time.Duration
	seen   map[string]bool
}

func (w *Watchdog) log(s string) {
	if w.Log != nil {
		w.Log(s)
	}
}

// Run waits until every job has ended, or until a watched process exits first,
// and then stops each job that has not ended.
func (w *Watchdog) Run(ctx context.Context) (WatchOutcome, error) {
	var out WatchOutcome
	if w.seen == nil {
		w.seen = map[string]bool{}
	}
	poll := w.Poll
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	for {
		if w.allEnded() {
			return out, nil
		}
		gone, err := w.Watch.Exited(poll)
		if err != nil {
			return out, err
		}
		if gone != nil {
			out.Exited = gone
			w.cancel(ctx, *gone, &out)
			return out, nil
		}
		if err := ctx.Err(); err != nil {
			return out, err
		}
	}
}

// allEnded says whether every job recorded its end, or was removed after it.
//
// ⚠ A JOB NOT CLAIMED YET IS NOT ENDED. A fleet claims its rows as each one
// starts, so a row this watch has never seen is a row still to come, and one it
// has seen whose directory is gone ended and was removed, as an ephemeral job is.
func (w *Watchdog) allEnded() bool {
	ended := true
	for _, id := range w.IDs {
		st, err := ReadJobState(w.Home, id)
		if err != nil {
			continue
		}
		if st.Known {
			w.seen[id] = true
		}
		if st.Result == nil && (st.Known || !w.seen[id]) {
			ended = false
		}
	}
	return ended
}

// cancel stops every job that has not ended, waits for their owners to record
// their ends, and records the end of a job whose owner is gone.
func (w *Watchdog) cancel(ctx context.Context, gone ProcessIdentity, out *WatchOutcome) {
	why := gone.String() + " exited before the job ended"
	ownerLost := gone.Role == RoleOwner
	var pending []string
	found := map[string]bool{}
	for _, id := range w.IDs {
		st, err := ReadJobState(w.Home, id)
		switch {
		case err != nil, st.Result != nil, w.seen[id] && !st.Known:
			continue
		case !st.Known && ownerLost:
			// A row its fleet will never claim now: there is nothing to stop.
			continue
		}
		there, err := w.Stop(ctx, id, why)
		if err != nil {
			w.log("could not stop job " + id + ": " + err.Error())
		}
		found[id] = there
		out.Stopped = append(out.Stopped, id)
		if st.Known {
			pending = append(pending, id)
		}
	}
	settle := w.Settle
	if settle <= 0 {
		settle = 2 * time.Minute
	}
	deadline := time.Now().Add(w.Grace + settle)
	for len(pending) > 0 {
		var still []string
		for _, id := range pending {
			st, err := ReadJobState(w.Home, id)
			switch {
			case err != nil, st.Result != nil, !st.Known:
			case !st.Owned:
				// ⛔ THE OWNER IS GONE AND SAID NOTHING, so this watch says it, and
				// a `wait` answers 130 at once rather than after the engine's
				// deadline.
				if err := RecordLostEnd(w.Home, id, why); err != nil {
					w.log("could not record the end of job " + id + ": " + err.Error())
					continue
				}
				out.Recorded = append(out.Recorded, id)
			default:
				still = append(still, id)
			}
		}
		pending = still
		if len(pending) == 0 || !time.Now().Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
	w.sweep(ctx, out.Recorded, found)
}

// sweep stops the container of a job whose owner was lost before it existed.
//
// ⚠ THE OWNER'S OWN wsl.exe CALL OUTLIVES IT. Windows does not end a killed
// process's children, so an image pull the owner started still ends in a
// container. It is looked for, for SweepFor, and stopped when it appears.
func (w *Watchdog) sweep(ctx context.Context, lost []string, found map[string]bool) {
	if w.Sweep == nil || w.SweepFor <= 0 {
		return
	}
	var ids []string
	for _, id := range lost {
		if !found[id] {
			ids = append(ids, id)
		}
	}
	every := w.SweepEvery
	if every <= 0 {
		every = 10 * time.Second
	}
	until := time.Now().Add(w.SweepFor)
	for len(ids) > 0 && time.Now().Before(until) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
		var still []string
		for _, id := range ids {
			if there, err := w.Sweep(ctx, id); err != nil || !there {
				still = append(still, id)
			}
		}
		ids = still
	}
}

// RecordLostEnd records the end of a job whose owner is gone and whose stop a
// watcher asked for: stopped, 130, with why, and its ledger record closed.
// WSL-121.
func RecordLostEnd(home, id, why string) error {
	owner, err := ClaimJob(home, id)
	if err != nil {
		return err
	}
	res := JobResult{ID: id, Exit: 130, Cancelled: true, Stopped: true,
		Error: "the job was stopped by --cancel-on-exit: " + why + ", and its owner was lost, so the watcher recorded this end"}
	led := &Ledger{path: filepath.Join(home, "ledger.jsonl")}
	if open, err := led.Open(); err == nil {
		for _, e := range open {
			if e.Kind == "job" && e.ID == id {
				res.Image, res.Label = e.Image, e.Image
			}
		}
	}
	res.Seal()
	if err := owner.Finish(res); err != nil {
		return err
	}
	return led.Append(LedgerEntry{Event: "close", Kind: "job", ID: id, Note: "closed by --cancel-on-exit: " + why + ", and its owner was lost"})
}

// RecordStop records a stop for a job, for a watcher that cannot reach the
// engine. A job not yet claimed then ends before its container starts.
func RecordStop(home, id, why string) error {
	if !ValidJobID(id) {
		return fmt.Errorf("%q is not a job id: an id is 16 lowercase hex characters", id)
	}
	led := &Ledger{path: filepath.Join(home, "ledger.jsonl")}
	return led.Append(LedgerEntry{Event: "stop", Kind: "job", ID: id, Note: CancelOnExitNote + why})
}

// errNoChain is a host where the processes above this one cannot be read.
var errNoChain = errors.New("this host does not say which process started this one")
