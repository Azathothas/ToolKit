// pool_test.go - the workers' slots and the state each one owns.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// privateTemp points the temporary directory at a fresh one. The slots a case
// takes are then its own, not those of another run on the host.
func privateTemp(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	if os.TempDir() != tmp {
		t.Skipf("this host resolves TempDir to %s rather than the fixture", os.TempDir())
	}
	return tmp
}

// lastValue is what a process started with env reads for key.
func lastValue(env []string, key string) (string, bool) {
	val, ok := "", false
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if k == key || (runtime.GOOS == "windows" && strings.EqualFold(k, key)) {
			val, ok = v, true
		}
	}
	return val, ok
}

// TestEachWorkerHasItsOwnState. ⛔ Two test processes never share a temporary
// directory, a home or a wsl-toolkit state directory, and both keep the host's
// build cache.
func TestEachWorkerHasItsOwnState(t *testing.T) {
	parent := []string{"PATH=/bin", "WSL_TOOLKIT_HOME=/shared", "HOME=/operator", "XDG_STATE_HOME=/operator/state"}
	goEnv := map[string]string{"GOCACHE": "/cache", "GOMODCACHE": "/mod", "GOPATH": "/gopath", "GOENV": "/goenv"}
	a, b := filepath.Join("slots", "w1"), filepath.Join("slots", "w2")
	envA, envB := workerEnv(parent, a, goEnv), workerEnv(parent, b, goEnv)
	for _, k := range []string{"TMPDIR", "TMP", "TEMP", "HOME", "USERPROFILE", "LOCALAPPDATA", "WSL_TOOLKIT_HOME"} {
		va, okA := lastValue(envA, k)
		vb, _ := lastValue(envB, k)
		if !okA || !strings.HasPrefix(va, a) {
			t.Errorf("%s is %q for the worker in %s", k, va, a)
		}
		if va == vb {
			t.Errorf("%s is %q for two workers", k, va)
		}
	}
	for k, want := range goEnv {
		if got, _ := lastValue(envA, k); got != want {
			t.Errorf("%s is %q, want the host's %q", k, got, want)
		}
	}
	// ⚠ DROPPED, so it follows the worker's HOME rather than the operator's.
	if v, ok := lastValue(envA, "XDG_STATE_HOME"); ok {
		t.Errorf("XDG_STATE_HOME reaches the worker as %q", v)
	}
	if v, _ := lastValue(envA, "PATH"); v != "/bin" {
		t.Errorf("PATH is %q, and the toolchain is found through it", v)
	}
	// ⛔ THE ANCESTRY IS ASKED FOR, and a setting the parent had is kept
	// before it. gotest.go ties a death to a case through it.
	want := "tracebackancestors=" + strconv.Itoa(ancestry)
	if v, _ := lastValue(envA, "GODEBUG"); v != want {
		t.Errorf("GODEBUG is %q, want %q", v, want)
	}
	withOwn := workerEnv(append(parent, "GODEBUG=madvdontneed=1"), a, goEnv)
	if v, _ := lastValue(withOwn, "GODEBUG"); v != "madvdontneed=1,"+want {
		t.Errorf("GODEBUG is %q over a parent's madvdontneed=1, want both", v)
	}
}

// TestASlotIsAFixedPath: the first free slot is w1 on every run, so Go's build
// cache, which keys a compile on its directory, hits the next time.
func TestASlotIsAFixedPath(t *testing.T) {
	tmp := privateTemp(t)
	want := filepath.Join(tmp, "repo-mutate", "w1")
	for run := 1; run <= 2; run++ {
		s, err := takeSlot()
		if err != nil {
			t.Fatal(err)
		}
		if s.dir != want {
			s.release()
			t.Fatalf("run %d staged in %s, want %s", run, s.dir, want)
		}
		s.release()
	}
}

// TestAHeldSlotIsPassedOver: two runs on one host never stage into one
// directory.
func TestAHeldSlotIsPassedOver(t *testing.T) {
	privateTemp(t)
	a, err := takeSlot()
	if err != nil {
		t.Fatal(err)
	}
	defer a.release()
	b, err := takeSlot()
	if err != nil {
		t.Fatal(err)
	}
	defer b.release()
	if a.dir == b.dir {
		t.Fatalf("two holders share %s", a.dir)
	}
}

