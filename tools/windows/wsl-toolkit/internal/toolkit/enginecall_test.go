// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAStalledEngineCallIsGivenUpAndSaysSo is finding 84's guard.
//
// ⛔ THE MEASURED DEFECT: a `podman pull` stopped dead with ZERO bytes read,
// ZERO written and ZERO processor time, and nothing gave up on it for 28
// minutes. The total ceiling was 30 minutes and correct; what was missing was
// any question about whether the transfer was still moving.
//
// ⭐ THE CLOCK IS INJECTED, so the case drives a four-minute stall in
// microseconds. A case that waited out a real deadline would be a case nobody
// runs.
func TestAStalledEngineCallIsGivenUpAndSaysSo(t *testing.T) {
	c := engineCall{Timeout: 30 * time.Minute, Stall: 4 * time.Minute, What: "podman pull REF"}
	w := &stallWatch{}
	base := time.Now()
	// Started half an hour ago, last byte 28 minutes ago: the shape measured.
	w.started, w.last, w.fired = base.Add(-28*time.Minute), base.Add(-28*time.Minute), true
	w.now = func() time.Time { return base }

	err := c.explain(w, 30*time.Minute, nil)
	if err == nil {
		t.Fatal("a stall was not explained at all")
	}
	for _, want := range []string{"podman pull REF", "produced nothing", "STALL", "4m"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal does not say %q: %v", want, err)
		}
	}
	// ⛔ A STALL MUST NOT READ AS A SLOW LINK. The whole reason it is a separate
	// deadline is that the two send a reader to different places.
	if strings.Contains(err.Error(), "did not finish within") {
		t.Fatalf("a stall was reported as the total ceiling: %v", err)
	}
}

// TestTheTotalCeilingAndTheStallAreToldApart. Two deadlines that report the
// same sentence are one deadline with extra code.
func TestTheTotalCeilingAndTheStallAreToldApart(t *testing.T) {
	base := time.Now()
	c := engineCall{Timeout: time.Minute, Stall: 10 * time.Second, What: "podman info"}

	// Ran past the total, and was never quiet: the ceiling fired.
	slow := &stallWatch{started: base.Add(-2 * time.Minute), last: base}
	slow.now = func() time.Time { return base }
	err := c.explain(slow, time.Minute, nil)
	if err == nil || !strings.Contains(err.Error(), "did not finish within") {
		t.Fatalf("a run past the ceiling reads %v", err)
	}
	if strings.Contains(err.Error(), "STALL") {
		t.Fatalf("the ceiling was reported as a stall: %v", err)
	}

	// ⛔ THE CALLER'S OWN CANCELLATION IS NEITHER, and blaming the engine for it
	// would send a reader after a machine that is working.
	if err := c.explain(slow, time.Minute, context.Canceled); err != nil {
		t.Fatalf("a caller cancellation was reported as a deadline: %v", err)
	}
	// A failure that is not a deadline at all is left to the caller's own error.
	quick := &stallWatch{started: base.Add(-time.Second), last: base}
	quick.now = func() time.Time { return base }
	if err := c.explain(quick, time.Minute, nil); err != nil {
		t.Fatalf("an ordinary failure was dressed as a deadline: %v", err)
	}
}

// TestEveryEngineCallCarriesADeadline is the guard that keeps the fix from
// rotting.
//
// ⛔ A DEADLINE APPLIED AT SIX CALL SITES IS ONE THAT WILL BE APPLIED AT FIVE,
// which is RULES.md section 3's own rule and is exactly what happened here. So
// this reads the SOURCE and refuses a host-engine invocation that did not go
// through engineCall. ⚠ It is a textual rule, and it is worth it: the
// alternative is remembering, and remembering is what failed.
func TestEveryEngineCallCarriesADeadline(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("engine.go"))
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		// `e.Path` and `exe` are the host engine. Reaching either through the
		// raw process helpers is the unbounded shape.
		if !strings.Contains(trimmed, "e.Path") && !strings.Contains(trimmed, ", exe,") {
			continue
		}
		for _, raw := range []string{"Output(", "runCommand(", "newCommand(", "outputRaw("} {
			if strings.Contains(trimmed, raw) {
				t.Errorf("engine.go:%d reaches the host engine through %s rather than engineCall, "+
					"so it carries whatever deadline its caller happened to hold: %s", i+1, raw, trimmed)
			}
		}
	}
}

