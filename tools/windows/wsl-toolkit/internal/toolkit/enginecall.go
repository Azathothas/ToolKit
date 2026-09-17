// enginecall.go - the one door every host-engine invocation goes through, and
// the deadline it cannot be run without.
//
// ⛔ WHAT WAS WRONG, AND IT COST 28 MINUTES OF A SESSION. Every call to podman
// carried a deadline chosen at its own call site: 90 seconds for `info`, five
// minutes for `create`, thirty minutes for `pull`, and whatever the caller
// happened to hold for the rest. That is a guard applied at six places, which
// RULES.md section 3 already says is a guard that will one day be applied at
// five - and the largest of them was a ceiling nobody would ever sit through.
// Measured on 2026-09-17: `podman pull` stopped dead with ZERO bytes read, ZERO
// written and ZERO processor time, and `base ensure` would have waited out the
// full half hour before saying anything was wrong. TODO/PROGRESS.md finding 84.
//
// ⭐ TWO DEADLINES, BECAUSE A HANG AND A SLOW TRANSFER ARE DIFFERENT THINGS.
// A total ceiling alone has to be set for the worst legitimate case, which
// makes it useless against a stall. A STALL deadline asks a different question:
// has anything happened recently. A pull moving bytes over a slow link is never
// killed, and a pull that has stopped is given up in minutes.
//
// ⛔ AND A STALL IS REPORTED AS A STALL. A deadline that fires and reports
// "context deadline exceeded" sends a reader to look for a slow network. This
// says which of the two limits fired, how long it waited, and what the last
// byte was.
//
// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// EngineCallTimeout is the default ceiling on ANY host-engine invocation.
//
// ⚠ IT IS THE FALLBACK, NOT THE RULE. A call that knows it is long, such as a
// pull, names its own; a call that names nothing gets this, so a new call site
// cannot be unbounded by forgetting. Two minutes is well past what every engine
// question in this tree has ever taken: the slowest, `podman info` on a cold
// machine, is bounded at 90 seconds by measurement.
const EngineCallTimeout = 2 * time.Minute

// EngineTransferTimeout is the ceiling on a pull or an export, which move
// bytes and legitimately take a long time over a slow link.
const EngineTransferTimeout = 30 * time.Minute

// EngineStallTimeout is how long a transfer may produce NOTHING before it is
// given up.
//
// ⭐ THIS IS THE ONE THAT CATCHES A HANG. The measured stall produced no bytes
// at all for 28 minutes; the same pull, working, printed a progress line every
// few seconds. ⚠ Four minutes rather than one: podman is silent while it
// resolves a manifest and while it writes a large blob to disk, and a limit
// tighter than the quietest legitimate stretch would kill working pulls.
const EngineStallTimeout = 4 * time.Minute

// engineCall is one bounded invocation of the host engine.
type engineCall struct {
	// Timeout is the total ceiling. Zero means EngineCallTimeout.
	Timeout time.Duration
	// Stall is how long the call may produce no output at all. Zero means no
	// stall detection, which is right for a call that answers in one burst.
	Stall time.Duration
	// What names the operation in a refusal, in the engine's own words, so a
	// reader is not told "process" about a pull.
	What string
}

// run executes the engine with both deadlines applied.
//
// ⛔ EVERY HOST-ENGINE CALL IN THIS PACKAGE GOES THROUGH HERE. That is the
// whole point: the deadline is a property of talking to the engine, not
// something each caller remembers.
func (c engineCall) run(ctx context.Context, exe string, args ...string) (string, string, error) {
	out := &boundedBuffer{max: 2 << 20}
	stderr := &boundedBuffer{max: 64 << 10}
	err := c.runStream(ctx, out, stderr, exe, args...)
	if err == nil && (out.Truncated() || stderr.Truncated()) {
		err = errors.New("process output exceeded the capture limit")
	}
	return out.String(), stderr.String(), err
}

