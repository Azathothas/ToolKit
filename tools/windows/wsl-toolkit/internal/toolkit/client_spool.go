// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"io"
	"os"
	"path/filepath"
)

// ⛔ WHY THE CLIENT SPOOLS TOO. A job run through the helper has its transcript
// written by the HELPER, under the helper's own state directory. Where a client
// and a helper share one `WSL_TOOLKIT_HOME` that is the same path and `logs`
// finds it; where they do not, the result named a directory that did not exist
// on the machine the caller was sitting at, and the recovery this tool
// advertises would have failed for exactly the restricted caller the helper
// exists for.
//
// ⭐ The bytes are already arriving. The client is decoding stdout and stderr
// events to write them to its own streams, so writing them to a file at the same
// time costs one more sink and no round trip.

// ClientSpool captures a streamed job's output on the machine that asked for it.
type ClientSpool struct {
	home   string
	dir    string
	out    *os.File
	err    *os.File
	ledger *Ledger
	id     string
	log    func(string)
}

// NewClientSpool opens a staging directory for one streamed job.
//
// ⚠ IT IS NEVER A REASON TO FAIL A JOB. A nil spool writes nothing and every
// method tolerates it, so a caller with no writable state directory still runs
// its job and simply has no local transcript.
//
// ⛔ IT SAYS WHY, THOUGH. Every one of these four returns used to be silent,
// so a job ran, succeeded, and `wsl-toolkit logs` found nothing, with no line
// anywhere saying which step gave up. The direct route's openSpool has always
// logged the same failure, so one route told the operator and the other did not.
// A degradation nobody is told about is the defect reported as issue #10, in a
// different place.
func NewClientSpool(home string, ledger *Ledger, log func(string)) *ClientSpool {
	gaveUp := func(why string, err error) *ClientSpool {
		if log != nil {
			log("no local transcript for this job: " + why + ": " + err.Error())
		}
		return nil
	}
	if home == "" {
		if log != nil {
			log("no local transcript for this job: this machine has no state directory")
		}
		return nil
	}
	id, err := newJobID()
	if err != nil {
		return gaveUp("a job id could not be generated", err)
	}
	dir := filepath.Join(home, "incoming", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return gaveUp("the staging directory could not be made", err)
	}
	s := &ClientSpool{home: home, dir: dir, ledger: ledger, id: id, log: log}
	// ⭐ The SAME opener the direct route uses, so the two cannot drift again.
	s.out = openSpool(dir, "stdout.log", log)
	s.err = openSpool(dir, "stderr.log", log)
	// ⛔ Recorded before it is written to, like every other resource here, so a
	// client that was killed mid-job leaves something cleanup can find. The
	// record also keeps a concurrent `gc --apply` from removing it underneath a
	// job that is still running.
	if ledger != nil {
		_ = ledger.Append(LedgerEntry{Event: "open", Kind: "incoming", ID: id, HostDir: dir})
	}
	return s
}

// Tee returns a writer that feeds both the caller's own stream and the spool.
//
// ⚠ A nil live writer is normal: under --json this process's stdout carries the
// answer, so there is nothing live to feed and the spool is the only sink.
func (s *ClientSpool) Tee(live io.Writer, stderr bool) io.Writer {
	if s == nil {
		return live
	}
	f := s.out
	if stderr {
		f = s.err
	}
	if f == nil {
		return live
	}
	if live == nil {
		return &bestEffort{w: f}
	}
	// ⛔ NOT io.MultiWriter. It stops at the first sink that errors and
	// returns that error, so a full disk on THIS machine would abort a job
	// running on the helper and throw away a result that had already been
	// produced. The spool is a convenience and cannot fail the work.
	return &teeToSpool{live: live, spool: f}
}

// teeToSpool writes to both and reports only the live one's outcome.
type teeToSpool struct {
	live  io.Writer
	spool io.Writer
}

func (t *teeToSpool) Write(p []byte) (int, error) {
	_, _ = t.spool.Write(p)
	return t.live.Write(p)
}

// bestEffort swallows a sink's errors, for a spool with no live stream beside
// it. ⚠ A short write is reported as complete on purpose: the caller here
// is the protocol reader, and telling it the stream failed would abandon a job
// over a file nobody asked for.
type bestEffort struct{ w io.Writer }

func (b *bestEffort) Write(p []byte) (int, error) {
	_, _ = b.w.Write(p)
	return len(p), nil
}

// Finish files the spool under the job's own id and returns the path a caller
// can read it back from.
//
// ⚠ IT YIELDS TO A TRANSCRIPT THAT IS ALREADY THERE. When the client and the
// helper share a state directory, the helper has already written the same bytes
// under the same id; overwriting would replace a complete transcript with one
// that starts wherever this client began reading.
func (s *ClientSpool) Finish(jobID string) string {
	if s == nil {
		return ""
	}
	s.close()
	defer s.release()
	if jobID == "" {
		// ⚠ NO ID MEANS NOTHING TO FILE IT UNDER, and leaving the staging
		// directory would be a leak on every job whose result did not carry
		// one. It goes, rather than waiting for cleanup to notice it.
		if s.dir != "" {
			_ = RemoveInside(s.home, s.dir)
			s.dir = ""
		}
		return ""
	}
	dest := filepath.Join(s.home, "jobs", jobID)
	if _, err := os.Stat(dest); err == nil {
		if err := RemoveInside(s.home, s.dir); err == nil {
			s.dir = ""
		}
		return dest
	}
	// ⛔ A FAILURE TO FILE IT IS NOT A REASON TO DENY IT EXISTS. Both of these
	// returned "" while a complete transcript sat in the staging directory, so
	// the caller reported no transcript for a job whose output was on this disk.
	// The staging path is returned instead: it is under the same state directory,
	// `logs` reads it, and gc collects it by age like anything else. The reason is
	// logged rather than swallowed, because a path in `incoming/` instead of
	// `jobs/` is a surprise an operator should be able to explain.
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return s.unfiled("the jobs directory could not be made", err)
	}
	if err := os.Rename(s.dir, dest); err != nil {
		return s.unfiled("the transcript could not be filed under the job id", err)
	}
	s.dir = ""
	return dest
}

// unfiled reports why a transcript stayed where it was written, and returns that
// place. ⚠ s.dir is deliberately NOT cleared: the directory is still there and
// the path being returned points at it.
func (s *ClientSpool) unfiled(why string, err error) string {
	if s.log != nil {
		s.log("the transcript stayed in " + s.dir + ": " + why + ": " + err.Error())
	}
	return s.dir
}

// Discard throws the spool away, for a job that never produced a result.
func (s *ClientSpool) Discard() {
	if s == nil {
		return
	}
	s.close()
	if s.dir != "" {
		_ = RemoveInside(s.home, s.dir)
		s.dir = ""
	}
	s.release()
}

func (s *ClientSpool) close() {
	for _, f := range []*os.File{s.out, s.err} {
		if f != nil {
			_ = f.Close()
		}
	}
	s.out, s.err = nil, nil
}

func (s *ClientSpool) release() {
	if s.ledger != nil && s.id != "" {
		_ = s.ledger.Append(LedgerEntry{Event: "close", Kind: "incoming", ID: s.id})
		s.id = ""
	}
}
