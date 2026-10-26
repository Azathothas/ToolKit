// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// Job control from a second process. A job id reaches a running job, a detached
// one, and one whose owner is gone. WSL-94, WSL-95.

// jobHints are the three commands that reach a job or a session, spelled for
// the instance it belongs to.
//
// ⛔ THEY NAMED NO INSTANCE. A job started with --instance was followed, waited
// on and stopped in the default instance's store, which has never heard of it,
// so the command a caller was told to run answered "no such job". WSL-116.
func jobHints(id string) (follow, wait, stop string) {
	return toolkit.ToolCommand("logs " + id + " --follow"), toolkit.ToolCommand("wait " + id), toolkit.ToolCommand("stop " + id)
}

// errNotRegistered refuses a base command whose base is not built, naming the
// command that builds this instance's base and not the default one's. WSL-116.
func errNotRegistered(name string) error {
	return fmt.Errorf("%s is not registered. Build it with: %s", name, toolkit.ToolCommand("base ensure"))
}

const stopUsage = `wsl-toolkit stop JOB-ID

  Stop one job or one detached base session.

  A job's container gets TERM, then KILL once --grace passes, and the process
  that owns the job reports it stopped with exit 130. A job still being
  prepared never starts its container. A session's process group gets the same
  two signals, and so does its systemd scope where the base runs one, which
  holds a child that left the group. What an ended session left in its scope
  is ended the same way. --json says which reached it, as contained_by.

  --grace D      time between TERM and KILL (default 10s)
  --json         write a structured answer
  --via-helper   stop a job through the local helper

  Exit 0: the job is no longer running. 1: it could not be stopped.
  2: the id is not a job this state directory knows.
` + jobIDForms

// jobIDForms is what every command that takes a job id says about it. WSL-120.
const jobIDForms = `
  JOB-ID is the id a job printed, its container's name wtk-ID, or the
  first four or more characters of either when one job alone starts with
  them. A prefix that names none, or several, is refused with exit 2, and
  125 from wait and logs --follow.
`

