// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ⛔ WHY A TICK EXISTS. A job that is running tells its caller almost nothing.
// `resources` reports what exists when asked and there was no periodic signal,
// so an agent watching a long matrix could not tell work in progress from a
// hang. WSL-50. `WSL-18` is the same lesson one level down: never emit nothing,
// render silence with a time on it and say what is still alive.
//
// ⛔ IT IS NOT A PROGRESS BAR AND MUST NOT BECOME ONE. It is a machine-readable
// event with a timestamp; whatever renders it belongs to the caller.
//
// ⚠ AND IT IS NOT A POLL LOOP AGAINST THE ENGINE. Everything a tick carries is
// already in this process: the elapsed time, the deadline it was given, the
// container's name and the bytes each stream has seen. Asking podman once a
// second for twelve rows would cost more than the signal is worth.

// TickInterval is how often a running job reports that it is still there.
//
// **SETTLED 2026-09-10 at five seconds**, configurable, and never below one.
//
// ⭐ FIVE STANDS, AND IT IS NOW A MEASUREMENT RATHER THAN A GUESS. WSL-50 said
// the number was a starting point and required somebody to measure the added
// load of a twelve-row matrix with ticks against one without. Measured on the
// development host on 2026-09-10, twelve rows sleeping 30s each:
//
//	with 5s ticks     95.9s wall, 72 tick events, 112 lines of progress
//	without           97.2s wall,  0 tick events,  40 lines
//
// ⚠ THE ESTIMATE IN THE ENTRY WAS HIGH BY ABOUT THREE TIMES. It predicted two
// and a half events per second, assuming twelve rows running concurrently for
// the whole duration; the fleet staggers them, so the real rate is 72 events
// over 96 seconds, or 0.75 per second. The wall-time difference is inside the
// noise and in the wrong direction to be a cost.
const TickInterval = 5 * time.Second

// MinTickInterval is the floor. ⛔ A caller asking for 100ms on a twelve-row
// fleet would produce 120 events a second, which is a progress bar by volume
// even if every event is machine-readable.
const MinTickInterval = time.Second

// TickEvent is one "still running" signal for one job.
type TickEvent struct {
	// ID is the job this is about, so a fleet's ticks can be told apart.
	ID string `json:"id"`
	// Label is what a report calls the row.
	Label string `json:"label,omitempty"`
	// Container is the engine's name for it, so a caller can go and look.
	Container string `json:"container,omitempty"`
	// ElapsedMS is how long this job has been running.
	ElapsedMS int64 `json:"elapsed_ms"`
	// RemainingMS is what is left of its deadline. ⚠ ABSENT rather than zero
	// where there is no deadline: zero means "out of time" and no deadline is a
	// different fact.
	RemainingMS *int64 `json:"remaining_ms,omitempty"`
	// StdoutBytes and StderrBytes are what each stream has carried so far.
	//
	// ⭐ THEY ARE THE HEARTBEAT'S REAL CONTENT. A tick with rising byte counts
	// is work; a tick with the same counts for a minute is a stall, and the two
	// are indistinguishable without them.
	StdoutBytes int64     `json:"stdout_bytes"`
	StderrBytes int64     `json:"stderr_bytes"`
	At          time.Time `json:"at"`
}

// ticker emits a TickEvent for one running job until it is stopped.
//
// ⛔ ONE GOROUTINE PER JOB AND IT IS ALWAYS STOPPED. A fleet starts twelve and
// stops twelve; a ticker that outlived its job would report a container that had
// gone as still running, which is worse than no tick at all.
type ticker struct {
	stop    chan struct{}
	stopped sync.Once
	done    chan struct{}
	stdout  *atomic.Int64
	stderr  *atomic.Int64
}

// startTicker begins emitting. A nil emit function means no tick at all, which
// is what a caller that did not ask for one passes.
func startTicker(ctx context.Context, interval time.Duration, ev TickEvent, deadline time.Time,
	stdout, stderr *atomic.Int64, emit func(TickEvent)) *ticker {
	if emit == nil || interval <= 0 {
		return nil
	}
	if interval < MinTickInterval {
		interval = MinTickInterval
	}
	t := &ticker{stop: make(chan struct{}), done: make(chan struct{}), stdout: stdout, stderr: stderr}
	started := time.Now()
	go func() {
		defer close(t.done)
		tk := time.NewTicker(interval)
		defer tk.Stop()
		for {
			select {
			case <-t.stop:
				return
			case <-ctx.Done():
				return
			case now := <-tk.C:
				out := ev
				out.At = now.UTC()
				out.ElapsedMS = now.Sub(started).Milliseconds()
				if !deadline.IsZero() {
					left := deadline.Sub(now).Milliseconds()
					out.RemainingMS = &left
				}
				if t.stdout != nil {
					out.StdoutBytes = t.stdout.Load()
				}
				if t.stderr != nil {
					out.StderrBytes = t.stderr.Load()
				}
				emit(out)
			}
		}
	}()
	return t
}

// Stop ends the ticks and waits for the goroutine, so no tick can arrive after
// the result that says the job is over.
//
// ⛔ THE WAIT IS THE POINT. Without it a tick already in flight would be
// serialised onto the stream AFTER the result event, and a caller reading
// events in order would see a job report that it finished and then that it is
// still running.
func (t *ticker) Stop() {
	if t == nil {
		return
	}
	t.stopped.Do(func() { close(t.stop) })
	<-t.done
}

// countingWriter counts what passes through it without holding any of it.
//
// ⚠ IT IS NOT A SECOND COPY OF THE STREAM. The bounded buffer and the
// transcript already hold bytes; this holds a number, which is what a tick
// needs and is the only thing safe to read from another goroutine while the
// stream is being written.
type countingWriter struct {
	to    interface{ Write([]byte) (int, error) }
	count *atomic.Int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.count.Add(int64(len(p)))
	if c.to == nil {
		return len(p), nil
	}
	return c.to.Write(p)
}
