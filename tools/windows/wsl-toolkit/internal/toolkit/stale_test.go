// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTheStaleRunStateVerdictIsReadFromWhatTheGuestSaid holds what `run`, `matrix`
// and `base exec` say after the check: nothing over a current state, what was
// removed over repair.sh's own last line, and the command that clears it over a
// stale state that is still there. WSL-137.
func TestTheStaleRunStateVerdictIsReadFromWhatTheGuestSaid(t *testing.T) {
	type verdictCase struct {
		name    string
		out     string
		stderr  string
		code    int
		err     error
		cleared bool
		says    []string
	}
	cases := []verdictCase{
		{name: "current", out: "run-state current\n"},
		{name: "cleared by another caller first", out: "run-state stale: the engine recorded another boot of WSL\nrun-state current: another caller cleared it first\n"},
		{
			name: "cleared",
			out: "run-state stale: the engine recorded another boot of WSL\nrun-base /tmp/r (from the engine)\n" +
				"removed /tmp/r/containers\nremoved /tmp/r/libpod/tmp\nrepaired run-state\n",
			cleared: true,
			says:    []string{"so it was cleared before this started", "removed /tmp/r/containers and /tmp/r/libpod/tmp"},
		},
		{
			name: "no flock",
			out:  "run-state stale: the engine recorded another boot of WSL\nrun-state kept: this base has no flock to clear it safely\n",
			says: []string{"no flock to clear it safely, so a container will be refused. Run: wsl-toolkit base ensure --repair"},
		},
		// ⛔ WSL-178: an earlier start's state on this boot is said as that, and
		// podman does not refuse it. The guest's own reason for keeping it is said.
		{
			name: "an earlier start's state cleared",
			out: "run-state stale: the engine recorded an earlier start of this base\nrun-base /tmp/r (from the engine)\n" +
				"removed /tmp/r/containers\nremoved /tmp/r/libpod/tmp\nrepaired run-state\n",
			cleared: true,
			says:    []string{"the engine's run state was left by an earlier start of this base, so it was cleared before this started: removed /tmp/r/containers"},
		},
		{
			name: "an earlier start's state a container runs on",
			out:  "run-state stale: the engine recorded an earlier start of this base\nrun-state kept: a container of this start of the base runs on it\n",
			says: []string{"left by an earlier start of this base and a container of this start of the base runs on it, so podman may read that start's containers as running. The next job or start of the base clears it"},
		},
		{
			name: "rule 5 refused", out: "run-state stale: the engine recorded another boot of WSL\n",
			stderr: "repair: the engine's run base is /x and the stale boot id was read under /y, so nothing was removed\n",
			code:   1, err: errors.New("exit 1"),
			says: []string{"clearing it failed (exit 1): repair: the engine's run base is /x", "Run: wsl-toolkit base ensure --repair"},
		},
		{
			name: "a repaired line under a failed exit", code: 1, err: errors.New("exit 1"),
			out:  "run-state stale: the engine recorded another boot of WSL\nrepaired run-state\n",
			says: []string{"clearing it failed (exit 1)"},
		},
		{
			name: "a zero exit that said nothing", out: "",
			says: []string{"exited 0 without saying what it found"},
		},
		{
			name: "could not ask", stderr: "wsl: something broke\nfatal: no shell\n", code: 2, err: errors.New("exit 2"),
			says: []string{"could not be checked for a stale boot id (exit 2): fatal: no shell"},
		},
	}
	for _, c := range cases {
		cleared, says := staleRunStateVerdict(c.out, c.stderr, c.code, c.err)
		if cleared != c.cleared {
			t.Errorf("%s: cleared %v, want %v", c.name, cleared, c.cleared)
		}
		if len(c.says) == 0 && says != "" {
			t.Errorf("%s: said %q over a state with nothing to say", c.name, says)
		}
		for _, want := range c.says {
			if !strings.Contains(says, want) {
				t.Errorf("%s: said %q, want a sentence carrying %q", c.name, says, want)
			}
		}
	}
}