func cmdStop(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("stop")
	grace := fs.Duration("grace", 10*time.Second, "time between TERM and KILL")
	asJSON := fs.Bool("json", false, "write a structured answer")
	viaHelper := fs.Bool("via-helper", false, "stop a job through the local helper")
	id, rest := splitLogsArgs(args)
	if err := parseArgs(fs, rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, stopUsage)
			return exitOK, nil
		}
		return exitCannot, err
	}
	if id == "" {
		return exitCannot, errors.New("stop takes a job id first: " + toolkit.ToolCommand("stop JOB-ID") + ". " + toolkit.ToolCommand("logs") + " lists the jobs this machine has")
	}
	// ⭐ A PREFIX OR A CONTAINER'S NAME IS READ AS THE JOB IT NAMES, and one
	// nothing here starts is handed to the helper, whose jobs are its own.
	// WSL-120.
	resolved, notHere, err := resolveRoutedJobID("stop", args, id)
	if err != nil {
		return exitCannot, err
	}
	id = resolved
	if notHere == nil && !toolkit.ValidJobID(id) {
		return exitCannot, fmt.Errorf("%q is not a job id: an id is 16 lowercase hex characters", id)
	}
	if *grace < 0 {
		return exitCannot, fmt.Errorf("--grace %s is negative", *grace)
	}
	home, err := toolkit.Home()
	if err != nil {
		return exitCannot, err
	}
	var rep toolkit.StopReport
	if rec, isSession, err := toolkit.ReadSessionRecord(home, id); err != nil {
		return exitCannot, err
	} else if isSession {
		if *viaHelper {
			return exitCannot, errors.New("a detached session runs a command in the base, which the helper protocol does not accept. Stop it through the session's WSL approval path")
		}
		w, err := toolkit.FindWsl()
		if err != nil {
			return exitCannot, err
		}
		before, err := toolkit.SessionNow(ctx, w, rec)
		if err != nil {
			return exitCannot, err
		}
		rep = toolkit.StopReport{Schema: toolkit.StopSchema, ID: id, Kind: "session", ContainedBy: rec.Containment(), Unit: rec.Unit}
		switch {
		case before.Gone:
			rep.Before, rep.Note = "gone", "the guest holds nothing for this session, so nothing was stopped"
		case !before.Alive:
			// ⛔ A SESSION THE BASE OUTLIVED IS SAID AS THAT, and nothing was
			// signalled: its ids may name another agent's processes now. WSL-184.
			rep.Before, rep.Note = "ended", goneNote(before, "the session had already ended")+", so nothing was stopped"
			v := before.Verdict()
			rep.Exit = &v
			// ⭐ WHAT THE SESSION LEFT IN ITS SCOPE IS STILL ITS OWN, and a stop
			// reaches it: a daemon the payload started and did not wait for.
			// WSL-126.
			//
			held, still, err := toolkit.StopSessionLeftovers(ctx, w, rec, *grace)
			applyLeftovers(&rep, held, still, err)
		default:
			after, lost, err := toolkit.StopSession(ctx, w, rec, *grace)
			if err != nil {
				return exitFailed, err
			}
			stoppedSession(&rep, after, lost)
		}
		return renderStop(rep, *asJSON)
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if c, err := useHelper(ctx, *viaHelper); notHere != nil && (err != nil || c == nil) {
		return exitCannot, notHere
	} else if err != nil {
		return exitCannot, err
	} else if c != nil {
		rep, err = c.StopJob(ctx, id, *grace)
		if toolkit.ValidJobID(rep.ID) {
			id = rep.ID
		} else if err != nil && notHere != nil {
			// ⛔ A HELPER THAT NAMED NO JOB FOR THE PREFIX LEAVES THE REFUSAL HERE.
			// A 7.0 helper refuses a prefix with a plain 400, which read as a failed
			// stop and exited 1, where 7.0.2 refused it here with exit 2. WSL-120.
			return exitCannot, notHere
		}
	} else {
		runner, rerr := toolkit.NewRunner(cfg, note)
		if rerr != nil {
			return exitCannot, rerr
		}
		rep, err = runner.StopJob(ctx, id, *grace)
	}
	if err != nil {
		if errors.Is(err, toolkit.ErrUnknownJob) {
			return exitCannot, err
		}
		return exitFailed, err
	}
	// ⭐ THE OWNER'S OWN ANSWER, where it gives one soon. It fetches /out and
	// tears down after the container stops, so its verdict follows the engine's.
	if rep.Stopped {
		if st := waitForResult(ctx, home, id, 20*time.Second); st.Result != nil {
			v := st.Result.Verdict()
			rep.Exit = &v
			rep.Note = "the owner recorded the job's end"
		} else if st.Owned {
			rep.Note = "the owner is still finishing: " + toolkit.ToolCommand("wait "+id)
		}
	}
	return renderStop(rep, *asJSON)
}

// waitForResult polls a job's host record for up to limit, for the owner's own
// verdict after a stop.
func waitForResult(ctx context.Context, home, id string, limit time.Duration) toolkit.JobState {
	deadline := time.Now().Add(limit)
	for {
		st, err := toolkit.ReadJobState(home, id)
		// ⚠ AN EPHEMERAL JOB'S RECORD GOES ONCE ITS ANSWER IS READY, and the
		// answer is read from the copy kept beside it. WSL-121.
		if res, ok := toolkit.EndedAnswer(home, id); err == nil && !st.Known && ok {
			st.Result = &res
		}
		if err != nil || st.Result != nil || !st.Owned || time.Now().After(deadline) {
			return st
		}
		select {
		case <-ctx.Done():
			return st
		case <-time.After(toolkit.FollowPoll):
		}
	}
}

func renderStop(rep toolkit.StopReport, asJSON bool) (int, error) {
	code := exitOK
	if rep.After == "running" {
		code = exitFailed
	}
	if asJSON {
		return code, writeJSON(rep)
	}
	line := fmt.Sprintf("  %s %s: it was %s", rep.Kind, rep.ID, rep.Before)
	if rep.Stopped {
		line += ", and it is stopped"
	}
	if rep.Exit != nil {
		line += fmt.Sprintf(". Exit %d", *rep.Exit)
	}
	logf("%s", line)
	if rep.Note != "" {
		logf("  %s", rep.Note)
	}
	if hint := containmentHint(rep); hint != "" {
		logf("  %s", hint)
	}
	return code, nil
}

