// SPDX-License-Identifier: 0BSD

package main

import (
	"errors"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// TestACancelledBsdRunExitsOneThreeZero is WSL-151, finding 9: a `bsd run` ended
// with Ctrl+C answers 130, as `distro run` answers one, rather than the exit 2 a
// could-not-run earns. The cancellation is read from the result, because the run
// error and a cancellation carry the same words.
func TestACancelledBsdRunExitsOneThreeZero(t *testing.T) {
	cancelled := toolkit.BsdResult{Cancelled: true, Exit: 130, Error: "the run was cancelled, so the guest was stopped. The answer is 130"}
	if code, err := bsdRunExit(cancelled, errors.New(cancelled.Error)); code != 130 || err != nil {
		t.Fatalf("a cancelled run exited %d, %v; want 130 and no error", code, err)
	}

	// A guest that finished keeps its own exit, and a could-not-run is exit 2.
	if code, err := bsdRunExit(toolkit.BsdResult{Exit: 3}, nil); code != 3 || err != nil {
		t.Fatalf("a finished run exited %d, %v; want its own 3", code, err)
	}
	boom := errors.New("qemu could not start")
	if code, err := bsdRunExit(toolkit.BsdResult{}, boom); code != exitCannot || err != boom {
		t.Fatalf("a run that could not start exited %d, %v; want %d and the error", code, err, exitCannot)
	}
}
