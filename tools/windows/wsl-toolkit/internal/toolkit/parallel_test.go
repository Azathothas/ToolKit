// SPDX-License-Identifier: 0BSD

package toolkit

import "testing"

// parallelCase lets a case run beside the other parallel cases of this package.
//
// ⭐ THE PACKAGE'S CASES TOOK 150 s ONE AFTER ANOTHER, and most of a case's time
// is a shell or a child it waits on. TOOL-40.
//
// ⛔ ONLY A CASE THAT SETS NOTHING ANOTHER CASE READS. No t.Setenv, no working
// directory, no variable of this package or of another.
// TestNoParallelCaseSetsWhatAnotherReads in internal/toolkitsource reads that
// from the source.
//
// ⚠ AND NO BOUND A LOADED HOST CAN BREAK. A child that must not stall for 2 s, or
// a watch that must hold within 300 ms, stays serial. Beside 30 other cases a
// fork in Git for Windows' sh can take that long.
func parallelCase(t *testing.T) {
	t.Helper()
	t.Parallel()
}