// applyLeftovers puts what an ended session left in its scope on the stop's
// answer. WSL-126.
//
// ⚠ A SCOPE THAT CANNOT BE READ IS SAID, AND THE ENDED SESSION'S ANSWER STANDS:
// the session itself is not running either way.
//
// ⛔ after IS NOT SET, AND THE EXIT STAYS 0. A stop of an ended session exited 0
// and left after out in 7.0.0, and a caller reads both. What its scope still
// holds is said in leftovers alone, which is new.
func applyLeftovers(rep *toolkit.StopReport, held, still bool, err error) {
	if err != nil {
		rep.Note += ". What it left in its scope could not be read: " + err.Error()
	}
	if !held {
		return
	}
	rep.Leftovers = "ended"
	rep.Note = "the session had already ended, and the processes it left in its scope were ended"
	if still {
		rep.Leftovers = "running"
		rep.Note = "the session had already ended, and the processes it left in its scope did not end"
	}
}

// containmentHint says what a session's stop reached. WSL-126.
//
// ⛔ ONLY WHERE SOMETHING WAS SIGNALLED. A session that was gone, or had ended
// and left nothing in its scope, was not signalled, and the hint said it was.
func containmentHint(rep toolkit.StopReport) string {
	if !rep.Stopped && rep.Leftovers == "" {
		return ""
	}
	switch rep.ContainedBy {
	case toolkit.ContainedByScope:
		return "its scope " + rep.Unit + " was signalled, so every process the session started was reached, a child that called setsid included"
	case toolkit.ContainedByGroup:
		return "its process group was signalled, because no systemd user manager answered when it started. A child that called setsid is not reached"
	}
	return ""
}

const waitUsage = `wsl-toolkit wait JOB-ID

  Wait for one job or one detached base session to end, and answer with its
  exit code, as run would have. The job keeps running when this is stopped.

  --timeout D   stop waiting after D. The answer is then 124, and --json
                says the job is still running
  --json        write a structured answer
` + jobIDForms

func cmdWait(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("wait")
	timeout := fs.Duration("timeout", 0, "stop waiting after this long. 0 waits for as long as the job runs")
	asJSON := fs.Bool("json", false, "write a structured answer")
	id, rest := splitLogsArgs(args)
	if err := parseArgs(fs, rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, waitUsage)
			return exitOK, nil
		}
		return exitCannot, err
	}
	if id == "" {
		return exitCannot, errors.New("wait takes a job id first: " + toolkit.ToolCommand("wait JOB-ID"))
	}
	resolved, err := resolveJobID("wait", args, id)
	if err != nil {
		return exitCannot, err
	}
	id = resolved
	if *timeout < 0 {
		return exitCannot, fmt.Errorf("--timeout %s is negative. Pass 0 to wait for as long as the job runs", *timeout)
	}
	waitCtx := ctx
	if *timeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
	}
	end, err := jobEnd(waitCtx, id, 0, nil, nil)
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		if *asJSON {
			return exitTimeout, writeJSON(map[string]any{"schema": "wsl-toolkit-wait/1", "id": id, "running": true})
		}
		logf("  job %s is still running after %s. It keeps running; wait again or stop it with: %s", id, *timeout, toolkit.ToolCommand("stop "+id))
		return exitTimeout, nil
	}
	if err != nil {
		return jobEndCode(err), err
	}
	return renderEnd(end, *asJSON)
}

// jobEndOutcome is how one job ended, as the second process learns it.
type jobEndOutcome struct {
	Schema  string                `json:"schema"`
	ID      string                `json:"id"`
	Kind    string                `json:"kind"`
	Result  *toolkit.JobResult    `json:"result,omitempty"`
	Session *toolkit.SessionState `json:"session,omitempty"`
	// JobStarted says the job's payload, or the session's command, started.
	// ⭐ The job's own field, lifted, so a reader of a job and of a session reads
	// one place. WSL-127.
	JobStarted bool `json:"job_started"`
	// Orphaned is true where no owner recorded the end, and the exit is the
	// engine's own, where it has one.
	Orphaned bool   `json:"orphaned,omitempty"`
	Note     string `json:"note,omitempty"`
	// Artifacts is the delivery of a session's out directory, where it was
	// started with --artifacts. WSL-124.
	Artifacts *toolkit.SessionDelivery `json:"artifacts,omitempty"`
	verdict   int
}