// runStream is the same two deadlines with the caller's own writers, for a
// transfer whose output is a FILE rather than a diagnostic.
//
// ⛔ run IS A WRAPPER OVER THIS ONE, so a `pull` into a buffer and an `export`
// into a tarball are bounded by the same code. Two implementations would be two
// places for a deadline to go missing, which is the whole defect this file is
// about.
func (c engineCall) runStream(ctx context.Context, out, stderr io.Writer, exe string, args ...string) error {
	total := c.Timeout
	if total <= 0 {
		total = EngineCallTimeout
	}
	bounded, cancel := context.WithTimeout(ctx, total)
	defer cancel()

	watch := &stallWatch{}
	stopStall := func() {}
	if c.Stall > 0 {
		// ⛔ THE CANCEL IS THE STALL'S OWN, so the caller can tell which limit
		// fired. Cancelling `bounded` directly would make a stall
		// indistinguishable from the total ceiling in the error.
		stalled, cancelStall := context.WithCancel(bounded)
		bounded = stalled
		stopStall = watch.start(bounded, c.Stall, cancelStall)
	}

	err := runCommand(bounded, newCommand(bounded, exe, args...), nil, watch.wrap(out), watch.wrap(stderr))
	stopStall()
	if err != nil {
		if e := c.explain(watch, total, ctx.Err()); e != nil {
			return e
		}
	}
	return err
}

// explain turns a deadline into the sentence a reader needs, and returns nil
// where the failure was not a deadline at all.
//
// ⛔ IT IS A FUNCTION WITH NO PROCESS IN IT, so a case can reach every branch
// without starting an engine. Finding 42 is why.
func (c engineCall) explain(watch *stallWatch, total time.Duration, callerErr error) error {
	what := c.What
	if what == "" {
		what = "the engine"
	}
	switch {
	case callerErr != nil:
		// The caller's own context ended. That is not this call's deadline and
		// saying it was would blame the engine for a cancellation.
		return nil
	case watch.Fired():
		return fmt.Errorf("%s produced nothing for %s and was given up; it had run %s in total. "+
			"⚠ This is a STALL and not a slow link: a transfer that is moving is never stopped here. "+
			"Check the registry and the engine's own connection with: podman system connection list",
			what, FormatSpan(c.Stall), FormatSpan(watch.Elapsed()))
	case watch.Elapsed() >= total:
		return fmt.Errorf("%s did not finish within %s, which is this tool's ceiling for it",
			what, FormatSpan(total))
	}
	return nil
}

// stallWatch records when the last byte arrived, on either stream.
//
// ⚠ IT HOLDS A TIME AND NEVER A COPY. tick.go's counting writer is the same
// idea for the same reason: the buffers already hold the bytes, and a time is
// the only thing safe to read from the watching goroutine.
type stallWatch struct {
	mu      sync.Mutex
	started time.Time
	last    time.Time
	fired   bool
	now     func() time.Time
}

func (s *stallWatch) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// wrap returns a writer that records the arrival of every byte.
func (s *stallWatch) wrap(to io.Writer) io.Writer { return &stallWriter{watch: s, to: to} }

// touch records that something arrived.
func (s *stallWatch) touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = s.clock()
}

// Fired says the stall deadline was what ended the call.
func (s *stallWatch) Fired() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fired
}

// Elapsed is how long the call ran.
func (s *stallWatch) Elapsed() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started.IsZero() {
		return 0
	}
	return s.clock().Sub(s.started)
}

// Quiet is how long it has been since the last byte.
func (s *stallWatch) Quiet() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last.IsZero() {
		return 0
	}
	return s.clock().Sub(s.last)
}

// start begins watching and returns a function that stops it and waits.
//
// ⛔ THE STOP WAITS, for the reason tick.go gives about its own: a goroutine
// still deciding whether to cancel, after the call returned, could cancel the
// NEXT thing that borrowed the context.
func (s *stallWatch) start(ctx context.Context, limit time.Duration, cancel context.CancelFunc) func() {
	s.mu.Lock()
	s.started, s.last = s.clock(), s.clock()
	s.mu.Unlock()
	stop, done := make(chan struct{}), make(chan struct{})
	// ⚠ The tick is a fraction of the limit rather than the limit itself, or a
	// stall beginning just after a tick would wait almost twice as long as it
	// says. Four checks inside the window bounds the overshoot at a quarter.
	every := limit / 4
	if every < time.Second {
		every = time.Second
	}
	go func() {
		defer close(done)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				if s.Quiet() < limit {
					continue
				}
				s.mu.Lock()
				s.fired = true
				s.mu.Unlock()
				cancel()
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(stop) })
		<-done
	}
}

type stallWriter struct {
	watch *stallWatch
	to    io.Writer
}

func (w *stallWriter) Write(p []byte) (int, error) {
	// ⚠ RECORDED EVEN FOR A ZERO-LENGTH WRITE, because a child that flushes an
	// empty buffer is still alive, and treating it as silence would kill a
	// process that is talking.
	w.watch.touch()
	if w.to == nil {
		return len(p), nil
	}
	return w.to.Write(p)
}
