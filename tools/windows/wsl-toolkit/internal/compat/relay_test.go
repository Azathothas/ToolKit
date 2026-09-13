// SPDX-License-Identifier: 0BSD

package compat

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func bgContext() context.Context { return context.Background() }

// pipePair is an in-memory reader that behaves like a child's pipe for the
// relay loop: reads block until Write, and Close ends them.
type pipePair struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func newPipe() *pipePair {
	r, w := io.Pipe()
	return &pipePair{r, w}
}

// relayHarness holds everything one relayLoop run needs without a WSL.
//
// ⭐ THE CLOCK IS THE TEST'S. The loop asks st.started() for its whole picture
// of time, so a fake clock lets the deadline, the tick and the flush bound be
// exercised in milliseconds of real time without any real silence.
type relayHarness struct {
	out   bytes.Buffer
	notes bytes.Buffer
	sess  *session
	state *streamState
	proc  *relayProc
	hit   bool
}

// newRelayHarness builds the session from a real parameter parse, so the
// harness exercises the same settings resolution a live run took.
func newRelayHarness(t *testing.T, opts ...string) *relayHarness {
	t.Helper()
	all := append([]string{"-Action", "Run", "-Name", "eph-x-1a2b", "-Command", "true"}, opts...)
	o, err := Parse(all)
	if err != nil {
		t.Fatal(err)
	}
	h := &relayHarness{}
	s := &session{
		opts: o, baseDir: t.TempDir(),
		log: &console{report: &h.out, note: &h.notes},
		out: &h.out, errw: &h.notes,
		stop: context.Background(),
	}
	s.relayOff = o.NoTimestamps || o.TimestampProfile == "raw"
	if !s.relayOff {
		settings, err := resolveStreamLogSettings(o, false)
		if err != nil {
			t.Fatal(err)
		}
		s.settings = settings
	}
	h.sess = s
	h.state = newStreamState("eph-x-1a2b", s.settings, func() dur { return 0 }, &h.out, &h.notes)
	return h
}

// speedUp replaces the state's clock with one that runs n times faster than
// the real one.
func (h *relayHarness) speedUp(n time.Duration) {
	start := time.Now()
	h.state.started = func() dur { return dur(time.Since(start)) * n }
}

// runRelay starts the loop over two in-memory pipes, writes to them the way a
// child would, and waits for the loop to end. ⛔ THE LOOP IS STARTED BEFORE
// ANYTHING IS WRITTEN: a pipe write with no reader is a blocked test, which is
// exactly the deadlock the read-before-wait rule exists for.
func (h *relayHarness) runRelay(t *testing.T, stdout, stderr []byte) {
	t.Helper()
	outPipe, errPipe := newPipe(), newPipe()
	h.proc = &relayProc{
		stdout: outPipe.r, stderr: errPipe.r,
		wait: func() int { return 0 }, kill: func() {},
	}
	done := make(chan error, 1)
	go func() { done <- relayLoop(h.state, h.sess, h.proc, &h.hit) }()
	_, _ = outPipe.w.Write(stdout)
	_, _ = errPipe.w.Write(stderr)
	_ = outPipe.w.Close()
	_ = errPipe.w.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the loop failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the relay loop did not end when both streams did")
	}
}

func TestTheRelayTagsTheGuestsStreamsAndKeepsThemApart(t *testing.T) {
	h := newRelayHarness(t)
	h.runRelay(t, []byte("hello from stdout\n"), []byte("bang\n"))
	// A guest stdout line goes to the report stream tagged 'out'; a guest
	// stderr line goes to stderr tagged 'err', because merging them would
	// destroy the fact the tag reports.
	if !strings.Contains(h.out.String(), "out ") || !strings.Contains(h.out.String(), "hello from stdout") {
		t.Errorf("the guest's stdout did not reach the report stream tagged out: %q", h.out.String())
	}
	if !strings.Contains(h.notes.String(), "err ") || !strings.Contains(h.notes.String(), "bang") {
		t.Errorf("the guest's stderr did not reach stderr tagged err: %q", h.notes.String())
	}
}

