// slots_test.go - the gate's children run at once, and never more of them than
// there are slots.
//
// ⛔ A CASE HERE DECIDES BY WHAT IT TESTS, NOT BY THE HOST'S TIMING. A case that
// needs two calls at once waits for the other one to start, and passes the
// moment it does. Only the broken code waits out the timeout, so a slow host
// cannot turn a serial run green.
//
// ⭐ THE BOUND IS ASKED, NOT WAITED FOR. A bound promises that something does
// NOT happen, and waiting for it to happen can only give up after a while.
// TestAChildWaitsForAFreeSlot asks the pool the way a second child would, and
// the answer comes at once on any host.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// withSlots gives the test a pool of n slots and puts the real one back after.
func withSlots(t *testing.T, n int) {
	t.Helper()
	saved := slots
	slots = make(chan struct{}, n)
	t.Cleanup(func() { slots = saved })
}

// TestEachRunsItsCallsAtOnce is the case for the whole speed-up: call 0 waits
// for call 1 to start. Run one after another, call 1 cannot start until call 0
// has given up, and call 0 says so.
func TestEachRunsItsCallsAtOnce(t *testing.T) {
	withSlots(t, 2)
	second := make(chan struct{})
	var alone atomic.Bool
	each(2, func(i int) {
		if i == 1 {
			close(second)
			return
		}
		select {
		case <-second:
		case <-time.After(10 * time.Second):
			alone.Store(true)
		}
	})
	if alone.Load() {
		t.Fatal("call 0 waited ten seconds for call 1 to start; each ran its calls one after another")
	}
}

// TestEachMakesEveryCallOnce holds the other half of each: every index is
// called, and none twice, however many workers share them.
func TestEachMakesEveryCallOnce(t *testing.T) {
	withSlots(t, 3)
	var mu sync.Mutex
	got := map[int]int{}
	each(10, func(i int) {
		mu.Lock()
		got[i]++
		mu.Unlock()
	})
	for i := 0; i < 10; i++ {
		if got[i] != 1 {
			t.Fatalf("call %d was made %d times; each makes every call exactly once: %v", i, got[i], got)
		}
	}
}

// TestAChildWaitsForAFreeSlot is the bound. With one slot, a running child holds
// it, so a second child would wait. The case asks for a slot as child does,
// but without waiting, and the pool must say it is full. Once the child
// returns, the same ask must find the slot free again.
//
// ⭐ IT ANSWERS AT ONCE, BOTH WAYS. It used to give a second child two seconds to
// start, which every run paid. A broken bound then passed if the host could not
// start a goroutine in that time.
func TestAChildWaitsForAFreeSlot(t *testing.T) {
	withSlots(t, 1)
	free := func() bool {
		select {
		case slots <- struct{}{}:
			<-slots
			return true
		default:
			return false
		}
	}
	var whileHeld bool
	_, _ = child(nil, func(*exec.Cmd) ([]byte, error) {
		whileHeld = free()
		return nil, nil
	})
	if whileHeld {
		t.Fatal("a second child could take a slot while the only one was held; the bound on children is gone")
	}
	if !free() {
		t.Fatal("the only slot was still held after its child returned, so the next child would wait forever")
	}
}

// TestEveryChildTakesASlot reads this package's own source. A call site that
// runs a child process directly is a child the bound does not count.
func TestEveryChildTakesASlot(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	direct := regexp.MustCompile(`\.(CombinedOutput|Output|Run|Start)\(\)`)
	read := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		read++
		for i, ln := range strings.Split(string(b), "\n") {
			if direct.MatchString(ln) && !isComment(ln) {
				t.Errorf("%s:%d runs a child with no slot held; use combined or output: %s", f, i+1, strings.TrimSpace(ln))
			}
		}
	}
	// ⛔ A SCAN THAT READ NOTHING FOUND NOTHING.
	if read == 0 {
		t.Fatal("no source file was read, so nothing was checked")
	}
}

// TestBalanceDealsEveryFileOnce is the guard that matters most in a share: a
// file dealt to no PowerShell session is never analysed and never named.
func TestBalanceDealsEveryFileOnce(t *testing.T) {
	cases := []struct {
		weight []int64
		k      int
	}{
		{nil, 4},
		{[]int64{5}, 4},
		{[]int64{1, 1, 1, 1}, 2},
		{[]int64{90, 3, 50, 7, 7, 1, 64}, 3},
		{[]int64{2, 9, 4}, 10},
		{[]int64{3, 3, 3}, 0},
	}
	for _, c := range cases {
		groups := balance(c.weight, c.k)
		if want := max(1, min(c.k, len(c.weight))); len(groups) != want {
			t.Fatalf("balance(%v, %d) made %d groups, want %d", c.weight, c.k, len(groups), want)
		}
		seen := map[int]int{}
		for _, g := range groups {
			for j, i := range g {
				seen[i]++
				if j > 0 && g[j-1] >= i {
					t.Fatalf("balance(%v, %d) gave a group out of order: %v", c.weight, c.k, groups)
				}
			}
		}
		for i := range c.weight {
			if seen[i] != 1 {
				t.Fatalf("balance(%v, %d) dealt file %d %d times: %v", c.weight, c.k, i, seen[i], groups)
			}
		}
		if len(seen) != len(c.weight) {
			t.Fatalf("balance(%v, %d) dealt an index that is no file: %v", c.weight, c.k, groups)
		}
	}
}

// TestTheModulesAreCheckedAtOnce holds GoModules' fan-out: module a waits for
// module b to start. And the findings come back in module order, whichever
// module finished first.
func TestTheModulesAreCheckedAtOnce(t *testing.T) {
	withSlots(t, 2)
	bStarted := make(chan struct{})
	got := goModules([]string{"a", "b"}, func(m string) []string {
		if m == "b" {
			close(bStarted)
			return []string{"b: go vet: one", "b: go test: two"}
		}
		select {
		case <-bStarted:
			return []string{"a: go build: three"}
		case <-time.After(10 * time.Second):
			return []string{"a: waited ten seconds for b, so the modules ran one after another"}
		}
	})
	want := []string{"a: go build: three", "b: go vet: one", "b: go test: two"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("goModules gave %q, want %q", got, want)
	}
}
