// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// ownerFlags are about the process that owns a job: --cancel-on-exit and the
// running cap, on run and matrix. WSL-121, WSL-136.
//
// ⚠ A STRUCT OF THEIR OWN AND NOT jobFlags, because none of them crosses the
// helper: the helper owns a job it runs, so each is refused on that route.
// TestEveryJobFlagCrossesTheWire holds jobFlags to the wire, and
// TestOwnerFlagsAreRefusedThroughTheHelper holds these to the refusal.
type ownerFlags struct {
	fs             *flag.FlagSet
	cancelOnExit   bool
	maxRunning     int
	maxRunningWait time.Duration
	// capFromConfig is a cap the project's file set, which the helper route
	// drops with a note rather than refusing the job over it.
	capFromConfig bool
	// lifecycle is the job's container lifecycle, which decides what an ended
	// job leaves on this machine. WSL-121.
	lifecycle string
}

// watchGrace is the time between the TERM and the KILL of a stop the watcher
// makes, which is `stop`'s own default.
const watchGrace = 10 * time.Second

// sweepEvery is how often a watcher whose job's owner was lost looks again for
// the container that owner's last call may still start.
//
// ⚠ EACH LOOK IS ONE wsl.exe CALL. A look every 10 seconds was 90 calls over
// the sweep's 15 minutes; one every 30 seconds is 30. WSL-121.
const sweepEvery = 30 * time.Second

// sweepFor is how long that look goes on: longer than the owner's own wsl.exe
// call takes to pull an image and start a container.
const sweepFor = 15 * time.Minute

func (o *ownerFlags) bind(fs *flag.FlagSet) {
	o.fs = fs
	fs.BoolVar(&o.cancelOnExit, "cancel-on-exit", false, "stop the job, TERM then KILL after 10s, when the process that started this command, or one above it, exits before the job ends. A detached watcher waits on them and on the job's owner")
	fs.IntVar(&o.maxRunning, "max-running", 0, "hold each job until fewer than N jobs run on this machine, in every instance. 0 lifts the cap jobs.max_running sets")
	fs.DurationVar(&o.maxRunningWait, "max-running-wait", 0, "with a running cap, wait this long for a running job to end rather than refusing at once. jobs.max_running_wait sets it for a project")
}

// passed says whether the command line named a flag.
func (o *ownerFlags) passed(name string) bool {
	found := false
	if o.fs != nil {
		o.fs.Visit(func(f *flag.Flag) { found = found || f.Name == name })
	}
	return found
}

// settle takes the project's defaults where the command line named none, and
// refuses the values that cannot mean what they say.
//
// ⛔ THE COMMAND LINE WINS, 0 included, as it does for --gitignore.
func (o *ownerFlags) settle(cfg toolkit.Config, lifecycle string) error {
	o.lifecycle = lifecycle
	if !o.passed("max-running") && cfg.Jobs.MaxRunning > 0 {
		o.maxRunning, o.capFromConfig = cfg.Jobs.MaxRunning, true
	}
	if !o.passed("max-running-wait") {
		wait, err := cfg.Jobs.RunningWait()
		if err != nil {
			return err
		}
		o.maxRunningWait = wait
	}
	if o.maxRunning < 0 {
		return fmt.Errorf("--max-running %d is negative. Pass 0 for no cap, or the most jobs that may run at once", o.maxRunning)
	}
	if o.maxRunningWait < 0 {
		return fmt.Errorf("--max-running-wait %s is negative. Pass 0 to refuse at once, or a positive duration", o.maxRunningWait)
	}
	if o.maxRunningWait > 0 && o.maxRunning == 0 && o.passed("max-running-wait") {
		return errors.New("--max-running-wait waits for a running job to end, and no --max-running or jobs.max_running is in force")
	}
	return nil
}