// jobEnd follows or waits on one job. With writers it writes the output as it
// arrives; with none it only waits.
//
// ⭐ THE HOST'S RECORD FIRST, THE ENGINE LAST. A result the owner wrote is the
// answer. A running owner is followed through its transcript, which needs no
// wsl.exe. Only a job with no owner and no result goes to the engine.
func jobEnd(ctx context.Context, id string, tail int, stdout, stderr io.Writer) (jobEndOutcome, error) {
	out := jobEndOutcome{Schema: "wsl-toolkit-job-end/1", ID: id, Kind: "job"}
	if !toolkit.ValidJobID(id) {
		return out, fmt.Errorf("%q is not a job id: an id is 16 lowercase hex characters", id)
	}
	home, err := toolkit.Home()
	if err != nil {
		return out, err
	}
	st, err := toolkit.ReadJobState(home, id)
	if err != nil {
		return out, err
	}
	if !st.Known {
		// ⚠ A READER THAT CAME JUST AFTER THE RECORD WENT gets the answer kept
		// beside it, as one that came just before does. WSL-121.
		if res, ok := toolkit.EndedAnswer(home, id); ok {
			out.Result, out.verdict = &res, res.Verdict()
			return out, nil
		}
		return out, fmt.Errorf("%w: %s. %s lists the jobs this machine has", toolkit.ErrUnknownJob, id, toolkit.ToolCommand("logs"))
	}
	if st.Session != nil {
		return sessionEnd(ctx, *st.Session, tail, stdout, stderr)
	}
	if !st.Owned && st.Result == nil {
		closeLostOwners(home, id)
	}
	dir := filepath.Join(home, "jobs", id)
	if st.Result == nil && st.Owned {
		if stdout != nil {
			if err := toolkit.FollowTranscript(ctx, dir, tail, stdout, stderr); err != nil {
				return out, err
			}
		} else if err := waitOwnerGone(ctx, dir); err != nil {
			return out, err
		}
		if st, err = toolkit.ReadJobState(home, id); err != nil {
			return out, err
		}
		if st.Result != nil {
			out.Result, out.verdict = st.Result, st.Result.Verdict()
			return out, nil
		}
		// ⛔ AN OWNED JOB WHOSE RECORD WENT IS AN ENDED EPHEMERAL JOB, never an
		// orphan. Its owner removed the record once the answer was ready, and
		// this read came after, so the answer is the copy kept beside it. WSL-121.
		if res, ok := toolkit.EndedAnswer(home, id); ok && !st.Known {
			out.Result, out.verdict = &res, res.Verdict()
			return out, nil
		}
	} else if st.Result != nil {
		if stdout != nil {
			if err := writeBothTranscripts(dir, tail, stdout, stderr); err != nil {
				return out, err
			}
		}
		out.Result, out.verdict = st.Result, st.Result.Verdict()
		return out, nil
	}
	// No owner, and no result: the owner is gone without saying.
	out.Orphaned = true
	cfg, err := loadConfig()
	if err != nil {
		return out, err
	}
	runner, err := toolkit.NewRunner(cfg, note)
	if err != nil {
		return out, err
	}
	so, se := stdout, stderr
	if so == nil {
		so, se = io.Discard, io.Discard
	}
	exit, present, err := runner.FollowContainer(ctx, id, tail, so, se)
	if err != nil {
		return out, err
	}
	stoppedBy := ""
	if exit != nil && runner.EndedByStop(id, *exit) {
		stoppedBy = runner.StoppedBy(id)
	}
	res, verdict := orphanEnd(id, exit, present, stoppedBy)
	out.Note, out.verdict = res.Error, verdict
	if exit == nil && !present && stdout != nil {
		_ = writeBothTranscripts(dir, tail, stdout, stderr)
	}
	out.Result = &res
	// ⭐ AN END THE ENGINE REPORTED IS RECORDED where no owner recorded one, so a
	// second reader gets the same answer after the container is gone. The claim
	// fails where an owner took the job back, and nothing is written then.
	if exit != nil {
		if owner, err := toolkit.ClaimJob(home, id); err == nil {
			if err := owner.Finish(res); err != nil {
				logf("  ! could not record the end of job %s: %s", id, err.Error())
			}
		}
	}
	return out, nil
}

