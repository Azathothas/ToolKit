// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"io"
	"os"
	"path/filepath"
	"sync"
)

// ⛔ WHAT WAS WRONG. A job's stdout went into an 8 MiB buffer and its stderr into
// a 2 MiB one, the overflow was dropped, each buffer remembered privately that it
// had dropped something, and nothing ever read that flag. A command writing 9 MiB
// got exactly 8,388,608 bytes back, exit 0, and no field saying so. The second
// half was worse for a caller who could not see it: nothing was written anywhere
// until the job was over, so a twelve-image fleet was seventy seconds of silence
// and a wedged compiler looked exactly like a working one.
//
// ⭐ THREE SINKS, ONE WRITE. The live one is the caller's own stream and gets the
// bytes as they arrive. The spool is a file and is not bounded, so the complete
// transcript survives whatever the memory copy did. The bounded copy is for the
// structured answer, where an unbounded string would be a JSON document nobody
// can hold, and it now says when it was cut.

// jobStream is one of a job's two output streams, written to every sink at once.
type jobStream struct {
	mu    sync.Mutex
	live  io.Writer // the caller's own stream, when one asked. May be nil.
	spool *os.File  // the complete transcript. May be nil.
	buf   *boundedBuffer
	// total is every byte the command produced, whatever any sink kept.
	total int64
	// spoilt records a sink that failed, so a partial transcript is never
	// reported as a complete one.
	spoilt string
}

func (s *jobStream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total += int64(len(p))
	// ⛔ THE BOUNDED COPY CANNOT FAIL THE WRITE. Its whole job is to stop early,
	// so an error from it is not a reason to tell the command its output could
	// not be written.
	_, _ = s.buf.Write(p)
	if s.spool != nil {
		if _, err := s.spool.Write(p); err != nil && s.spoilt == "" {
			s.spoilt = "the transcript could not be written: " + err.Error()
		}
	}
	if s.live != nil {
		if _, err := s.live.Write(p); err != nil && s.spoilt == "" {
			s.spoilt = "the live stream could not be written: " + err.Error()
		}
	}
	return len(p), nil
}

// DefaultOutputLimits are what an answer keeps of each stream when a caller
// names no ceiling.
//
// ⭐ They are a MEMORY bound on the structured answer, and no longer a
// bound on the output itself: the transcript beside the job is complete, and the
// result says when these two cut anything.
func DefaultOutputLimits() (stdout, stderr int64) { return 8 << 20, 2 << 20 }

// jobStreams is a job's pair, plus the directory holding their transcripts.
type jobStreams struct {
	Out *jobStream
	Err *jobStream
	Dir string
}

// newJobStreams opens the sinks for one job.
//
// ⚠ A SPOOL THAT CANNOT BE OPENED IS NOT A REASON TO REFUSE THE JOB. The bounded
// copy and the live stream still work, and the result says the transcript is
// missing rather than pretending it is there.
func newJobStreams(home, id string, liveOut, liveErr io.Writer, maxOutput int64, log func(string)) *jobStreams {
	outMax, errMax := DefaultOutputLimits()
	if maxOutput > 0 {
		// ⚠ stderr gets a quarter, as it does by default. A caller raising
		// the ceiling for a build log does not want four times as much room for
		// a compiler's warnings as well.
		outMax, errMax = maxOutput, maxOutput/4+1
	}
	s := &jobStreams{
		Out: &jobStream{live: liveOut, buf: &boundedBuffer{max: int(outMax)}},
		Err: &jobStream{live: liveErr, buf: &boundedBuffer{max: int(errMax)}},
	}
	if home == "" || id == "" {
		return s
	}
	dir := filepath.Join(home, "jobs", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		if log != nil {
			log("no transcript for this job: " + err.Error())
		}
		return s
	}
	s.Dir = dir
	s.Out.spool = openSpool(dir, "stdout.log", log)
	s.Err.spool = openSpool(dir, "stderr.log", log)
	return s
}

func openSpool(dir, name string, log func(string)) *os.File {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		if log != nil {
			log("no " + name + " for this job: " + err.Error())
		}
		return nil
	}
	return f
}

// Close finishes both transcripts.
func (s *jobStreams) Close() {
	for _, st := range []*jobStream{s.Out, s.Err} {
		st.mu.Lock()
		if st.spool != nil {
			if err := st.spool.Close(); err != nil && st.spoilt == "" {
				st.spoilt = "the transcript could not be closed: " + err.Error()
			}
			st.spool = nil
		}
		st.mu.Unlock()
	}
}

// Apply puts what the streams saw onto the result.
//
// ⭐ Bytes AND truncation, separately. A caller reading `stdout_bytes` against
// `len(stdout)` can tell how much is missing; a caller reading `stdout_truncated`
// can tell THAT something is, which is the question a test asks.
func (s *jobStreams) Apply(res *JobResult) {
	res.Stdout, res.Stderr = s.Out.buf.String(), s.Err.buf.String()
	res.StdoutBytes, res.StderrBytes = s.Out.total, s.Err.total
	res.StdoutTruncated = s.Out.buf.Truncated()
	res.StderrTruncated = s.Err.buf.Truncated()
	if s.Dir != "" {
		res.Transcript = s.Dir
	}
	for _, st := range []*jobStream{s.Out, s.Err} {
		if st.spoilt != "" && res.Error == "" {
			res.Error = st.spoilt
		}
	}
}