// helperRefusal answers a flag the helper route cannot honour, as
// helperRouteRefusal does for --exclusive.
//
// ⛔ THE HELPER OWNS A JOB IT RUNS. A watcher started here would watch a process
// that owns nothing, and a cap held here would be held by no process at all.
//
// ⚠ A CAP THE PROJECT'S FILE SET IS DROPPED ON THIS ROUTE, WITH A NOTE, and the
// job runs. A sandboxed caller reaches wsl.exe only through the helper, and a
// refusal there stopped every job of a project that set the key. WSL-136.
func (o *ownerFlags) helperRefusal(viaHelper bool) error {
	route := "this process cannot reach wsl.exe, so the job would go through the helper, which owns it"
	if viaHelper {
		route = "a job through the helper is owned by the helper"
	}
	switch {
	case o.cancelOnExit:
		return errors.New("--cancel-on-exit starts a watcher beside the process that owns the job, and " + route + ". Run it where wsl.exe can be called")
	case o.maxRunning > 0 && o.capFromConfig:
		note("jobs.max_running is held by the process that owns the job, and " + route + ", so the cap does not hold this job")
		o.maxRunning, o.maxRunningWait, o.capFromConfig = 0, 0, false
	case o.maxRunning > 0:
		return errors.New("--max-running is held by the process that owns the job, and " + route + ". Run it where wsl.exe can be called")
	}
	return nil
}

// closeLostOwners closes the open records whose owner is gone, as a reader
// finds them, and says so. WSL-121.
func closeLostOwners(home string, ids ...string) {
	closed, err := toolkit.CloseLostOwners(home, ids...)
	if err != nil {
		logf("  ! the open records of jobs whose owner was lost could not be closed: %s", err.Error())
		return
	}
	for _, id := range closed {
		logf("  job %s: its owner was lost, so its open record is closed", id)
	}
}

// -- --cancel-on-exit --------------------------------------------------------

// callersOf is the chain of processes the job is tied to: the one handed over
// by the client that started this detached owner, or this process's own.
func callersOf() ([]toolkit.ProcessIdentity, error) {
	if os.Getenv(toolkit.DetachedEnv) != "" {
		return toolkit.DecodeWatched(os.Getenv(toolkit.CallersEnv))
	}
	return toolkit.Callers()
}

// callersEnv is the chain a detaching client hands to the owner it starts.
//
// ⛔ THE CLIENT'S CHAIN AND NOT THE OWNER'S. The owner's own parent is the
// client, which answers with the job's id and exits at once, so a watch over
// it would stop every job in its first second.
//
// ⚠ THE CLIENT IS HANDED OVER TOO, as itself. Its exit is no stop: the watcher
// then drops each nearest caller that ended with it, as the stub Git Bash runs
// a Windows program under does. WSL-121.
func callersEnv() ([]toolkit.ProcessIdentity, string, error) {
	callers, err := toolkit.Callers()
	if err != nil {
		return nil, "", fmt.Errorf("--cancel-on-exit could not read which process started this one: %w", err)
	}
	client, err := toolkit.Self()
	if err != nil {
		return nil, "", fmt.Errorf("--cancel-on-exit: this process could not be named to its owner: %w", err)
	}
	client.Role = toolkit.RoleClient
	value, err := toolkit.EncodeWatched(append([]toolkit.ProcessIdentity{client}, callers...))
	if err != nil {
		return nil, "", err
	}
	return callers, toolkit.CallersEnv + "=" + value, nil
}

// watcherJobObjectWarning is what a caller learns when the watcher could not
// leave the Windows job object this process runs in.
const watcherJobObjectWarning = "the job object this process runs in refused to release the --cancel-on-exit watcher, so a kill of that object ends the watcher with it, and the job then ends at its deadline"