func TestTheRelayRedactsAndBoundsBeforeTheSink(t *testing.T) {
	h := newRelayHarness(t, "-Redact", "sk-[a-z0-9]+", "-MaxLineBytes", "30")
	long := strings.Repeat("word ", 20)
	h.runRelay(t, []byte("token sk-abc123\n"+long+"\n"), nil)
	if strings.Contains(h.out.String(), "sk-abc123") {
		t.Errorf("a secret reached the sink: %q", h.out.String())
	}
	if !strings.Contains(h.out.String(), "***") {
		t.Errorf("the marker is missing: %q", h.out.String())
	}
	if !strings.Contains(h.out.String(), "bytes cut)") {
		t.Errorf("the bound did not say what it dropped: %q", h.out.String())
	}
}

func TestTheRelayConsumesAProgressLine(t *testing.T) {
	h := newRelayHarness(t, "-ProgressPrefix", "EPH:")
	h.runRelay(t, []byte("EPH: 42 unpacking\n"), nil)
	// ⛔ CONSUMED, NOT RELAYED: a line the parse understood never reaches the
	// sink. A tool that swallowed every line beginning with some chosen string
	// would be a tool that eats somebody's output, which is why the token has
	// no default; having chosen one, the caller gets the consumption.
	if strings.Contains(h.out.String(), "unpacking") {
		t.Errorf("a progress line was relayed: %q", h.out.String())
	}
	if h.state.progress == nil || h.state.progress.Percent != 42 || h.state.progress.Label != "unpacking" {
		t.Errorf("the progress was not recorded: %+v", h.state.progress)
	}
}

// TestTheRelayTerminatesTheDistroAtTheDeadline is the mutation case for the
// deadline: a loop that waits on the child forever never reaches it.
func TestTheRelayTerminatesTheDistroAtTheDeadline(t *testing.T) {
	h := newRelayHarness(t, "-CommandTimeoutSeconds", "1")
	h.speedUp(10) // one fake second is a tenth of a real one
	previous := resolveWsl
	resolveWsl = func() (string, error) { return "/nonexistent/wsl-for-test", nil }
	t.Cleanup(func() { resolveWsl = previous })

	// The child never prints and never exits, which is exactly the wedged-init
	// shape the deadline exists for.
	outPipe, errPipe := newPipe(), newPipe()
	killed := make(chan struct{})
	h.proc = &relayProc{
		stdout: outPipe.r, stderr: errPipe.r,
		wait: func() int { return 0 },
		kill: func() {
			_ = outPipe.w.Close()
			_ = errPipe.w.Close()
			close(killed)
		},
	}
	done := make(chan error, 1)
	go func() { done <- relayLoop(h.state, h.sess, h.proc, &h.hit) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the deadline path failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the loop did not reach the deadline; the caller would wait forever")
	}
	if !h.hit {
		t.Error("the deadline fired without reporting itself")
	}
	select {
	case <-killed:
	default:
		t.Error("the child was not killed; the caller would wait forever beside it")
	}
	if !strings.Contains(h.notes.String(), "124") {
		t.Errorf("the deadline line does not say 124: %q", h.notes.String())
	}
}

func TestTheRelayShowsAnUnterminatedLineAfterTheFlushBound(t *testing.T) {
	h := newRelayHarness(t)
	h.speedUp(10)
	outPipe, errPipe := newPipe(), newPipe()
	h.proc = &relayProc{
		stdout: outPipe.r, stderr: errPipe.r,
		wait: func() int { return 0 }, kill: func() {},
	}
	done := make(chan error, 1)
	go func() { done <- relayLoop(h.state, h.sess, h.proc, &h.hit) }()
	_, _ = outPipe.w.Write([]byte("downloading 3%"))
	// No newline and no carriage return follows, and the pipe STAYS OPEN: the
	// line sits unterminated until the flush bound shows it early, marked as
	// partial. A prompt waiting on stdin that will never arrive is exactly
	// this shape.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(h.out.String(), "downloading 3%") {
		time.Sleep(5 * time.Millisecond)
	}
	_ = outPipe.w.Close()
	_ = errPipe.w.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.out.String(), "downloading 3%") {
		t.Fatalf("the unterminated line was never shown: %q", h.out.String())
	}
	// Shown early means marked: without the '~' a partial line reads as a
	// complete one the guest never wrote.
	if !strings.Contains(h.out.String(), "~") {
		t.Errorf("the shown line is not marked as unterminated: %q", h.out.String())
	}
}