// ⭐ A FAKE wsl.exe, AND IT IS THIS TEST BINARY. A Wsl whose Path names the binary
// running the suite reaches init below in a child process, which records the
// call, answers it, and exits before the testing package reads a flag. So a job
// runs through the real Runner on any host, with no WSL at all. WSL-137.
//
// ⚠ It answers by what a script SAYS, not by running it: the line each caller
// reads back is the line it prints.
const staleFakeWslEnv = "WSL_TOOLKIT_STALE_FAKE_WSL"

func init() {
	if dir := os.Getenv(staleFakeWslEnv); dir != "" {
		os.Exit(staleFakeWsl(dir, os.Args[1:], os.Stdin, os.Stdout))
	}
}

// staleFakeWsl answers one call: `-d NAME -u USER [--cd DIR] -- ARGV...`. Each
// call is appended to the directory's calls file as KIND@USER.
func staleFakeWsl(dir string, args []string, stdin io.Reader, stdout io.Writer) int {
	raw, _ := io.ReadAll(stdin)
	if len(args) < 6 || args[0] != "-d" || args[2] != "-u" {
		return 0
	}
	user, rest := args[3], args[4:]
	for len(rest) > 0 && rest[0] != "--" {
		rest = rest[1:]
	}
	kind := "argv"
	if len(rest) > 1 && rest[1] == "/bin/sh" {
		kind = staleFakeKind(string(raw))
	}
	record := kind + "@" + user
	if kind == "stale" && bytes.HasPrefix(raw, guestRuntimePrologue()) {
		record += "+prologue"
	}
	if f, err := os.OpenFile(filepath.Join(dir, "calls"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		fmt.Fprintln(f, record)
		_ = f.Close()
	}
	switch kind {
	case "home":
		fmt.Fprintln(stdout, "/home/toolkit")
	case "stale":
		answer, err := os.ReadFile(filepath.Join(dir, "stale-answer"))
		if err != nil {
			answer = []byte("run-state current\n")
		}
		fmt.Fprint(stdout, string(answer))
	case "teardown":
		if _, err := os.Stat(filepath.Join(dir, "teardown-fails")); err == nil {
			return 1
		}
		fmt.Fprintln(stdout, "removed")
	}
	return 0
}

// staleFakeKind names a script by the line its caller reads back.
func staleFakeKind(script string) string {
	switch {
	case strings.Contains(script, "printf 'run-state current\\n'"):
		return "stale"
	case strings.Contains(script, `printf '%s\n' "$HOME"`):
		return "home"
	case strings.Contains(script, "exec podman"):
		return "job"
	case strings.Contains(script, "podman rm -f"):
		return "teardown"
	}
	return "other"
}

// TestRunClearsAStaleBootIDBeforeItsContainer drives a job through the fake WSL
// and holds the order: the stale check, as the base's account and with its
// runtime directory set, before the container. ⛔ Finding 67: only `base ensure
// --repair` cleared the state, one base at a time, so the first `run` after
// `wsl --shutdown` exited 2. WSL-137.
func TestRunClearsAStaleBootIDBeforeItsContainer(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(staleFakeWslEnv, dir)
	t.Setenv("WSL_TOOLKIT_HOME", home)
	answer := "run-state stale: the engine recorded another boot of WSL\nrun-base /tmp/wsl-toolkit-run-1000 (from the engine)\n" +
		"removed /tmp/wsl-toolkit-run-1000/containers\nremoved /tmp/wsl-toolkit-run-1000/libpod/tmp\nrepaired run-state\n"
	if err := os.WriteFile(filepath.Join(dir, "stale-answer"), []byte(answer), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Base: BaseConfig{Name: "wsl-toolkit-stale", Image: "docker.io/library/alpine:latest", User: "agent"}}
	var logged []string
	log := func(s string) { logged = append(logged, s) }
	r := &Runner{cfg: cfg, wsl: &Wsl{Path: self}, home: home, ledger: &Ledger{path: filepath.Join(home, "ledger.jsonl")}, log: log}
	res := r.Run(t.Context(), JobSpec{
		Image: "docker.io/library/alpine:latest", Script: []byte("true\n"),
		ContainerLifecycle: ContainerEphemeral,
	})
	if res.Unreached {
		t.Fatalf("the job did not reach its container: %s", res.Error)
	}
	data, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Fields(string(data))
	stale, job := -1, -1
	for i, c := range calls {
		switch c {
		case "stale@agent+prologue":
			if stale < 0 {
				stale = i
			}
		case "job@agent":
			if job < 0 {
				job = i
			}
		}
	}
	if stale < 0 || job < 0 || stale > job {
		t.Fatalf("the stale check ran at %d and the container at %d; calls %v", stale, job, calls)
	}
	said := strings.Join(logged, "\n")
	if !strings.Contains(said, "so it was cleared before this started") ||
		!strings.Contains(said, "removed /tmp/wsl-toolkit-run-1000/containers and /tmp/wsl-toolkit-run-1000/libpod/tmp") {
		t.Errorf("a cleared run state was not said, with what was removed: %q", logged)
	}
}

// staleHarness stages a runtime directory, a stand-in podman and an alive
// record, and runs the scripts `run` and `base exec` send, after the prologue.
type staleHarness struct {
	root, bin, fdLog string
}

// newStaleHarness is a staged run root. ⚠ The prologue is not sent: it would
// point XDG_RUNTIME_DIR at this host's own /tmp/wsl-toolkit-run-UID.
func newStaleHarness(t *testing.T, alive string, podmanRunRoot string) staleHarness {
	t.Helper()
	posixShell(t)
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("this host has no flock, and the check takes one")
	}
	if _, err := os.Stat("/proc/sys/kernel/random/boot_id"); err != nil {
		t.Skip("this host has no boot id to compare against")
	}
	dir := t.TempDir()
	h := staleHarness{root: filepath.Join(dir, "run"), bin: filepath.Join(dir, "bin"), fdLog: filepath.Join(dir, "fd.log")}
	for _, d := range []string{h.root + "/containers/overlay", h.root + "/libpod/tmp", h.root + "/crun", h.bin} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if alive != "" {
		if err := os.WriteFile(h.root+"/libpod/tmp/alive", []byte(alive), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if podmanRunRoot == "" {
		podmanRunRoot = h.root + "/containers"
	}
	// ⚠ The stand-in records whether it was handed the lock's descriptor.
	writeStandIn(t, h.bin, "podman", `[ -e "/proc/$$/fd/9" ] && echo leaked >> "$TK_FD_LOG"
printf '%s\n' '`+podmanRunRoot+`'`)
	return h
}

func (h staleHarness) env() []string {
	return []string{"PATH=" + h.bin + ":/usr/bin:/bin", "XDG_RUNTIME_DIR=" + h.root, "TK_FD_LOG=" + h.fdLog}
}

func (h staleHarness) lock() string { return filepath.Join(h.root, ".wsl-toolkit-run-state.lock") }

// command is the payload after its prologue: stale.sh then repair.sh, or
// repair.sh alone as `base ensure --repair` sends it.
func (h staleHarness) command(script []byte, stdout, stderr io.Writer) *exec.Cmd {
	cmd := exec.Command("/bin/sh", "-s")
	cmd.Stdin = bytes.NewReader(script)
	cmd.Env = h.env()
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd
}

// ⚠ It carries bootmark, as every payload that ships does. WSL-178.
func staleOnlyPayload() []byte {
	return runStateChecks()
}

func (h staleHarness) run(t *testing.T) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := exitOf(t, h.command(staleOnlyPayload(), &stdout, &stderr).Run())
	return stdout.String(), stderr.String(), code
}

func exitOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var exited *exec.ExitError
	if !errors.As(err, &exited) {
		t.Fatal(err)
	}
	return exited.ExitCode()
}