// startWatchdog starts the watcher for --cancel-on-exit beside the process
// that owns ids: a detached copy of this program, under the same selection,
// waiting on this process and on callers. WSL-121.
//
// ⚠ IT IS STARTED WITH THE SELECTION AND NOTHING ELSE. `--instance auto`
// resolved here would resolve again there, to another instance, so the name it
// resolved to is what it is given.
func startWatchdog(ids []string, callers []toolkit.ProcessIdentity) error {
	self, err := toolkit.Self()
	if err != nil {
		return fmt.Errorf("--cancel-on-exit: this process could not be named to its watcher: %w", err)
	}
	value, err := toolkit.EncodeWatched(append([]toolkit.ProcessIdentity{self}, callers...))
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	var args []string
	if toolkit.ExplicitConfigPath != "" {
		args = append(args, "--config", toolkit.ExplicitConfigPath)
	}
	if name := toolkit.SelectedInstance.Name; name != toolkit.DefaultInstance {
		args = append(args, "--instance", name)
	}
	env := withoutEnv(os.Environ(), toolkit.JobIDEnv, toolkit.DetachedEnv, toolkit.CallersEnv, toolkit.WatchdogEnv, toolkit.WatchedEnv)
	cmd, left, err := toolkit.StartDetached(func() *exec.Cmd {
		c := exec.Command(exe, args...)
		c.Env = append(env, toolkit.WatchdogEnv+"="+toolkit.WatchdogValue(ids), toolkit.WatchedEnv+"="+value)
		return c
	})
	if err != nil {
		return fmt.Errorf("--cancel-on-exit: the watcher could not start: %w", err)
	}
	_ = cmd.Process.Release()
	if !left {
		logf("  ! %s", watcherJobObjectWarning)
	}
	logf("  %s", cancelLine(strings.Join(ids, ", "), callers))
	return nil
}

// cancelLine names what a watcher waits on, for the line that says so.
func cancelLine(ids string, all []toolkit.ProcessIdentity) string {
	var callers []toolkit.ProcessIdentity
	for _, c := range all {
		if c.Role != toolkit.RoleClient {
			callers = append(callers, c)
		}
	}
	if len(callers) == 0 {
		return "--cancel-on-exit: job " + ids + " stops if its owner exits first. No process above this one could be waited on"
	}
	names := make([]string, 0, len(callers))
	for _, c := range callers {
		names = append(names, fmt.Sprintf("process %d (%s)", c.PID, c.Name))
	}
	return "--cancel-on-exit: job " + ids + " stops if its owner, or " + strings.Join(names, ", or ") + ", exits first"
}

// withoutEnv is an environment with some names left out.
func withoutEnv(env []string, names ...string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, n := range names {
			drop = drop || strings.EqualFold(name, n)
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

// runWatchdog is this program as the watcher of --cancel-on-exit. Its own
// streams go nowhere: what it does is in the ledger's stop records and in the
// jobs' results. WSL-121.
func runWatchdog(ctx context.Context, ids []string) int {
	watched, err := toolkit.DecodeWatched(os.Getenv(toolkit.WatchedEnv))
	_ = os.Unsetenv(toolkit.WatchdogEnv)
	_ = os.Unsetenv(toolkit.WatchedEnv)
	if err != nil || len(watched) == 0 {
		return exitCannot
	}
	home, err := toolkit.Home()
	if err != nil {
		return exitCannot
	}
	watch, err := toolkit.WatchProcesses(watched)
	if err != nil {
		return exitCannot
	}
	defer watch.Close()
	var runner *toolkit.Runner
	runnerFor := func() *toolkit.Runner {
		if runner == nil {
			if cfg, err := loadConfig(); err == nil {
				runner, _ = toolkit.NewRunner(cfg, note)
			}
		}
		return runner
	}
	w := toolkit.Watchdog{
		Home: home, IDs: ids, Watch: watch, Grace: watchGrace, Log: note,
		Stop: func(ctx context.Context, id, why string) (bool, error) {
			r := runnerFor()
			if r == nil {
				// The stop is still recorded, so an owner that is alive ends the job.
				return false, toolkit.RecordStop(home, id, why)
			}
			rep, err := r.StopForCancelOnExit(ctx, id, watchGrace, why)
			return rep.ContainerFound(), err
		},
		Sweep: func(ctx context.Context, id string) (bool, error) {
			r := runnerFor()
			if r == nil {
				return false, errors.New("the base could not be reached")
			}
			rep, err := r.StopContainerOf(ctx, id, watchGrace)
			return rep.ContainerFound(), err
		},
		SweepFor: sweepFor, SweepEvery: sweepEvery,
		Watched: watched, Rewatch: toolkit.WatchProcesses,
	}
	if _, err := w.Run(ctx); err != nil {
		return exitFailed
	}
	return exitOK
}

// rowIDs draws the ids a fleet's rows run under, so a watcher knows them
// before any row is claimed.
func rowIDs(n int) ([]string, error) {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		var raw [8]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return nil, fmt.Errorf("no cryptographic randomness for a row's id: %w", err)
		}
		out = append(out, hex.EncodeToString(raw[:]))
	}
	return out, nil
}

