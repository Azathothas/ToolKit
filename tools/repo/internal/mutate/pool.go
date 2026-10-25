// pool.go - rows run side by side, each worker in a world of its own.
//
// ⭐ ONE ROW USES ABOUT TWO OF FOUR PROCESSORS. A `go test` run is a compile, a
// link and a short test, and much of it is serial. Rows ran one after another
// with half the machine idle. Measured: two workers ran a share 1.8 times as
// fast, three 2.2 times, and four was slower.
//
// ⛔ TWO TEST PROCESSES NEVER SHARE STATE. Each worker has its own staged copy
// of every module it touches and its own TMPDIR, TMP, TEMP, HOME, USERPROFILE,
// LOCALAPPDATA and WSL_TOOLKIT_HOME. A case that writes a fixed name there
// would otherwise meet a neighbour's file. It would go red for a reason the
// mutation did not cause.
//
// ⭐ THE COPY SITS AT A FIXED PATH, w<k> under repo-mutate in the temporary
// directory. Go's build cache keys a compile on the package's directory, so a
// copy at a new random path recompiled every package on every run. The same
// path hits the cache across rows and across runs.
//
// ⚠ A FIXED PATH NEEDS A LOCK, because two runs on one host would otherwise
// stage into the same directory. Each slot has a lock file the operating system
// releases when the process exits, so a killed run never holds one. A slot
// another run holds is passed over for the next number.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// DefaultJobs is half this host's processors, and at least one.
func DefaultJobs() int {
	return max(1, runtime.NumCPU()/2)
}

// slotRoot is where the fixed slots live.
func slotRoot() string { return filepath.Join(os.TempDir(), "repo-mutate") }

// maxSlots bounds the search for a free slot. Past it a worker stages into a
// private directory, which still works and only misses the cache.
const maxSlots = 256

// slot is one worker's directory, held for the length of a run.
type slot struct {
	dir    string
	unlock func()
}

// takeSlot claims the lowest free slot.
func takeSlot() (*slot, error) {
	root := slotRoot()
	if err := os.MkdirAll(root, 0o700); err == nil {
		for n := 1; n <= maxSlots; n++ {
			unlock, ok := lockSlot(filepath.Join(root, fmt.Sprintf("w%d.lock", n)))
			if !ok {
				continue
			}
			dir := filepath.Join(root, fmt.Sprintf("w%d", n))
			// ⛔ EMPTIED BEFORE USE. A killed run leaves its copy here, and a
			// copy laid over it would keep any file the module has since lost.
			if err := os.RemoveAll(dir); err == nil {
				if err := os.MkdirAll(dir, 0o700); err == nil {
					return &slot{dir: dir, unlock: unlock}, nil
				}
			}
			unlock()
		}
	}
	dir, err := os.MkdirTemp("", "mutate-")
	if err != nil {
		return nil, err
	}
	return &slot{dir: dir}, nil
}

// release removes the slot's contents and lets another run take it.
func (s *slot) release() {
	_ = os.RemoveAll(s.dir)
	if s.unlock != nil {
		s.unlock()
	}
}

// sweepSlots removes every slot no live run holds. ⚠ The lock is the test,
// not an age. A slot nobody can lock is in use, and one anybody can lock was
// left by a killed run.
func sweepSlots(w io.Writer) {
	root := slotRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	removed := 0
	var bytes int64
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "w") {
			continue
		}
		unlock, ok := lockSlot(filepath.Join(root, e.Name()+".lock"))
		if !ok {
			continue
		}
		p := filepath.Join(root, e.Name())
		size := treeSize(p)
		if err := os.RemoveAll(p); err == nil {
			removed++
			bytes += size
		}
		unlock()
	}
	if removed > 0 && w != nil {
		fmt.Fprintf(w, "  swept %d slot(s) a killed run left behind, %d MiB\n", removed, bytes>>20)
	}
}

// moduleKey names a module's copy inside a slot: readable, and distinct for
// two modules whose paths flatten alike.
func moduleKey(module string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(module))
	return fmt.Sprintf("%s-%08x", path.Base(module), h.Sum32())
}

// goEnvKeys are the Go settings a worker's fake HOME would otherwise move.
//
// ⛔ READ ONCE FROM THE PARENT AND HANDED DOWN. GOCACHE, GOPATH and GOENV
// default to places under HOME or LOCALAPPDATA. A worker with its own HOME and
// no GOCACHE would compile everything from nothing. With no GOMODCACHE it would
// download what the host already holds.
var goEnvKeys = []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOENV"}