func (h staleHarness) present(leaf string) bool {
	_, err := os.Stat(filepath.Join(h.root, leaf))
	return err == nil
}

func (h staleHarness) leaked() bool {
	b, _ := os.ReadFile(h.fdLog)
	return len(b) > 0
}

// holdLock takes the run-state lock in another process, as a second caller
// would, and answers the function that lets it go.
func (h staleHarness) holdLock(t *testing.T) func() {
	t.Helper()
	holder := exec.Command("flock", h.lock(), "/bin/sh", "-c", "read x")
	holder.Env = h.env()
	in, err := holder.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		_ = in.Close()
		_ = holder.Wait()
	}
	t.Cleanup(release)
	for deadline := time.Now().Add(10 * time.Second); ; {
		probe := exec.Command("flock", "-n", h.lock(), "true")
		if probe.Run() != nil {
			return release
		}
		if time.Now().After(deadline) {
			t.Fatal("the stand-in caller never took the run-state lock")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitFor polls a file until it carries want.
func waitFor(t *testing.T, path, want string) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); ; {
		if b, _ := os.ReadFile(path); strings.Contains(string(b), want) {
			return
		}
		if time.Now().After(deadline) {
			b, _ := os.ReadFile(path)
			t.Fatalf("%s never carried %q: %q", path, want, b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestTheStaleCheckClearsOnlyRunStateFromAnotherBoot runs stale.sh and repair.sh
// in a real shell. ⛔ Only an alive record naming another boot is cleared, and
// only its two directories; a current record, an absent one, an empty one and a
// root the engine does not agree with keep everything. ⛔ The engine is never
// handed the lock's descriptor. WSL-137.
func TestTheStaleCheckClearsOnlyRunStateFromAnotherBoot(t *testing.T) {
	stale := newStaleHarness(t, "00000000-0000-0000-0000-000000000000\n", "")
	out, stderr, code := stale.run(t)
	if code != 0 || !strings.Contains(out, "repaired run-state") {
		t.Fatalf("a record from another boot: exit %d, out %q, stderr %q", code, out, stderr)
	}
	if stale.present("containers") || stale.present("libpod/tmp") {
		t.Error("a record from another boot was answered as cleared and its run state is still there")
	}
	if !stale.present("crun") || !stale.present(".wsl-toolkit-run-state.lock") {
		t.Error("the clear removed something beside the two directories podman names")
	}
	if stale.leaked() {
		t.Error("podman was handed the run-state lock's descriptor, so a process it leaves running holds the lock")
	}

	now, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		t.Fatal(err)
	}
	// ⚠ THE FIRST READ ALONE ANSWERS over a state that is not stale, so a job
	// neither waits on the lock nor reaches repair.sh. repair.sh's second read
	// would keep the state too, and these cases would not see the first one go.
	current := newStaleHarness(t, string(now), "")
	out, _, code = current.run(t)
	if code != 0 || out != "run-state current\n" || !current.present("containers") || !current.present("libpod/tmp/alive") {
		t.Fatalf("this boot's record was cleared or misread: exit %d, out %q", code, out)
	}

	for name, alive := range map[string]string{"a missing record": "", "an empty record": "\n"} {
		h := newStaleHarness(t, alive, "")
		out, _, code = h.run(t)
		if code != 0 || out != "run-state current\n" || !h.present("containers") || !h.present("libpod/tmp") {
			t.Fatalf("%s was not read as current: exit %d, out %q", name, code, out)
		}
	}

	elsewhere := newStaleHarness(t, "00000000-0000-0000-0000-000000000000\n", "/var/tmp/somewhere-else/containers")
	out, stderr, code = elsewhere.run(t)
	if code == 0 || !strings.Contains(stderr, "so nothing was removed") || !elsewhere.present("containers") || !elsewhere.present("libpod/tmp") {
		t.Fatalf("an engine root that disagrees with the stale record: exit %d, out %q, stderr %q", code, out, stderr)
	}
}

// TestTheStaleCheckKeepsTheStateWithoutFlock holds "no lock, no deletion": a base
// with no flock keeps a stale state and says so. WSL-137.
func TestTheStaleCheckKeepsTheStateWithoutFlock(t *testing.T) {
	h := newStaleHarness(t, "00000000-0000-0000-0000-000000000000\n", "")
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("this host has no cat")
	}
	if err := os.Symlink(cat, filepath.Join(h.bin, "cat")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cmd := h.command(staleOnlyPayload(), &stdout, &stderr)
	cmd.Env = []string{"PATH=" + h.bin, "XDG_RUNTIME_DIR=" + h.root, "TK_FD_LOG=" + h.fdLog}
	code := exitOf(t, cmd.Run())
	if code != 0 || !strings.Contains(stdout.String(), "run-state kept: this base has no flock") {
		t.Fatalf("a base with no flock: exit %d, out %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if !h.present("containers") || !h.present("libpod/tmp/alive") {
		t.Error("a base with no flock had its run state removed")
	}
}

// TestTheStaleCheckReadsItsRecordAgainUnderTheLock stands in for a second caller
// that holds the lock and clears the state first, as a matrix's first row does
// while its second waits. ⛔ The waiting caller reads the record again under the
// lock and removes nothing, whether the first caller's engine has rewritten the
// record for this boot or has not started and left no record. WSL-137.
func TestTheStaleCheckReadsItsRecordAgainUnderTheLock(t *testing.T) {
	now, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		posixShell(t)
		t.Skip("this host has no boot id to compare against")
	}
	// What the first caller left while the second waited.
	firstCaller := map[string]func(h staleHarness) error{
		"a record naming this boot": func(h staleHarness) error {
			return os.WriteFile(h.root+"/libpod/tmp/alive", now, 0o644)
		},
		"no record": func(h staleHarness) error { return os.RemoveAll(h.root + "/libpod/tmp") },
	}
	for name, clear := range firstCaller {
		h := newStaleHarness(t, "00000000-0000-0000-0000-000000000000\n", "")
		release := h.holdLock(t)
		outPath := filepath.Join(t.TempDir(), "out")
		outFile, err := os.Create(outPath)
		if err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		cmd := h.command(staleOnlyPayload(), outFile, &stderr)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		waitFor(t, outPath, "run-state stale")
		if err := clear(h); err != nil {
			t.Fatal(err)
		}
		release()
		code := exitOf(t, cmd.Wait())
		_ = outFile.Close()
		out, _ := os.ReadFile(outPath)
		if code != 0 || !strings.Contains(string(out), "another caller cleared it first") || strings.Contains(string(out), "repaired") {
			t.Fatalf("%s: a caller that waited for the lock: exit %d, out %q, stderr %q", name, code, out, stderr.String())
		}
		if !h.present("containers") {
			t.Errorf("%s: a caller that waited for the lock removed the state a live engine had just started on", name)
		}
	}
}

// TestRepairTakesTheRunStateLock holds `base ensure --repair` to the same lock:
// repair.sh alone waits while another caller holds it, then clears as it did
// before. WSL-137.
func TestRepairTakesTheRunStateLock(t *testing.T) {
	h := newStaleHarness(t, "00000000-0000-0000-0000-000000000000\n", "")
	release := h.holdLock(t)
	var stdout, stderr bytes.Buffer
	cmd := h.command(repairPayload(), &stdout, &stderr)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		t.Fatalf("repair.sh ended while another caller held the lock: %v, out %q, stderr %q", err, stdout.String(), stderr.String())
	case <-time.After(1500 * time.Millisecond):
	}
	if !h.present("containers") || !h.present("libpod/tmp") {
		t.Fatal("repair.sh removed the run state while another caller held the lock")
	}
	release()
	var code int
	select {
	case err := <-done:
		code = exitOf(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("repair.sh did not finish after the lock was let go")
	}
	if code != 0 || !strings.Contains(stdout.String(), "repaired run-state") || h.present("containers") || h.present("libpod/tmp") {
		t.Fatalf("repair.sh after the lock: exit %d, out %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if h.leaked() {
		t.Error("podman was handed the run-state lock's descriptor by repair.sh")
	}
}

// TestBaseEnsureRepairReadsWhatRepairShSaid holds `base ensure --repair` to
// repair.sh's own lines: a clear, or a record another caller had already cleared
// and an engine had rewritten for this boot. Both go on to the probe. WSL-137.
func TestBaseEnsureRepairReadsWhatRepairShSaid(t *testing.T) {
	if err := repairRunStateVerdict("run-base /tmp/r (from the engine)\nremoved /tmp/r/containers\nremoved /tmp/r/libpod/tmp\nrepaired run-state\n", "", 0, nil); err != nil {
		t.Errorf("a clear was refused: %v", err)
	}
	// ⛔ Another caller cleared it first and its engine is live, so repair.sh
	// removed nothing. That is not a failed --repair.
	if err := repairRunStateVerdict("run-state current: another caller cleared it first\n", "", 0, nil); err != nil {
		t.Errorf("a state another caller had cleared first failed --repair: %v", err)
	}
	// ⛔ A state kept under a container of this start is no repair. WSL-178.
	if err := repairRunStateVerdict("run-state kept: a container of this start of the base runs on it\n", "", 0, nil); err == nil ||
		err.Error() != "the run state was kept, because a container of this start of the base runs on it" {
		t.Errorf("a state kept under a container of this start: %v", err)
	}
	if err := repairRunStateVerdict("", "", 0, nil); err == nil || !strings.Contains(err.Error(), "without saying what it did") {
		t.Errorf("a zero exit that said nothing: %v", err)
	}
	if err := repairRunStateVerdict("still-present /tmp/r/containers\n", "repair: at least one directory is still there\n", 1, nil); err == nil || !strings.Contains(err.Error(), "repair exited 1") {
		t.Errorf("a failed clear: %v", err)
	}
	if err := repairRunStateVerdict("run-state current: another caller cleared it first\n", "", 1, nil); err == nil {
		t.Error("a non-zero exit was read as a state another caller cleared")
	}
}

// busyboxFlock is a stand-in for BusyBox's flock applet, the one an alpine base
// carries: it takes -s -x -u and -n, refuses anything else as BusyBox does, and
// locks with this host's own flock.
func busyboxFlock(t *testing.T, bin string) {
	t.Helper()
	flockPath, err := exec.LookPath("flock")
	if err != nil {
		t.Skip("this host has no flock")
	}
	writeStandIn(t, bin, "flock", `for a in "$@"; do
  case $a in
    -[sxun]) ;;
    -*) echo "Usage: flock [-sxun] FD | { FILE [-c] PROG ARGS }" >&2; exit 1 ;;
  esac
done
exec '`+flockPath+`' "$@"`)
}

// TestTheClearWorksWithBusyBoxFlock holds both ways in to a flock with no -w.
// ⛔ `flock -w 120 9` failed at once on an alpine base, so `base ensure --repair`,
// which cleared in 7.0.0, exited 1 blaming a lock nobody held, and the automatic
// clear never ran. WSL-137.
func TestTheClearWorksWithBusyBoxFlock(t *testing.T) {
	for name, payload := range map[string][]byte{"base ensure --repair": repairPayload(), "the stale check": staleOnlyPayload()} {
		h := newStaleHarness(t, "00000000-0000-0000-0000-000000000000\n", "")
		busyboxFlock(t, h.bin)
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		var stdout, stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, "/bin/sh", "-s")
		cmd.Stdin = bytes.NewReader(payload)
		cmd.Env = h.env()
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		cancel()
		if err != nil || !strings.Contains(stdout.String(), "repaired run-state") || h.present("containers") || h.present("libpod/tmp") {
			t.Errorf("%s with BusyBox's flock: %v, out %q, stderr %q", name, err, stdout.String(), stderr.String())
		}
	}
}

// TestRepairKeepsAStateAnotherCallerClearedFirst stands in for a job that took
// the lock, cleared, and started its container while `base ensure --repair`
// waited. ⛔ repair.sh reads the record again under the lock, finds it naming this
// boot, and removes nothing under that engine. ⚠ A missing record still clears,
// as it did in 7.0.0. WSL-137.
func TestRepairKeepsAStateAnotherCallerClearedFirst(t *testing.T) {
	h := newStaleHarness(t, "00000000-0000-0000-0000-000000000000\n", "")
	release := h.holdLock(t)
	var stdout, stderr bytes.Buffer
	cmd := h.command(repairPayload(), &stdout, &stderr)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	time.Sleep(1500 * time.Millisecond)
	now, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.root+"/libpod/tmp/alive", now, 0o644); err != nil {
		t.Fatal(err)
	}
	release()
	var code int
	select {
	case err := <-done:
		code = exitOf(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("repair.sh did not finish after the lock was let go")
	}
	if code != 0 || !strings.Contains(stdout.String(), "another caller cleared it first") || strings.Contains(stdout.String(), "removed") {
		t.Fatalf("repair.sh after another caller's clear: exit %d, out %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if !h.present("containers") || !h.present("libpod/tmp/alive") {
		t.Error("base ensure --repair removed the state a live engine of this boot had just started on")
	}

	missing := newStaleHarness(t, "", "")
	stdout.Reset()
	stderr.Reset()
	code = exitOf(t, missing.command(repairPayload(), &stdout, &stderr).Run())
	if code != 0 || !strings.Contains(stdout.String(), "repaired run-state") || missing.present("containers") {
		t.Errorf("base ensure --repair over a missing record no longer clears: exit %d, out %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

// TestTheClearStopsAtTheFirstDirectoryStillThere stands in for a run root that
// will not go. ⛔ libpod/tmp, which holds the stale record, is left, so every
// later check still names the state instead of reading a missing record as
// current over a half-removed run root. WSL-137.
func TestTheClearStopsAtTheFirstDirectoryStillThere(t *testing.T) {
	h := newStaleHarness(t, "00000000-0000-0000-0000-000000000000\n", "")
	rmPath, err := exec.LookPath("rm")
	if err != nil {
		t.Skip("this host has no rm")
	}
	writeStandIn(t, h.bin, "rm", `case "$*" in
  */containers) exit 1 ;;
esac
exec '`+rmPath+`' "$@"`)
	out, stderr, code := h.run(t)
	if code == 0 || !strings.Contains(out, "still-present "+h.root+"/containers") {
		t.Fatalf("a run root that would not go: exit %d, out %q, stderr %q", code, out, stderr)
	}
	if !h.present("libpod/tmp/alive") {
		t.Error("the stale record was removed after the run root failed to go, so later checks read the state as current")
	}
	again, _, _ := h.run(t)
	if !strings.Contains(again, "run-state stale") {
		t.Errorf("the next check no longer names the stale state: %q", again)
	}
}

// TestAHungEngineDoesNotHoldTheLock stands in for a `podman info` that never
// answers. ⛔ repair.sh asks it while it holds the run-state lock, and the call
// that sent it kills wsl.exe and not this shell, so an unbounded ask held the
// lock and every later job waited 120s. The ask is bounded, and the stand-in
// timeout shortens the bound to a second. WSL-137.
func TestAHungEngineDoesNotHoldTheLock(t *testing.T) {
	h := newStaleHarness(t, "00000000-0000-0000-0000-000000000000\n", "")
	timeoutPath, err := exec.LookPath("timeout")
	if err != nil {
		t.Skip("this host has no timeout")
	}
	writeStandIn(t, h.bin, "podman", "exec sleep 600")
	writeStandIn(t, h.bin, "timeout", `printf '%s\n' "$1" >> "$TK_FD_LOG.timeout"
shift
exec '`+timeoutPath+`' 1 "$@"`)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "/bin/sh", "-s")
	cmd.Stdin = bytes.NewReader(staleOnlyPayload())
	cmd.Env = h.env()
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// ⚠ A killed shell leaves the stand-in's sleep holding stdout.
	cmd.WaitDelay = 2 * time.Second
	err = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("repair.sh was still asking a hung engine after 20s: out %q, stderr %q", stdout.String(), stderr.String())
	}
	if err != nil || !strings.Contains(stdout.String(), "(from the environment)") || !strings.Contains(stdout.String(), "repaired run-state") {
		t.Fatalf("a hung engine: %v, out %q, stderr %q", err, stdout.String(), stderr.String())
	}
	if b, _ := os.ReadFile(h.fdLog + ".timeout"); strings.TrimSpace(string(b)) != "60" {
		t.Errorf("the ask was bounded by %q, want 60", b)
	}
	if exec.Command("flock", "-n", h.lock(), "true").Run() != nil {
		t.Error("the run-state lock is still held after repair.sh ended")
	}
}
