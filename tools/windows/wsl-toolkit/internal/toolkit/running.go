// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ⭐ A MACHINE-WIDE CAP ON RUNNING JOBS. Builds ran one at a time by
// convention, because every instance's base runs in the one WSL VM and spends
// the one machine's memory. `run --max-running N` and `jobs.max_running` hold a
// job until fewer than N jobs run, in every instance, and it waits or is
// refused at once and says which. WSL-136.
//
// ⛔ A RUNNING JOB IS ONE WHOSE OWNER HOLDS ITS LOCK AND WHICH WAS ADMITTED. The
// owner lock alone counts a job that still waits for its own turn, so two
// waiting jobs would each count the other and neither would start. Admission
// writes a mark beside the lock, and only a marked job with a held lock counts.
// A killed job leaves its mark beside a lock Windows let go, so it does not.

// admittedName marks a job that passed its waits and runs.
const admittedName = "admitted"

// ErrOverRunningCap is a job refused because the machine already runs as many
// jobs as its cap allows.
var ErrOverRunningCap = errors.New("as many jobs run on this machine as the cap allows")

// ErrStoppedWaitingToRun is a job that a stop ended while it waited to run.
var ErrStoppedWaitingToRun = errors.New("stopped while it waited for a running slot")

// runningPoll is how often a waiting job counts again, and runningSay how often
// it says it still waits.
var (
	runningPoll = 2 * time.Second
	runningSay  = 30 * time.Second
)

// RunningJob is one job the cap counts.
type RunningJob struct {
	ID       string `json:"id"`
	Instance string `json:"instance"`
}

func (j RunningJob) String() string {
	if j.Instance == DefaultInstance {
		return "job " + j.ID
	}
	return "job " + j.ID + " of instance " + j.Instance
}

// RunningWait is jobs.max_running_wait as a duration, zero where it is empty.
func (j JobConfig) RunningWait() (time.Duration, error) {
	if strings.TrimSpace(j.MaxRunningWait) == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(j.MaxRunningWait)
	if err != nil {
		return 0, fmt.Errorf("jobs.max_running_wait %q is not a duration such as 30m: %w", j.MaxRunningWait, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("jobs.max_running_wait %s is negative. Leave it out to refuse at once, or give a positive duration", d)
	}
	return d, nil
}

// markAdmitted writes the mark that makes a job count as running, once.
func markAdmitted(home, id string) error {
	if home == "" || !ValidJobID(id) {
		return nil
	}
	path := filepath.Join(home, "jobs", id, admittedName)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return writeFileAtomic(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o600)
}

// RunningJobs is every job on this machine that runs now, in every instance
// under this state root.
func RunningJobs() []RunningJob {
	var out []RunningJob
	for _, h := range InstanceHomes() {
		entries, err := os.ReadDir(filepath.Join(h.Home, "jobs"))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || !ValidJobID(e.Name()) {
				continue
			}
			dir := filepath.Join(h.Home, "jobs", e.Name())
			if _, err := os.Stat(filepath.Join(dir, admittedName)); err != nil {
				continue
			}
			if held, _, err := lockProbe(filepath.Join(dir, ownerLockName)); err == nil && held {
				out = append(out, RunningJob{ID: e.Name(), Instance: h.Name})
			}
		}
	}
	return out
}

// runningList names the jobs a refusal waits on, five at most.
func runningList(jobs []RunningJob) string {
	names := make([]string, 0, len(jobs))
	for i, j := range jobs {
		if i == 5 {
			names = append(names, fmt.Sprintf("and %d more", len(jobs)-5))
			break
		}
		names = append(names, j.String())
	}
	return strings.Join(names, ", ")
}

// RunningCapRefusal is the refusal a job gets while the machine runs limit jobs
// or more, and nil while it runs fewer, for a caller that refuses before it
// starts anything. ⚠ It is a probe and not an admission: AdmitRunning in the
// job's owner is what decides.
func RunningCapRefusal(limit int) error {
	if limit <= 0 {
		return nil
	}
	if running := RunningJobs(); len(running) >= limit {
		return capRefusedAtOnce(limit, running)
	}
	return nil
}

func capRefusedAtOnce(limit int, running []RunningJob) error {
	return fmt.Errorf("%w: %d of %d run now (%s), so this job is refused at once. Pass --max-running-wait to wait for one to end",
		ErrOverRunningCap, len(running), limit, runningList(running))
}

// AdmitRunning holds a claimed job until fewer than limit jobs run on this
// machine, then marks it running, and answers how long it waited.
//
// ⛔ THE COUNT AND THE MARK ARE ONE STEP, under a lock every instance shares.
// Two jobs that counted at once would each see room for one and both start.
// The lock is held for the length of a count, and Windows lets it go however
// its holder ends.
//
// ⚠ wait 0 refuses at once. A positive wait counts again every 2 seconds, says
// every 30 seconds that it still waits and for whom, and refuses when it runs
// out. stopped is asked at every count, so a stop ends the wait with 130.
func AdmitRunning(ctx context.Context, owner *JobOwner, limit int, wait time.Duration, log func(string), stopped func() bool) (time.Duration, error) {
	started := time.Now()
	if owner == nil || limit <= 0 {
		return 0, nil
	}
	root, err := StateRoot()
	if err != nil {
		return 0, err
	}
	dir := filepath.Join(root, "locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, err
	}
	lock := filepath.Join(dir, "max-running.lock")
	home := filepath.Dir(filepath.Dir(owner.Dir))
	deadline := started.Add(wait)
	lastSaid := time.Time{}
	for {
		release, err := holdBriefly(ctx, lock)
		if err != nil {
			return time.Since(started), fmt.Errorf("--max-running: %w", err)
		}
		running := RunningJobs()
		if len(running) < limit {
			err := markAdmitted(home, owner.ID)
			release()
			if err != nil {
				return time.Since(started), fmt.Errorf("--max-running: the mark that counts this job as running: %w", err)
			}
			waited := time.Since(started)
			if log != nil && !lastSaid.IsZero() {
				log(fmt.Sprintf("job %s runs after waiting %s for a running slot", owner.ID, waited.Round(time.Second)))
			}
			return waited, nil
		}
		release()
		if stopped != nil && stopped() {
			return time.Since(started), fmt.Errorf("%w: %d of %d ran (%s)", ErrStoppedWaitingToRun, len(running), limit, runningList(running))
		}
		if wait <= 0 {
			return time.Since(started), capRefusedAtOnce(limit, running)
		}
		if !time.Now().Before(deadline) {
			return time.Since(started), fmt.Errorf("%w: %d of %d still run after it waited %s (%s), so this job is refused",
				ErrOverRunningCap, len(running), limit, wait, runningList(running))
		}
		if log != nil && (lastSaid.IsZero() || time.Since(lastSaid) >= runningSay) {
			if lastSaid.IsZero() {
				log(fmt.Sprintf("job %s waits for a running slot: %d of %d run now (%s). It waits up to %s", owner.ID, len(running), limit, runningList(running), wait))
			} else {
				log(fmt.Sprintf("job %s still waits for a running slot: %d of %d run now (%s)", owner.ID, len(running), limit, runningList(running)))
			}
			lastSaid = time.Now()
		}
		select {
		case <-ctx.Done():
			return time.Since(started), ctx.Err()
		case <-time.After(runningPoll):
		}
	}
}

// holdBriefly takes a lock that its holders keep for the length of a count,
// retrying while another holds it.
func holdBriefly(ctx context.Context, path string) (func(), error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		release, err := lockExclusive(path)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, errLockHeld) || time.Now().After(deadline) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
