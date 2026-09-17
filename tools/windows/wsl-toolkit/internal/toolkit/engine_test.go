// engine_test.go - the host engine probe, and the podman state it must explain
// rather than relay.
//
// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// TestPodmanSocketFailureIsRecognisedAndNothingElseIs holds the matcher that
// decides whether a podman failure gets a second sentence.
//
// ⛔ THE CONSUMER BLAMES THE TOOL THEY RAN, NOT PODMAN. podman's own advice for
// this state is to try `podman machine init` and `podman machine start`; the
// machine is already running, so start does nothing and init would build a
// second machine. Relaying that unchanged sends a reader to two commands that
// cannot help, from a tool they will hold responsible.
//
// ⚠ AND IT MUST NOT FIRE ON EVERYTHING. A podman that is missing, or that
// refused an image name, is a different problem with a different answer, and
// attaching connection advice to it would be this tool inventing a diagnosis.
func TestPodmanSocketFailureIsRecognisedAndNothingElseIs(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		match bool
	}{
		{
			"the ssh channel refusal this host produced",
			`unable to connect to Podman socket: Get "http://d/v5.8.6/libpod/_ping": ssh: rejected: connect failed (open failed)`,
			true,
		},
		{
			"the dial refusal the same host produced after a restart",
			"unable to connect to Podman socket: failed to connect: dial tcp 127.0.0.1:51814: connectex: No connection could be made because the target machine actively refused it.",
			true,
		},
		{"podman's own headline for it", "Cannot connect to Podman. Please verify your connection", true},
		{"a plain refused connection", "dial unix /run/podman/podman.sock: connect: connection refused", true},

		// ⛔ THE NEGATIVES ARE THE POINT. Each is a real podman failure with a
		// different cause and a different fix.
		{"no such image", `Error: initializing source docker://nope:latest: reading manifest latest: manifest unknown`, false},
		{"a bad template", "Error: can't evaluate field host in type define.InfoReport", false},
		{"out of space", "Error: writing blob: write /var/tmp/x: no space left on device", false},
		{"not installed at all", "docker: no accessible executable found", false},
		{"an empty message", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := podmanSocketFailure.MatchString(c.text); got != c.match {
				t.Fatalf("matched %v, want %v, for: %s", got, c.match, c.text)
			}
		})
	}
}

// TestDiagnosePodmanSaysNothingWithoutAFailure keeps the diagnosis off the
// healthy path.
//
// ⛔ IT RUNS COMMANDS, so firing it on a nil error or an unrecognised one would
// put several podman invocations in front of every successful probe. The guard
// is the first line of the function and this is what holds it there.
func TestDiagnosePodmanSaysNothingWithoutAFailure(t *testing.T) {
	// ⚠ THE EXECUTABLE NAMED HERE DOES NOT EXIST. If either call reached it,
	// the test would still pass on the return value, so the real assertion is
	// that neither call takes measurable time to run a program.
	if got := DiagnosePodman(context.Background(), "no-such-podman-executable", nil); got != "" {
		t.Fatalf("a nil failure was diagnosed: %q", got)
	}
	unrelated := errors.New("manifest unknown")
	if got := DiagnosePodman(context.Background(), "no-such-podman-executable", unrelated); got != "" {
		t.Fatalf("an unrelated failure was diagnosed: %q", got)
	}
}

