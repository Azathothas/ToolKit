// SPDX-License-Identifier: 0BSD

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-184: a session the base restarted under ------------------------------

// TestASessionTheBaseOutlivedIsSaidAsThat: `wait`, `stop` and `inspect` name a
// restart or a reused id, and say what they said before for a supervisor that
// went on its own.
func TestASessionTheBaseOutlivedIsSaidAsThat(t *testing.T) {
	restarted := toolkit.SessionState{Lost: toolkit.SessionLostRestart}
	if got := goneNote(restarted, "otherwise"); !strings.Contains(got, "started again") {
		t.Errorf("a session from another boot reads %q", got)
	}
	reused := toolkit.SessionState{Lost: toolkit.SessionLostReused}
	if got := goneNote(reused, "otherwise"); !strings.Contains(got, "other processes") {
		t.Errorf("a session whose supervisor id was reused reads %q", got)
	}
	if got := goneNote(toolkit.SessionState{}, "otherwise"); got != "otherwise" {
		t.Errorf("a supervisor that went on its own reads %q", got)
	}

	var b strings.Builder
	p := func(format string, a ...any) error { _, err := fmt.Fprintf(&b, format, a...); return err }
	if err := renderSession(p, toolkit.SessionRecord{Distro: "wsl-toolkit", User: "toolkit"}, &restarted); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "now         gone: the base started again") || strings.Contains(b.String(), "ended. Exit") {
		t.Errorf("inspect of a session from another boot reads:\n%s", b.String())
	}
}

// TestAStopThatFoundItsSessionLostSaysItStoppedNothing: a session read as
// running, which the stop itself found lost, is reported as ended, with why and
// that nothing was stopped, and exits 0. A stop that signalled says so. WSL-184.
func TestAStopThatFoundItsSessionLostSaysItStoppedNothing(t *testing.T) {
	var lost toolkit.StopReport
	stoppedSession(&lost, toolkit.SessionState{}, toolkit.SessionLostReused)
	if lost.Before != "ended" || lost.Stopped || lost.After != "" || lost.Exit == nil || *lost.Exit != exitFailed ||
		!strings.Contains(lost.Note, "other processes") || !strings.HasSuffix(lost.Note, ", so nothing was stopped") {
		t.Errorf("a stop that found its session lost reports %+v", lost)
	}
	if code, err := renderStop(lost, true); err != nil || code != exitOK {
		t.Errorf("a stop that found its session lost exited %d, %v; it exits 0", code, err)
	}
	var stopped toolkit.StopReport
	code := 130
	stoppedSession(&stopped, toolkit.SessionState{Exit: &code, Stopped: true}, "")
	if stopped.Before != "running" || !stopped.Stopped || stopped.After != "ended" || stopped.Exit == nil || *stopped.Exit != 130 {
		t.Errorf("a stop that signalled reports %+v", stopped)
	}
}
