// pool_test.go - the workers' slots and the state each one owns.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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

// TestDefaultJobsIsHalfTheProcessors. ⭐ Measured: one row uses about two of
// four processors, and four workers on four were slower than three.
func TestDefaultJobsIsHalfTheProcessors(t *testing.T) {
	if got, want := DefaultJobs(), max(1, runtime.NumCPU()/2); got != want {
		t.Fatalf("DefaultJobs is %d on %d processors, want %d", got, runtime.NumCPU(), want)
	}
}