// TestTheSweepSparesAHeldSlot: a slot nobody holds was left by a killed run
// and is removed; a held one is a live run's and is not.
func TestTheSweepSparesAHeldSlot(t *testing.T) {
	tmp := privateTemp(t)
	held, err := takeSlot()
	if err != nil {
		t.Fatal(err)
	}
	defer held.release()
	if err := os.WriteFile(filepath.Join(held.dir, "live"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	left := filepath.Join(tmp, "repo-mutate", "w7")
	if err := os.MkdirAll(left, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(left, "stale"), make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	sweepSlots(&log)
	if _, err := os.Stat(left); !os.IsNotExist(err) {
		t.Fatalf("the abandoned slot survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(held.dir, "live")); err != nil {
		t.Fatalf("a held slot was swept: %v", err)
	}
	if !strings.Contains(log.String(), "swept 1 slot") {
		t.Fatalf("the sweep did not say what it did: %q", log.String())
	}
}

// TestAStagedCopyIsPutBackAsItWasStaged: what a test process can leave in its
// package directory, and the row's own file, which stays.
//
// ⛔ A FILE REWRITTEN WITH ITS OWN BYTES IS NOT NAMED. The restore of every
// mutated file does that, and a note on every row is one nobody reads.
func TestAStagedCopyIsPutBackAsItWasStaged(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	write := func(root, rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	write(src, "a.go", "package a\n")
	write(src, "same.txt", "same\n")
	write(src, "mutated.go", "package a\n")
	write(src, "sub/b.txt", "bee\n")
	write(src, "sub/c.txt", "sea\n")
	if err := copyTree(src, dest); err != nil {
		t.Fatal(err)
	}
	// ⚠ STAGED AN HOUR AGO. A filesystem's clock can be coarser than the gap
	// between two writes here, and a compile always sits between the two in a
	// real run.
	earlier := time.Now().Add(-time.Hour)
	for _, rel := range []string{"a.go", "same.txt", "mutated.go", "sub/b.txt", "sub/c.txt"} {
		if err := os.Chtimes(filepath.Join(dest, filepath.FromSlash(rel)), earlier, earlier); err != nil {
			t.Fatal(err)
		}
	}
	stamps, err := stampTree(dest)
	if err != nil {
		t.Fatal(err)
	}
	s := &staging{src: src, stamps: stamps}

	write(dest, "left.txt", "a new file")
	write(dest, "newdir/deep/f.txt", "a new directory")
	write(dest, "a.go", "package a // longer\n")
	write(dest, "sub/b.txt", "BEE\n")
	if err := os.Remove(filepath.Join(dest, "sub", "c.txt")); err != nil {
		t.Fatal(err)
	}
	write(dest, "same.txt", "same\n")
	write(dest, "mutated.go", "package a // the row's own change\n")

	moved, err := putBack(dest, s, "mutated.go")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(moved, ","), "a.go,left.txt,newdir,sub/b.txt,sub/c.txt"; got != want {
		t.Errorf("put back %s, want %s", got, want)
	}
	for _, rel := range []string{"left.txt", "newdir"} {
		if _, err := os.Stat(filepath.Join(dest, rel)); !os.IsNotExist(err) {
			t.Errorf("%s the copy did not hold is still there: %v", rel, err)
		}
	}
	for rel, want := range map[string]string{"a.go": "package a\n", "sub/b.txt": "bee\n", "sub/c.txt": "sea\n", "same.txt": "same\n", "mutated.go": "package a // the row's own change\n"} {
		if got := read(rel); got != want {
			t.Errorf("%s holds %q, want %q", rel, got, want)
		}
	}
	if again, err := putBack(dest, s, "mutated.go"); err != nil || len(again) != 0 {
		t.Errorf("a copy already put back was changed again: %v, %v", again, err)
	}
}

// TestACopyThatWillNotGoBackIsStagedAgain: a test process changed a staged file
// whose source has gone from the tree, so nothing can put it back.
//
// ⛔ THE COPY IS DROPPED, NOT KEPT. A copy left half put back is the one the
// next row would run in, and it says so.
func TestACopyThatWillNotGoBackIsStagedAgain(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "mod")
	if err := os.MkdirAll(tree, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "a.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	wk := &worker{root: root, slot: &slot{dir: t.TempDir()}, dirs: map[string]string{}, staged: map[string]*staging{}, p: newPrinter(&log, 8, 1, 1)}
	dest, err := wk.dir("mod")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "a.go"), []byte("package a // changed by a case\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(tree, "a.go")); err != nil {
		t.Fatal(err)
	}
	wk.doing(`"a row"`)
	wk.settle(dest, "")
	if _, ok := wk.dirs["mod"]; ok {
		t.Fatal("a copy that could not be put back is still the one the next row runs in")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("the dropped copy is still on disk: %v", err)
	}
	if !strings.Contains(log.String(), `restaged: the copy of mod could not be put back after "a row"`) {
		t.Fatalf("the log does not say the copy was dropped:\n%s", log.String())
	}
}

// TestDefaultJobsIsHalfTheProcessors. ⭐ Measured: one row uses about two of
// four processors, and four workers on four were slower than three.
func TestDefaultJobsIsHalfTheProcessors(t *testing.T) {
	if got, want := DefaultJobs(), max(1, runtime.NumCPU()/2); got != want {
		t.Fatalf("DefaultJobs is %d on %d processors, want %d", got, runtime.NumCPU(), want)
	}
}