// readGoEnv asks the toolchain where those settings point now.
func readGoEnv(dir string) (map[string]string, error) {
	cmd := exec.Command("go", append([]string{"env"}, goEnvKeys...)...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go env could not be read, so a worker would lose the build cache: %w", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	got := map[string]string{}
	for i, k := range goEnvKeys {
		if i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			got[k] = strings.TrimSpace(lines[i])
		}
	}
	return got, nil
}

// workerEnv is the parent's environment with the worker's own state on top.
//
// ⚠ THE XDG DIRECTORIES ARE DROPPED, not pointed somewhere. Unset, they follow
// HOME, which is the worker's; a runner that sets one would otherwise hand every
// worker the same directory.
func workerEnv(parent []string, dir string, goEnv map[string]string) []string {
	drop := map[string]bool{"XDG_CONFIG_HOME": true, "XDG_CACHE_HOME": true, "XDG_STATE_HOME": true, "XDG_DATA_HOME": true}
	var env []string
	for _, kv := range parent {
		k, _, _ := strings.Cut(kv, "=")
		if drop[strings.ToUpper(k)] {
			continue
		}
		env = append(env, kv)
	}
	tmp := filepath.Join(dir, "tmp")
	home := filepath.Join(dir, "home")
	set := []string{
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"HOME=" + home, "USERPROFILE=" + home,
		"LOCALAPPDATA=" + filepath.Join(dir, "localappdata"),
		"WSL_TOOLKIT_HOME=" + filepath.Join(dir, "wsl-toolkit"),
	}
	for _, k := range goEnvKeys {
		if v := goEnv[k]; v != "" {
			set = append(set, k+"="+v)
		}
	}
	// ⚠ APPENDED, NOT SPLICED. exec uses the last value of a repeated key,
	// and on Windows it compares keys without regard to case.
	return append(env, set...)
}

// worker runs groups in its own slot.
type worker struct {
	root string
	slot *slot
	env  []string
	dirs map[string]string
}

func newWorker(root string, s *slot, goEnv map[string]string) (*worker, error) {
	for _, d := range []string{"tmp", "home", "localappdata"} {
		if err := os.MkdirAll(filepath.Join(s.dir, d), 0o700); err != nil {
			return nil, err
		}
	}
	return &worker{root: root, slot: s, env: workerEnv(os.Environ(), s.dir, goEnv), dirs: map[string]string{}}, nil
}

// dir returns the worker's staged copy of the module, making it on first use.
func (wk *worker) dir(module string) (string, error) {
	if d, ok := wk.dirs[module]; ok {
		return d, nil
	}
	dest := filepath.Join(wk.slot.dir, "src", moduleKey(module))
	if err := os.RemoveAll(dest); err != nil {
		return "", err
	}
	if err := copyTree(filepath.Join(wk.root, filepath.FromSlash(module)), dest); err != nil {
		_ = os.RemoveAll(dest)
		return "", err
	}
	wk.dirs[module] = dest
	return dest, nil
}

// drop forgets a staged copy, so the next row that needs it makes a fresh one
// at the same path.
func (wk *worker) drop(module string) {
	if d, ok := wk.dirs[module]; ok {
		_ = os.RemoveAll(d)
		delete(wk.dirs, module)
	}
}

// goTest runs `go test` in dir with the worker's environment.
func (wk *worker) goTest(dir string, args ...string) (testRun, error) {
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	cmd.Dir = dir
	cmd.Env = wk.env
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return readTestRun(stdout.String(), stderr.String()), err
}

// printer writes verdicts in table order as the finished prefix grows.
//
// ⚠ TABLE ORDER, NOT FINISHING ORDER. Two runs of one table must print the
// same lines in the same order, or a diff of two logs is noise.
type printer struct {
	mu    sync.Mutex
	w     io.Writer
	width int
	done  []bool
	got   []Verdict
	next  int
}

func (p *printer) put(i int, v Verdict) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.got[i], p.done[i] = v, true
	for p.next < len(p.done) && p.done[p.next] {
		fmt.Fprint(p.w, verdictLine(p.got[p.next], p.width))
		p.next++
	}
}