// orphanEnd is how a job with no owner and no result ended, from what the
// engine holds: its container's exit, whether the container is still there, and
// the stop that ended it, where one did.
//
// ⚠ AN EXIT THE ENGINE HOLDS IS A CONTAINER THAT RAN, so the job started. Its
// wrapper writes the marker before anything else. WSL-127.
//
// ⛔ AN END THAT WAS NOT RECORDED IS THIS TOOL'S FAILURE, not the job's. It
// answered 1, "ran and disagreed", with job_started false, where the direct
// route answers a lost end 125 with tool_failed. job_started is true there
// because the job may have run, and false would read a retry as safe. WSL-127.
func orphanEnd(id string, exit *int, present bool, stoppedBy string) (toolkit.JobResult, int) {
	res := toolkit.JobResult{ID: id, JobStarted: true}
	switch {
	case exit != nil && stoppedBy != "":
		res.Cancelled, res.Stopped, res.Exit = true, true, 130
		res.Error = "no process owned this job when " + stoppedBy + " ended its container"
	case exit != nil:
		res.Exit = *exit
		res.Error = "no process owned this job when it ended, so the exit is the container's own"
	case present:
		res.ToolFailed, res.Exit = true, exitCannot
		res.Error = "no process owned this job, and the engine holds no exit for its container"
	default:
		res.ToolFailed, res.Exit = true, exitCannot
		res.Error = "no process owned this job and its container is gone, so how it ended was not recorded"
	}
	res.Seal()
	return res, res.EffectiveExit
}

// waitOwnerGone waits for a job's owner to let its lock go.
func waitOwnerGone(ctx context.Context, dir string) error {
	return toolkit.FollowTranscript(ctx, dir, -1, io.Discard, io.Discard)
}

// sessionEnd follows or waits on one detached base session.
func sessionEnd(ctx context.Context, rec toolkit.SessionRecord, tail int, stdout, stderr io.Writer) (jobEndOutcome, error) {
	out := jobEndOutcome{Schema: "wsl-toolkit-job-end/1", ID: rec.ID, Kind: "session"}
	w, err := toolkit.FindWsl()
	if err != nil {
		return out, err
	}
	so, se := stdout, stderr
	if so == nil {
		so, se = io.Discard, io.Discard
	}
	st, err := toolkit.FollowSession(ctx, w, rec, tail, so, se)
	if err != nil {
		return out, err
	}
	out.Session = &st
	out.verdict = st.Verdict()
	switch {
	case st.Gone:
		out.Note, out.verdict = "the guest holds nothing for this session", exitFailed
	case st.Exit == nil:
		out.Note = goneNote(st, "the session's supervisor is gone and no exit was recorded")
	}
	// ⭐ THE SESSION'S OUTPUT IS DELIVERED HERE, by whoever reads its end, which
	// is what makes `base exec --detach --artifacts` the attached command with
	// nobody waiting. ⛔ A delivery that failed fails a command that succeeded, as
	// a job's does. WSL-124.
	if rec.ArtifactDir != "" && !st.Alive {
		home, err := toolkit.Home()
		if err != nil {
			return out, err
		}
		out.Artifacts = toolkit.DeliverSessionArtifacts(ctx, w, home, rec, st)
		out.verdict = deliveryVerdict(out.verdict, out.Artifacts)
	}
	return out, nil
}

