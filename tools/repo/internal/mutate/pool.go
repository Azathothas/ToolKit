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
// ⛔ AND NOR DO TWO THAT RUN ONE AFTER ANOTHER ON ONE WORKER. That state is
// emptied before every test process. It used to be made once per run, so a
// file one row's mutated run left behind reached whichever group the worker
// took next. Which group that was depended on finishing times, so one table
// gave different verdicts at different --jobs. Deleting a cleanup guard leaves
// exactly such a file. ⚠ The staged copy is the exception, because the build
// cache needs its path. Only the mutated file is put back there, so a case
// writes its files under the temporary directory and never beside its source.
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
	"strconv"
	"strings"
	"sync"
	"time"
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
	// ⛔ THE ANCESTRY IS WHAT TIES A DEATH TO A CASE. A goroutine the case
	// started through the code under test names only its creator. Its own
	// trace never reaches testing.tRunner, so gotest.go could not tell it from
	// one that init started. A setting the parent already has is kept, and this
	// one is added after it.
	godebug := "tracebackancestors=" + strconv.Itoa(ancestry)
	if v, ok := lookupEnv(parent, "GODEBUG"); ok && v != "" {
		godebug = v + "," + godebug
	}
	set = append(set, "GODEBUG="+godebug)
	// ⚠ APPENDED, NOT SPLICED. exec uses the last value of a repeated key,
	// and on Windows it compares keys without regard to case.
	return append(env, set...)
}

// ancestry is how many creating goroutines a trace names. A case goroutine is
// one or two hops from any goroutine the code under test starts.
const ancestry = 16

// lookupEnv is the value a process started with env reads for key.
func lookupEnv(env []string, key string) (string, bool) {
	val, ok := "", false
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if k == key || (runtime.GOOS == "windows" && strings.EqualFold(k, key)) {
			val, ok = v, true
		}
	}
	return val, ok
}

// worker runs groups in its own slot.
type worker struct {
	root   string
	slot   *slot
	parent []string
	goEnv  map[string]string
	env    []string
	dirs   map[string]string
	// gen numbers the state directory. It moves on only when the last one
	// could not be emptied.
	gen int
	// id and p are where the worker says what it is running.
	id int
	p  *printer
}

func newWorker(root string, s *slot, goEnv map[string]string) (*worker, error) {
	wk := &worker{root: root, slot: s, parent: os.Environ(), goEnv: goEnv, dirs: map[string]string{}}
	if err := wk.fresh(); err != nil {
		return nil, err
	}
	return wk, nil
}

// fresh empties the worker's own state before a test process starts.
//
// ⚠ A DIRECTORY THAT WILL NOT EMPTY IS LEFT FOR THE SLOT'S RELEASE, and the
// next number is used. A process a case left running still holds its files on
// Windows. A new directory beside it is just as empty.
func (wk *worker) fresh() error {
	var last error
	for tries := 0; tries < 8; tries++ {
		dir := filepath.Join(wk.slot.dir, "state"+strconv.Itoa(wk.gen))
		if err := os.RemoveAll(dir); err != nil {
			last = err
			wk.gen++
			continue
		}
		for _, d := range []string{"tmp", "home", "localappdata"} {
			if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
				return err
			}
		}
		wk.env = workerEnv(wk.parent, dir, wk.goEnv)
		return nil
	}
	return fmt.Errorf("the worker's own state could not be emptied: %w", last)
}

// doing tells the printer what the worker runs now.
func (wk *worker) doing(what string) {
	if wk.p != nil {
		wk.p.start(wk.id, what)
	}
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

// goTest runs `go test` in dir with the worker's environment, emptied first.
func (wk *worker) goTest(dir string, args ...string) (testRun, error) {
	if err := wk.fresh(); err != nil {
		return testRun{text: err.Error()}, err
	}
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	cmd.Dir = dir
	cmd.Env = wk.env
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return readTestRun(stdout.String(), stderr.String()), err
}

// printer writes each verdict as its row finishes. When nothing has been
// written for a while, it says what each worker is running.
//
// ⛔ FINISHING ORDER, NOT TABLE ORDER. Groups are handed out heaviest first,
// and the table opens with light rows. A printer that held each line until the
// rows above it were done printed a CI shard's first verdict at 97% of the run.
// A shard the time limit killed left no verdict in its log, and a hung row could
// not be found. ⭐ Each line carries the row's place in the table, as [37/201].
// Report lists every row that is not ok in table order, so two logs still
// compare on that section.
type printer struct {
	mu    sync.Mutex
	w     io.Writer
	width int
	got   []Verdict
	done  int
	last  time.Time
	busy  []activity
}

// activity is what one worker runs, and since when.
type activity struct {
	what  string
	since time.Time
}

func newPrinter(w io.Writer, width, rows, workers int) *printer {
	return &printer{w: w, width: width, got: make([]Verdict, rows), busy: make([]activity, workers), last: time.Now()}
}

// put records row i's verdict and prints it now.
func (p *printer) put(i int, v Verdict) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.got[i] = v
	p.done++
	n := len(p.got)
	fmt.Fprintf(p.w, "  [%*d/%d] %s", len(strconv.Itoa(n)), i+1, n, verdictLine(v, p.width))
	p.last = time.Now()
}

// note prints a line that is not a verdict.
func (p *printer) note(line string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprint(p.w, line)
	p.last = time.Now()
}

// start records what worker k runs from now on.
func (p *printer) start(k int, what string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if k >= 0 && k < len(p.busy) {
		p.busy[k] = activity{what: what, since: time.Now()}
	}
}

// beat names what every worker runs, when nothing was printed for every.
func (p *printer) beat(every time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if now.Sub(p.last) < every {
		return
	}
	var running []string
	for k, a := range p.busy {
		if a.what != "" {
			running = append(running, fmt.Sprintf("worker %d on %s for %s", k+1, a.what, now.Sub(a.since).Round(time.Second)))
		}
	}
	fmt.Fprintf(p.w, "  still running, %d of %d row(s) done: %s\n", p.done, len(p.got), strings.Join(running, "; "))
	p.last = now
}

// watch beats every interval until stop closes, and closes done after.
func (p *printer) watch(every time.Duration, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			p.beat(every)
		}
	}
}