// TestAZeroTimeoutGetsTheDefaultRatherThanNone. A call that names nothing must
// not be unbounded, which is the whole point of a default.
func TestAZeroTimeoutGetsTheDefaultRatherThanNone(t *testing.T) {
	if EngineCallTimeout <= 0 {
		t.Fatal("the default ceiling is not positive, so a call naming none would be unbounded")
	}
	if EngineStallTimeout <= 0 || EngineStallTimeout >= EngineTransferTimeout {
		t.Fatalf("the stall deadline is %s and the transfer ceiling is %s; a stall that is not "+
			"shorter can never fire", EngineStallTimeout, EngineTransferTimeout)
	}
	base := time.Now()
	w := &stallWatch{started: base.Add(-3 * EngineCallTimeout), last: base}
	w.now = func() time.Time { return base }
	// explain is given the resolved total, so this asserts the resolution the
	// caller does: zero means the default.
	c := engineCall{What: "podman anything"}
	total := c.Timeout
	if total <= 0 {
		total = EngineCallTimeout
	}
	if err := c.explain(w, total, nil); err == nil || !strings.Contains(err.Error(), "ceiling") {
		t.Fatalf("a call with no stated timeout was not held to the default: %v", err)
	}
}

// TestTheStallWatchRecordsEveryWriteIncludingAnEmptyOne.
//
// ⚠ A CHILD THAT FLUSHES AN EMPTY BUFFER IS ALIVE. Treating a zero-length write
// as silence would give up on a process that is talking, which is a worse
// failure than the one this file fixes.
func TestTheStallWatchRecordsEveryWriteIncludingAnEmptyOne(t *testing.T) {
	w := &stallWatch{}
	base := time.Now()
	w.now = func() time.Time { return base }
	w.started, w.last = base.Add(-time.Hour), base.Add(-time.Hour)
	if got := w.Quiet(); got < time.Hour {
		t.Fatalf("quiet is %s before any write", got)
	}
	if _, err := w.wrap(nil).Write(nil); err != nil {
		t.Fatal(err)
	}
	if got := w.Quiet(); got != 0 {
		t.Fatalf("an empty write did not count as life: quiet is %s", got)
	}
	// And it forwards what it is given, or it would be swallowing output.
	var sink strings.Builder
	n, err := w.wrap(&sink).Write([]byte("bytes"))
	if err != nil || n != 5 || sink.String() != "bytes" {
		t.Fatalf("the watcher did not forward: n=%d err=%v got=%q", n, err, sink.String())
	}
}

// TestAStallWatchStopsCleanlyAndOnlyOnce. The stop waits, for the reason
// tick.go gives: a goroutine still deciding whether to cancel, after the call
// returned, could cancel the next thing that borrowed the context.
func TestAStallWatchStopsCleanlyAndOnlyOnce(t *testing.T) {
	w := &stallWatch{}
	cancelled := false
	stop := w.start(context.Background(), time.Hour, func() { cancelled = true })
	stop()
	stop()
	if cancelled {
		t.Fatal("a watch that was stopped at once cancelled the call")
	}
	if w.Fired() {
		t.Fatal("a watch that never saw a stall reported one")
	}
	// A context already done ends it without firing, which is the cancelled-run
	// case: the call is over and the watcher must not act on it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w2 := &stallWatch{}
	w2.start(ctx, time.Millisecond, func() { t.Error("a cancelled run was reported as a stall") })()
	if w2.Fired() {
		t.Fatal("a cancelled run reported a stall")
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("the fixture's own context did not cancel")
	}
}

// TestDriveAStallAgainstARealProcess runs a real child that produces one line
// and then goes silent for ever, and asserts the stall deadline ends it.
func TestDriveAStallAgainstARealProcess(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	started := time.Now()
	c := engineCall{Timeout: 5 * time.Minute, Stall: 2 * time.Second, What: "the fake engine"}
	out, _, err := c.run(context.Background(), sh, "-c", "echo moving; sleep 300")
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("a child that went silent for ever was not given up")
	}
	if !strings.Contains(err.Error(), "produced nothing for 2s") {
		t.Fatalf("the refusal does not name the stall: %v", err)
	}
	if elapsed > 30*time.Second {
		t.Fatalf("the stall took %s to fire, and the limit was 2s", elapsed)
	}
	if !strings.Contains(out, "moving") {
		t.Fatalf("the output before the stall was lost: %q", out)
	}
	t.Logf("DRIVEN: stall fired after %s, refusal: %v", elapsed.Round(time.Millisecond), err)
}

// TestDriveAChildThatKeepsTalkingIsNotKilled is the other half: a slow transfer
// that is still moving must never be stopped.
func TestDriveAChildThatKeepsTalkingIsNotKilled(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	c := engineCall{Timeout: time.Minute, Stall: 2 * time.Second, What: "the fake engine"}
	// Six seconds of work, a byte every second: three times the stall window in
	// total, and never quiet for it.
	out, _, err := c.run(context.Background(), sh, "-c", "i=0; while [ $i -lt 6 ]; do echo tick; sleep 1; i=$((i+1)); done")
	if err != nil {
		t.Fatalf("a child that kept talking was killed: %v", err)
	}
	if n := strings.Count(out, "tick"); n != 6 {
		t.Fatalf("got %d ticks, want 6: %q", n, out)
	}
}