// stoppedSession fills a stop's report from what the stop of a session read as
// running found, and the state after it.
//
// ⛔ A STOP THAT FOUND THE SESSION LOST SIGNALLED NOTHING, AND SAYS SO. The stop
// script reads the session again before it sends anything, and over a session
// the base outlived it prints LOST and exits 0. That was reported as `running`,
// then stopped. WSL-184.
func stoppedSession(rep *toolkit.StopReport, after toolkit.SessionState, lost string) {
	if lost != "" {
		after.Alive, after.Lost = false, lost
		rep.Before, rep.Note = "ended", goneNote(after, "the session had already ended")+", so nothing was stopped"
		v := after.Verdict()
		rep.Exit = &v
		return
	}
	rep.Before, rep.Stopped = "running", true
	rep.After = "ended"
	if after.Alive {
		rep.After = "running"
	}
	v := after.Verdict()
	rep.Exit = &v
}

// goneNote is what a reader is told about a session that is not running and
// recorded no exit: why, where the base outlived it, and otherwise as given.
// WSL-184.
func goneNote(st toolkit.SessionState, otherwise string) string {
	if why := st.LostReason(); why != "" {
		return why
	}
	return otherwise
}

// deliveryVerdict is a session's exit once its output was delivered, or was not:
// the command's own failure first, and a failed delivery of a command that
// succeeded is 1, as a job's is. WSL-124.
func deliveryVerdict(verdict int, d *toolkit.SessionDelivery) int {
	if d != nil && d.Error != "" && verdict == exitOK {
		return exitFailed
	}
	return verdict
}

// artifactsNote is the line a reader gets about a session's delivered output.
func artifactsNote(d *toolkit.SessionDelivery) string {
	switch {
	case d == nil:
		return ""
	case d.Error != "":
		return "! artifacts: " + d.Error
	case d.Earlier:
		return fmt.Sprintf("artifacts: an earlier wait delivered %d entries to %s", d.Delivered, d.To)
	}
	return toolkit.ArtifactsLine(toolkit.ArtifactTransfer{Delivered: d.Delivered, Bytes: d.Bytes}, d.To)
}

// jobEndCode is the exit for a job-control call that could not answer.
func jobEndCode(err error) int {
	if errors.Is(err, toolkit.ErrUnknownJob) {
		return exitCannot
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return exitCannot
}

func renderEnd(end jobEndOutcome, asJSON bool) (int, error) {
	if asJSON {
		// ⛔ RESEALED HERE, as reportJob seals at the point of rendering. The
		// owner sealed the record under its own --tool-exit, or a 7.x owner
		// sealed an unreached job at 2, so effective_exit disagreed with the
		// code this wait answers. A copy, so the record stays as written. WSL-127.
		if end.Result != nil {
			sealed := *end.Result
			sealed.Seal()
			end.Result = &sealed
		}
		end.JobStarted = endStarted(end)
		return end.verdict, writeJSON(end)
	}
	switch {
	case end.Result != nil && end.Result.Stopped:
		logf("  job %s was stopped. Exit %d", end.ID, end.verdict)
	case end.Result != nil:
		logf("  job %s ended. Exit %d", end.ID, end.verdict)
	case end.Session != nil:
		logf("  session %s ended. Exit %d", end.ID, end.verdict)
	}
	if end.Note != "" {
		logf("  %s", end.Note)
	}
	if line := artifactsNote(end.Artifacts); line != "" {
		logf("  %s", line)
	}
	return end.verdict, nil
}

// endStarted says whether the job or the session a wait read had started.
// WSL-127.
//
// ⚠ A SESSION THAT HAS A RECORD STARTED. Its record is written once its
// supervisor is running, and a command that could not enter its directory is
// still a command that ran and answered.
func endStarted(end jobEndOutcome) bool {
	switch {
	case end.Result != nil:
		return end.Result.JobStarted
	case end.Session != nil:
		return true
	}
	return false
}

// writeBothTranscripts writes a job's two recorded streams, each to its own.
func writeBothTranscripts(dir string, tail int, stdout, stderr io.Writer) error {
	if _, err := copyTranscript(filepath.Join(dir, "stdout.log"), tail, stdout); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := copyTranscript(filepath.Join(dir, "stderr.log"), tail, stderr); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