// -- the running cap ---------------------------------------------------------

// admit holds a claimed job until the machine runs fewer jobs than the cap,
// and ends it when it is refused or stopped while it waits. waited is how long
// it was held, ended is true where the job is over, and code and err are then
// the answer. WSL-136.
func (o *ownerFlags) admit(ctx context.Context, owner *toolkit.JobOwner, label, ref string, asJSON bool) (waited time.Duration, ended bool, code int, err error) {
	if o.maxRunning <= 0 {
		return 0, false, exitOK, nil
	}
	waited, err = toolkit.AdmitRunning(ctx, owner, o.maxRunning, o.maxRunningWait, note,
		func() bool { return toolkit.StopRecorded(owner.ID) })
	switch {
	case err == nil:
		return waited, false, exitOK, nil
	case errors.Is(err, toolkit.ErrStoppedWaitingToRun):
		res := toolkit.JobResult{ID: owner.ID, Label: label, Image: ref, Exit: 130, Cancelled: true, Stopped: true, Error: err.Error(), WaitedToRun: waited}
		res.Seal()
		o.finish(owner, &res)
		code, err = reportJob(res, asJSON)
		return waited, true, code, err
	}
	res := toolkit.JobResult{ID: owner.ID, Label: label, Image: ref, Exit: exitCannot, Unreached: true, Error: err.Error(), EffectiveExit: exitCannot, WaitedToRun: waited}
	o.finish(owner, &res)
	return waited, true, exitCannot, err
}

// rowAdmit is the cap as a fleet holds it: each row is held as its runner
// claims it, and nil is no cap. WSL-136.
//
// ⛔ A FLEET'S ROWS ARE JOBS THE CAP COUNTS, so a fleet is held by it too.
// jobs.max_running was read by run alone, and a project's matrix ran every
// row at once beside it.
func (o *ownerFlags) rowAdmit() func(context.Context, *toolkit.JobOwner) (time.Duration, error) {
	if o.maxRunning <= 0 {
		return nil
	}
	limit, wait := o.maxRunning, o.maxRunningWait
	// ⭐ THE FLEET'S OWN ROWS HOLD ITS NEXT ROW AND DO NOT REFUSE IT, so a cap
	// of 1 runs a matrix one row after another. WSL-136.
	var mu sync.Mutex
	rows := map[string]bool{}
	ours := func(j toolkit.RunningJob) bool {
		mu.Lock()
		defer mu.Unlock()
		return rows[j.ID]
	}
	return func(ctx context.Context, owner *toolkit.JobOwner) (time.Duration, error) {
		mu.Lock()
		rows[owner.ID] = true
		mu.Unlock()
		return toolkit.AdmitRunningBeside(ctx, owner, limit, wait, note,
			func() bool { return toolkit.StopRecorded(owner.ID) }, ours)
	}
}

