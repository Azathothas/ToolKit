// SPDX-License-Identifier: 0BSD

package checks

import (
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
)

// ⭐ THE GATE RUNS ITS CHECKS AT ONCE, AND THIS FILE IS WHAT BOUNDS "AT ONCE".
// One after another, the checks spent most of their time waiting on one child
// process at a time. That was one `sh -n` per fenced block, one shellcheck per
// script and one Go module after another. Started together with no bound, the
// same work starts hundreds of children on a runner with four CPUs.
//
// ⛔ EVERY CHILD PROCESS THIS PACKAGE STARTS HOLDS A SLOT WHILE IT RUNS, and
// there is one slot per CPU. A call site that runs a child without combined or
// output escapes the bound, and TestEveryChildTakesASlot refuses one.
//
// ⚠ THE BOUND IS ON CHILDREN, NOT ON CPU TIME. A `go test` or a pwsh session
// holds one slot and may use more than one CPU. The Go work in this process is
// bounded by GOMAXPROCS, which is the same number.

// slots holds one token per child process allowed to run at one time.
var slots = make(chan struct{}, runtime.NumCPU())

// child runs a child process's own call with a slot held for as long as the
// call takes. A call never asks for a second slot while it holds one, so no
// two callers can each hold what the other waits for.
func child(cmd *exec.Cmd, call func(*exec.Cmd) ([]byte, error)) ([]byte, error) {
	slots <- struct{}{}
	defer func() { <-slots }()
	return call(cmd)
}

// combined is cmd.CombinedOutput with a slot held.
func combined(cmd *exec.Cmd) ([]byte, error) { return child(cmd, (*exec.Cmd).CombinedOutput) }

// output is cmd.Output with a slot held.
func output(cmd *exec.Cmd) ([]byte, error) { return child(cmd, (*exec.Cmd).Output) }

// each calls fn(0) to fn(n-1) at once and returns when every call has.
//
// ⛔ A CALLER STORES WHAT CALL i FOUND AT INDEX i, and reads the slice in order
// once each returns. That keeps a check's findings in the order its loop gave
// them. Concurrency changes when a finding is made, and never which findings
// there are or how they read.
//
// ⚠ NO MORE WORKERS THAN SLOTS. A worker beyond the slot count could only queue
// for a slot. A check with 345 calls queued ahead of every other check would
// make all of them wait for its 345.
func each(n int, fn func(i int)) {
	workers := min(n, cap(slots))
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				fn(i)
			}
		}()
	}
	wg.Wait()
}
