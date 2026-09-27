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

const stopUsage = `wsl-toolkit stop JOB-ID

  Stop one job or one detached base session.

  A job's container gets TERM, then KILL once --grace passes, and the process
  that owns the job reports it stopped with exit 130. A job still being
  prepared never starts its container. A session's process group gets the same
  two signals.

  --grace D      time between TERM and KILL (default 10s)
  --json         write a structured answer
  --via-helper   stop a job through the local helper

  Exit 0: the job is no longer running. 1: it could not be stopped.
  2: the id is not a job this state directory knows.
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
		return exitCannot, errors.New("stop takes a job id first: wsl-toolkit stop JOB-ID. wsl-toolkit logs lists the jobs this machine has")
	}
	if !toolkit.ValidJobID(id) {
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
		rep = toolkit.StopReport{Schema: toolkit.StopSchema, ID: id, Kind: "session"}
		switch {
		case before.Gone:
			rep.Before, rep.Note = "gone", "the guest holds nothing for this session, so nothing was stopped"
		case !before.Alive:
			rep.Before, rep.Note = "ended", "the session had already ended, so nothing was stopped"
			v := before.Verdict()
			rep.Exit = &v
		default:
			rep.Before, rep.Stopped = "running", true
			after, err := toolkit.StopSession(ctx, w, rec, *grace)
			if err != nil {
				return exitFailed, err
			}
			rep.After = "ended"
			if after.Alive {
				rep.After = "running"
			}
			v := after.Verdict()
			rep.Exit = &v
		}
		return renderStop(rep, *asJSON)
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if c, err := useHelper(ctx, *viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		rep, err = c.StopJob(ctx, id, *grace)
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
			rep.Note = "the owner is still finishing: wsl-toolkit wait " + id
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
	return code, nil
}

const waitUsage = `wsl-toolkit wait JOB-ID

  Wait for one job or one detached base session to end, and answer with its
  exit code, as run would have. The job keeps running when this is stopped.

  --timeout D   stop waiting after D. The answer is then 124, and --json
                says the job is still running
  --json        write a structured answer
`

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
		return exitCannot, errors.New("wait takes a job id first: wsl-toolkit wait JOB-ID")
	}
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
		logf("  job %s is still running after %s. It keeps running; wait again or stop it with: wsl-toolkit stop %s", id, *timeout, id)
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
	// Orphaned is true where no owner recorded the end, and the exit is the
	// engine's own, where it has one.
	Orphaned bool   `json:"orphaned,omitempty"`
	Note     string `json:"note,omitempty"`
	verdict  int
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
		return out, fmt.Errorf("%w: %s. wsl-toolkit logs lists the jobs this machine has", toolkit.ErrUnknownJob, id)
	}
	if st.Session != nil {
		return sessionEnd(ctx, *st.Session, tail, stdout, stderr)
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
	res := toolkit.JobResult{ID: id}
	switch {
	case exit != nil && runner.EndedByStop(id, *exit):
		res.Cancelled, res.Stopped, res.Exit = true, true, 130
		out.Note = "no process owned this job when wsl-toolkit stop ended its container"
		out.verdict = 130
	case exit != nil:
		res.Exit = *exit
		out.Note = "no process owned this job when it ended, so the exit is the container's own"
		out.verdict = *exit
	case present:
		res.Exit, out.verdict = 1, exitFailed
		out.Note = "no process owned this job, and the engine holds no exit for its container"
	default:
		res.Exit, out.verdict = 1, exitFailed
		out.Note = "no process owned this job and its container is gone, so how it ended was not recorded"
		if stdout != nil {
			_ = writeBothTranscripts(dir, tail, stdout, stderr)
		}
	}
	res.Error = out.Note
	res.Seal()
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
		out.Note = "the session's supervisor is gone and no exit was recorded"
	}
	return out, nil
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
	return end.verdict, nil
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