// waitedLine says how long the cap held a job, or "" where it held it not at
// all. WSL-136.
func waitedLine(res toolkit.JobResult) string {
	if res.WaitedToRun <= 0 || res.ID == "" {
		return ""
	}
	return fmt.Sprintf("job %s waited %s for a running slot before it ran", res.ID, res.WaitedToRun.Round(time.Second))
}

// -- what a job leaves -------------------------------------------------------

// ⭐ AN EPHEMERAL JOB LEAVES NOTHING, as the help of --container-lifecycle has
// always said: its container, its guest directory and, once its answer is
// ready, its host record. A caller that needs the record runs the job
// persistent. WSL-121, as ruled.

// finish records a job's end and then removes what an ephemeral job leaves.
//
// ⛔ A DETACHED EPHEMERAL JOB KEEPS ITS RECORD, and says why before its end is
// recorded. Its answer is the record, which `wait ID` and `logs ID --follow`
// read after this process is gone.
func (o *ownerFlags) finish(owner *toolkit.JobOwner, res *toolkit.JobResult) {
	detached := os.Getenv(toolkit.DetachedEnv) != ""
	if detached && o.lifecycle == toolkit.ContainerEphemeral && res.ID != "" {
		res.RecordKept = toolkit.DetachedRecordKept
	}
	if err := owner.Finish(*res); err != nil {
		logf("  ! could not record the end of job %s: %s", res.ID, err.Error())
	}
	if !detached {
		o.leave(res)
	}
}

// leave removes an ended ephemeral job's host record, and says on the result
// what it did.
func (o *ownerFlags) leave(res *toolkit.JobResult) {
	if o.lifecycle != toolkit.ContainerEphemeral || res.ID == "" {
		return
	}
	home, err := toolkit.Home()
	if err != nil {
		res.RecordKept = "the state directory could not be named: " + err.Error()
		return
	}
	toolkit.RemoveJobRecord(home, o.lifecycle, res)
}

// leaveRows removes each ephemeral row's host record once the fleet's answer
// is ready, as one ephemeral job does. WSL-121.
func (o *ownerFlags) leaveRows(report *toolkit.MatrixReport) {
	if o.lifecycle != toolkit.ContainerEphemeral {
		return
	}
	removed := 0
	for i := range report.Rows {
		row := &report.Rows[i]
		o.leave(row)
		switch {
		case row.RecordRemoved:
			removed++
		case row.RecordKept != "":
			logf("  row %s keeps its host record: %s", row.Label, row.RecordKept)
		}
	}
	if removed > 0 {
		logf("  %d row(s) left nothing: their containers, guest directories and host records are removed", removed)
	}
}

// keptLine says exactly what an ended job leaves on this machine, or "" where
// another line already does. WSL-121.
//
// ⛔ AN EPHEMERAL JOB SAID ONLY "the complete output is kept". Its container and
// guest directory were removed and its host record was not, and a caller
// reading the line could not tell which.
func keptLine(res toolkit.JobResult) string {
	switch {
	case res.ID == "" || res.Unreached && res.Transcript == "" && !res.RecordRemoved:
		return ""
	case res.RecordRemoved:
		return "job " + res.ID + " left nothing: its container, its guest directory and its host record are removed"
	case res.RecordKept != "":
		return "the host record of job " + res.ID + " is kept: " + res.RecordKept + ". Remove it with: " + toolkit.ToolCommand("gc --job "+res.ID+" --apply")
	case res.ContainerLifecycle == toolkit.ContainerEphemeral && res.Container == "" && res.GuestDir == "" && res.Transcript != "":
		// ⚠ A helper of an earlier release removes no record.
		return "job " + res.ID + " removed its container and its guest directory. Its host record stays until " + toolkit.ToolCommand("gc --job "+res.ID+" --apply")
	}
	return ""
}