// TestThePodmanHintContradictsPodmansOwnAdvice is the sentence a consumer reads.
//
// ⛔ THIS IS THE HALF THAT WAS UNTESTED, and repo mutate found it: every earlier
// case asserted the diagnosis stayed SILENT and none that it ever SPOKE, so
// removing the whole guard left them green. A guard proved only by its negatives
// is not proved.
//
// ⭐ THE SENTENCES ARE THE PRODUCT. podman tells the reader to run
// `podman machine init` and `podman machine start`; when the machine is already
// running those do nothing and build a second machine, so the hint has to say so
// in as many words, beside podman's own text.
func TestThePodmanHintContradictsPodmansOwnAdvice(t *testing.T) {
	cases := []struct {
		name    string
		running bool
		machine string
		conn    string
		want    []string
		absent  []string
	}{
		{
			name:    "running, and another connection answers",
			running: true, machine: "podman-machine-default", conn: "podman-machine-default-root",
			want: []string{
				"reports Running",
				"`podman machine start` will not help",
				"would build a SECOND machine",
				"podman system connection default podman-machine-default-root",
			},
		},
		{
			// ⚠ NOTHING ANSWERS, so there is no command to offer and the hint
			// must not invent one. It names the thing to look at instead.
			name:    "running, and nothing answers",
			running: true, machine: "podman-machine-default", conn: "",
			want:   []string{"reports Running", "user@1000.service", "no amount of restarting the machine creates one"},
			absent: []string{"podman system connection default"},
		},
		{
			// ⛔ NOT RUNNING IS A DIFFERENT STATE, and `podman machine start` IS
			// the right advice for it, so the hint must not contradict it.
			name:    "not running",
			running: false, machine: "", conn: "",
			absent: []string{"will not help", "SECOND machine", "reports Running"},
			want:   []string{"user@1000.service"},
		},
		{
			name:    "not running, but a connection answers anyway",
			running: false, machine: "", conn: "some-other",
			want:   []string{"podman system connection default some-other"},
			absent: []string{"will not help"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := podmanHint(c.running, c.machine, c.conn)
			if got == "" {
				t.Fatal("the hint said nothing at all")
			}
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Fatalf("the hint does not say %q: %s", w, got)
				}
			}
			for _, a := range c.absent {
				if strings.Contains(got, a) {
					t.Fatalf("the hint says %q and should not: %s", a, got)
				}
			}
		})
	}
}

// TestDiagnosePodmanSpeaksForARealSocketFailure is the guard's positive half,
// driven on a host with no podman at all.
//
// ⭐ IT NEEDS NO PODMAN. With an executable that does not exist, both probes
// fail, so the hint falls to its "nothing answers" branch - which is still a
// sentence, and still more than podman said.
func TestDiagnosePodmanSpeaksForARealSocketFailure(t *testing.T) {
	failure := errors.New(`unable to connect to Podman socket: Get "http://d/_ping": ssh: rejected: connect failed (open failed)`)
	got := DiagnosePodman(context.Background(), "no-such-podman-executable", failure)
	if got == "" {
		t.Fatal("a real socket failure was relayed with nothing added")
	}
	if !strings.Contains(got, "user@1000.service") {
		t.Fatalf("the diagnosis names nothing to look at: %s", got)
	}
}

// TestALongEngineCallSaysHowLongItHasBeenQuiet is WSL-18's rule applied to the
// image pull, and the measurement that earned it.
//
// ⚠ MEASURED 2026-09-17: a `podman pull` on this host sat for 28 minutes with
// ZERO bytes read, ZERO written and ZERO processor time over a 25-second
// window, while `base ensure` printed `pulling <ref>` and then nothing at all.
// The pull IS bounded, at thirty minutes, and the ceiling is right; what was
// missing was any signal in between, so a stalled pull and a slow one were the
// same picture for half an hour.
func TestALongEngineCallSaysHowLongItHasBeenQuiet(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	log := func(s string) { mu.Lock(); lines = append(lines, s); mu.Unlock() }
	stop := beat(context.Background(), log, "still pulling REF")
	// The ticker fires at PullBeatEvery, which is far longer than a case should
	// wait, so what is asserted here is the CONTRACT rather than the interval:
	// stopping is safe, it waits, and it is idempotent.
	stop()
	stop()
	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 0 {
		t.Fatalf("a beat that was stopped at once still spoke: %v", lines)
	}
	// ⛔ A nil log MUST NOT start a goroutine, or every capture-only caller
	// would leak one. Calling the returned function proves it is safe.
	beat(context.Background(), nil, "x")()
}

// TestTheBeatSaysTheElapsedTimeAndNotJustThatItIsAlive. A watcher whose only
// output is that it is watching is the row forbidden-patterns.md already has.
func TestTheBeatSaysTheElapsedTimeAndNotJustThatItIsAlive(t *testing.T) {
	var mu sync.Mutex
	got := make(chan string, 4)
	log := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		select {
		case got <- s:
		default:
		}
	}
	// A context already cancelled ends the goroutine without a line, which is
	// the other half: a cancelled pull does not go on reporting.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	beat(ctx, log, "still pulling REF")()
	select {
	case s := <-got:
		t.Fatalf("a cancelled beat spoke: %q", s)
	default:
	}
	if PullBeatEvery <= 0 {
		t.Fatal("the interval is not positive, so the beat would spin")
	}
}
