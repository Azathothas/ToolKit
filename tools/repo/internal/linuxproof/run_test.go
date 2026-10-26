// SPDX-License-Identifier: 0BSD

package linuxproof

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeEnv makes this test binary stand in for wsl-toolkit: it appends its
// arguments and the names in the workspace's archive to the file the variable
// names, then does what fakeModeEnv says.
//
// ⚠ The other variables steer one job at a time, because the jobs run side by
// side and a case has to fail one of them alone. fakeFailIf exits 3 and
// fakeCannotIf exits 2 for a job whose arguments hold the text. fakeTogether
// holds each job until that many have started, and fakeCache answers `run
// --help` with the --cache flag.
const (
	fakeEnv       = "LINUXPROOF_FAKE_LOG"
	fakeModeEnv   = "LINUXPROOF_FAKE_MODE"
	fakeFailIf    = "LINUXPROOF_FAKE_FAIL_IF"
	fakeCannotIf  = "LINUXPROOF_FAKE_CANNOT_IF"
	fakeTogether  = "LINUXPROOF_FAKE_TOGETHER"
	fakeCache     = "LINUXPROOF_FAKE_CACHE"
	togetherLimit = 20 * time.Second
)

// TestMain lets the test binary answer as the fake. ⚠ It acts only when the
// variable is set, which no ordinary `go test` run does.
func TestMain(m *testing.M) {
	if log := os.Getenv(fakeEnv); log != "" {
		os.Exit(fake(log, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fake is wsl-toolkit as the cases see it.
func fake(log string, args []string) int {
	if len(args) == 2 && args[0] == "run" && args[1] == "--help" {
		fmt.Println("Usage of run:\n  -c string\n    \tthe shell command to run in the container")
		if os.Getenv(fakeCache) != "" {
			fmt.Println("  -cache value\n    \tNAME:/path: a named cache")
		}
		return 0
	}
	logged := args
	// ⚠ A git repository variable the job inherited is logged with the
	// arguments, so a case can say it reached the job. TOOL-33.
	for _, v := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"} {
		if val, ok := os.LookupEnv(v); ok {
			logged = append(logged, "LEAKED "+v+"="+val)
		}
	}
	appendLine(log, strings.Join(logged, "\x1f")+"\x1e"+strings.Join(archived(args), ","))
	image := argAfter(args, "--image")
	fmt.Printf("fake ran in %s\nno newline after %s", image, image)
	if n, _ := strconv.Atoi(os.Getenv(fakeTogether)); n > 0 && !together(log+".started", n) {
		fmt.Fprintf(os.Stderr, "%s ran alone: %d jobs never ran at once\n", image, n)
		return 5
	}
	joined := strings.Join(args, " ")
	if s := os.Getenv(fakeFailIf); s != "" && strings.Contains(joined, s) {
		return 3
	}
	if s := os.Getenv(fakeCannotIf); s != "" && strings.Contains(joined, s) {
		return 2
	}
	switch os.Getenv(fakeModeEnv) {
	case "fail":
		return 3
	case "cannot":
		// wsl-toolkit's answer for a base it cannot use.
		return 2
	case "cannot-8":
		// wsl-toolkit 8's answer for a base it cannot use: its own code, which
		// WSL_TOOLKIT_TOOL_EXIT moves. WSL-127.
		if code, err := strconv.Atoi(os.Getenv("WSL_TOOLKIT_TOOL_EXIT")); err == nil {
			return code
		}
		return 125
	case "payload-125":
		// A job whose own payload answered 125, as a nested engine that could
		// not run a container does. wsl-toolkit forwards it. WSL-127.
		return 125
	}
	return 0
}

// together marks this job started in dir and waits for n marks.
func together(dir string, n int) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(os.Getpid())), nil, 0o644); err != nil {
		return false
	}
	for deadline := time.Now().Add(togetherLimit); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if entries, err := os.ReadDir(dir); err == nil && len(entries) >= n {
			return true
		}
	}
	return false
}

// appendLine adds one line to path, in one write, so jobs that log at once
// do not split each other's lines.
func appendLine(path, line string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		os.Exit(90)
	}
	f.WriteString(line + "\n")
	f.Close()
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// archived lists the files in the workspace's archive, as the job would see
// them, and the rows file beside it. It names a workspace that holds
// anything else.
func archived(args []string) []string {
	ws := argAfter(args, "--workspace")
	entries, err := os.ReadDir(ws)
	if err != nil {
		return []string{"NO-WORKSPACE"}
	}
	var labels []string
	for _, e := range entries {
		switch e.Name() {
		case TarName:
		case LabelsName:
			body, err := os.ReadFile(filepath.Join(ws, LabelsName))
			if err != nil {
				return []string{"UNREADABLE-LABELS"}
			}
			labels = append(labels, "labels="+strings.ReplaceAll(string(body), "\n", "|"))
		default:
			return []string{"WORKSPACE-NOT-THE-ARCHIVE-ALONE"}
		}
	}
	f, err := os.Open(filepath.Join(ws, TarName))
	if err != nil {
		return []string{"NO-ARCHIVE"}
	}
	defer f.Close()
	var names []string
	r := tar.NewReader(f)
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return []string{"BAD-ARCHIVE"}
		}
		if h.Typeflag == tar.TypeReg {
			names = append(names, h.Name)
		}
	}
	sort.Strings(names)
	return append(names, labels...)
}

type proofCase struct {
	work, common, log, tool string
}

const oneRow = `{"schema":"repo-mutations/1","mutations":[{"label":"old row","find":"x"}]}`

// newProofCase is a committed repository holding a mutation table, with this
// test binary as its wsl-toolkit and the working directory inside it.
func newProofCase(t *testing.T) proofCase {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH, and the proof is keyed by git's tree")
	}
	tool, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	work := filepath.Join(root, "work")
	gitIn(t, root, "init", "-q", work)
	table := filepath.Join(work, filepath.FromSlash(TableRel))
	if err := os.MkdirAll(filepath.Dir(table), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(table, []byte(oneRow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".gitignore"), []byte(".tmp/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "add", "-A")
	gitIn(t, work, "commit", "-q", "-m", "one")
	gitIn(t, work, "tag", "base")
	t.Chdir(work)
	log := filepath.Join(root, "fake.log")
	t.Setenv(fakeEnv, log)
	t.Setenv(fakeModeEnv, "pass")
	return proofCase{work: work, common: gitIn(t, work, "rev-parse", "--absolute-git-dir"), log: log, tool: tool}
}

func (c proofCase) run(t *testing.T, opts Options) (int, string) {
	t.Helper()
	opts.WSLToolkit = c.tool
	var out, errOut bytes.Buffer
	code := Run(opts, &out, &errOut)
	return code, out.String() + errOut.String()
}

// calls is each fake run: its arguments, then the files its archive held.
func (c proofCase) calls(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(c.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(body)), "\n")
}

func (c proofCase) write(t *testing.T, rel, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(c.work, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAProofIsFiledForTheTreeEveryJobPassedOn.
func TestAProofIsFiledForTheTreeEveryJobPassedOn(t *testing.T) {
	c := newProofCase(t)
	tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	code, msg := c.run(t, Options{})
	if code != 0 || !Has(c.common, tree) {
		t.Fatalf("exit %d, proved %v: %s", code, Has(c.common, tree), msg)
	}
	got := c.calls(t)
	goJob := callWith(got, GoImage)
	if len(got) != 2 || callWith(got, ShellCheckImage) == "" || goJob == "" {
		t.Fatalf("the jobs run were %q; want shellcheck and go", got)
	}
	if !strings.Contains(goJob, tree) {
		t.Fatalf("the job does not compare its copy with tree %s: %q", tree, goJob)
	}
}

// callWith is the one fake run whose line holds s, and empty when none does.
func callWith(calls []string, s string) string {
	for _, c := range calls {
		if strings.Contains(c, s) {
			return c
		}
	}
	return ""
}

// TestTheJobsCheckTheCommitNotTheWorkingTree. ⛔ The copy is `git archive` of
// the commit, so an uncommitted file never reaches it, and the run says so.
func TestTheJobsCheckTheCommitNotTheWorkingTree(t *testing.T) {
	c := newProofCase(t)
	c.write(t, "uncommitted", "x\n")
	c.write(t, filepath.ToSlash(TableRel), "edited and not committed\n")
	tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	code, msg := c.run(t, Options{})
	if code != 0 || !Has(c.common, tree) || !strings.Contains(msg, "uncommitted") {
		t.Fatalf("a dirty tree: exit %d, %q; want 0, HEAD's tree proved, and the change named", code, msg)
	}
	for _, call := range c.calls(t) {
		files := call[strings.Index(call, "\x1e")+1:]
		if files != ".gitignore,"+TableRel {
			t.Fatalf("the job's archive held %q; want the committed files alone", files)
		}
	}
}

// TestAFailedJobFilesNoProof. ⛔ Side by side, every job is waited for, so
// each failure is read, and none of them files a proof.
func TestAFailedJobFilesNoProof(t *testing.T) {
	c := newProofCase(t)
	gitIn(t, c.work, "config", ParallelKey, "true")
	t.Setenv(fakeModeEnv, "fail")
	tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	code, msg := c.run(t, Options{})
	if code != 1 || Has(c.common, tree) || !strings.Contains(msg, "the shellcheck job exited 3") || !strings.Contains(msg, "the go job exited 3") {
		t.Fatalf("a failed job: exit %d, proved %v, %q", code, Has(c.common, tree), msg)
	}
	if got := c.calls(t); len(got) != 2 {
		t.Fatalf("the jobs run were %q; want both, each read", got)
	}
}

// TestOneFailedJobAmongPassingOnesFilesNoProof. ⛔ Side by side, the proof
// waits for the last job and needs every one to pass.
func TestOneFailedJobAmongPassingOnesFilesNoProof(t *testing.T) {
	c := newProofCase(t)
	gitIn(t, c.work, "config", ParallelKey, "true")
	t.Setenv(fakeFailIf, GoImage)
	tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	code, msg := c.run(t, Options{})
	if code != 1 || Has(c.common, tree) {
		t.Fatalf("the go job failed beside a passing shellcheck job: exit %d, proved %v, %q", code, Has(c.common, tree), msg)
	}
	if !strings.Contains(msg, "the shellcheck job passed") || !strings.Contains(msg, "the go job exited 3") || len(c.calls(t)) != 2 {
		t.Fatalf("each job's end is not read: %q", msg)
	}
}

// TestTheFirstJobThatDidNotPassDecidesTheExit. ⚠ Side by side, more than one
// job can fail. The first in Jobs' order decides, which is the job a run one
// after another stopped at, so 1 and 2 keep their meaning.
func TestTheFirstJobThatDidNotPassDecidesTheExit(t *testing.T) {
	ok := jobEnd{ran: true}
	failed, cannot := jobEnd{ran: true, code: 3}, jobEnd{ran: true, code: 2}
	unstarted := jobEnd{ran: true, code: -1, err: os.ErrNotExist}
	for _, tc := range []struct {
		ends []jobEnd
		want int
	}{
		{[]jobEnd{ok, ok, ok}, 0},
		{[]jobEnd{ok, failed, cannot}, 1},
		{[]jobEnd{ok, cannot, failed}, 2},
		{[]jobEnd{unstarted, failed}, 2},
		{[]jobEnd{failed, {}}, 1},
		{[]jobEnd{ok, {}}, 1},
	} {
		if got := exitOf(tc.ends); got != tc.want {
			t.Errorf("exitOf(%+v) = %d, want %d", tc.ends, got, tc.want)
		}
	}
	c := newProofCase(t)
	gitIn(t, c.work, "config", ParallelKey, "true")
	t.Setenv(fakeCannotIf, ShellCheckImage)
	t.Setenv(fakeFailIf, GoImage)
	if code, msg := c.run(t, Options{}); code != 2 {
		t.Fatalf("shellcheck could not run and go failed: exit %d, %q; want 2, shellcheck's", code, msg)
	}
}

// TestTheJobsRunSideBySide when linux-proof.parallel is set. It holds each
// fake job until all three have started, so a run that starts one after
// another never passes. The run says which shape it took. ⭐ Each line a job
// prints carries its name, and a last line with no newline is still kept.
func TestTheJobsRunSideBySide(t *testing.T) {
	c := newProofCase(t)
	c.write(t, filepath.ToSlash(TableRel), `{"schema":"repo-mutations/1","mutations":[{"label":"a new row","find":"y"},{"label":"old row","find":"x"}]}`)
	gitIn(t, c.work, "commit", "-q", "-a", "-m", "two")
	gitIn(t, c.work, "config", ParallelKey, "true")
	t.Setenv(fakeTogether, "3")
	code, msg := c.run(t, Options{RowsSince: "base"})
	if code != 0 || len(c.calls(t)) != 3 {
		t.Fatalf("three jobs held until all had started: exit %d, %d run, %q", code, len(c.calls(t)), msg)
	}
	if !strings.Contains(msg, ParallelKey+" is set, so the jobs run side by side") {
		t.Errorf("a run side by side does not say so: %s", msg)
	}
	for name, image := range map[string]string{"shellcheck": ShellCheckImage, "go        ": GoImage} {
		for _, want := range []string{name + " | fake ran in " + image + "\n", name + " | no newline after " + image + "\n"} {
			if !strings.Contains(msg, want) {
				t.Errorf("the output lacks %q: %s", want, msg)
			}
		}
	}
	if !strings.Contains(msg, "mutations  | fake ran in "+GoImage+"\n") {
		t.Errorf("the rows job's lines do not carry its name: %s", msg)
	}
}

// TestEachJobsLinesCarryItsNameAndStayWhole. ⛔ Two jobs write in pieces of
// any size at once, and a piece written as it came would split a line.
func TestEachJobsLinesCarryItsNameAndStayWhole(t *testing.T) {
	var mu sync.Mutex
	var out bytes.Buffer
	a := &lineWriter{mu: &mu, w: &out, prefix: "a | "}
	b := &lineWriter{mu: &mu, w: &out, prefix: "b | "}
	a.Write([]byte("par"))
	b.Write([]byte("whole\n"))
	a.Write([]byte("tial\nlast with no newline"))
	b.Close()
	a.Close()
	if want := "b | whole\na | partial\na | last with no newline\n"; out.String() != want {
		t.Fatalf("the lines were %q; want %q", out.String(), want)
	}
	out.Reset()
	a.Write(bytes.Repeat([]byte("x"), maxLine+5))
	if got := out.String(); got != "a | "+strings.Repeat("x", maxLine+5)+"\n" {
		t.Fatalf("a line that never ends was held back: %d bytes written", len(got))
	}
}

// TestTheJobsRunOneAfterAnotherByDefault. ⛔ With nothing set, one container
// runs at a time, the shape the hook was timed at. The run stops at the first
// job that did not pass and names the job it never reached.
func TestTheJobsRunOneAfterAnotherByDefault(t *testing.T) {
	c := newProofCase(t)
	t.Setenv(fakeModeEnv, "fail")
	tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	code, msg := c.run(t, Options{})
	if code != 1 || Has(c.common, tree) || len(c.calls(t)) != 1 {
		t.Fatalf("a default run whose first job failed: exit %d, %d run, %q; want 1 and one run", code, len(c.calls(t)), msg)
	}
	if !strings.Contains(msg, "the go job was not run, because a job before it did not pass") || strings.Contains(msg, "side by side") {
		t.Fatalf("the job a default run never reached is not named: %q", msg)
	}
}

// TestSettingsAreReadAndABadOneIsRefused. ⛔ A setting spelt wrong is not
// taken as unset, which would drop it without a word. ⚠ A mutate-jobs of 0 is
// refused, because mutate reads 0 as half the processors.
func TestSettingsAreReadAndABadOneIsRefused(t *testing.T) {
	from := func(kv map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := kv[k]; return v, ok }
	}
	if s, err := ReadSettings(from(nil)); err != nil || s != (Settings{}) {
		t.Fatalf("unset settings = %+v, %v; want the zero value", s, err)
	}
	if s, err := ReadSettings(from(map[string]string{MutateJobsKey: "2", ParallelKey: "Yes"})); err != nil || s.MutateJobs != 2 || !s.Parallel {
		t.Fatalf("mutate-jobs 2 and parallel yes = %+v, %v", s, err)
	}
	for _, v := range []string{"on", "true", "1"} {
		if s, err := ReadSettings(from(map[string]string{ParallelKey: v})); err != nil || !s.Parallel {
			t.Errorf("parallel %q = %+v, %v; want true", v, s, err)
		}
	}
	for _, bad := range []map[string]string{{MutateJobsKey: "two"}, {MutateJobsKey: "-1"}, {MutateJobsKey: "0"}, {ParallelKey: "maybe"}} {
		if _, err := ReadSettings(from(bad)); err == nil {
			t.Errorf("%v was taken as a setting", bad)
		}
	}
	c := newProofCase(t)
	gitIn(t, c.work, "config", MutateJobsKey, "two")
	if code, msg := c.run(t, Options{}); code != 2 || len(c.calls(t)) != 0 || !strings.Contains(msg, MutateJobsKey) {
		t.Fatalf("a mutate-jobs that does not read: exit %d, %d run, %q; want 2 and nothing run", code, len(c.calls(t)), msg)
	}
}

// TestTheRowsRunInOneProcessFromExactLabels. ⭐ One `repo mutate` builds the
// tool once and runs the rows side by side, named exactly by a file, so a
// label that is part of another selects one row.
func TestTheRowsRunInOneProcessFromExactLabels(t *testing.T) {
	c := newProofCase(t)
	c.write(t, filepath.ToSlash(TableRel), `{"schema":"repo-mutations/1","mutations":[{"label":"a new row","find":"y"},{"label":"it's new","find":"z"},{"label":"old row","find":"x"}]}`)
	gitIn(t, c.work, "commit", "-q", "-a", "-m", "two")
	code, msg := c.run(t, Options{RowsSince: "base"})
	rows := callWith(c.calls(t), "mutate ")
	if want := fmt.Sprintf("sh scripts/common/repo.sh mutate --jobs %d --labels-file '%s'", DefaultMutateJobs(runtime.NumCPU()), ContainerLabels); code != 0 || !strings.Contains(rows, want) {
		t.Fatalf("exit %d; the rows job was %q; want one process, %s", code, rows, want)
	}
	if !strings.HasSuffix(rows, ",labels=a new row|it's new|") {
		t.Fatalf("the rows job's labels file is not the added labels exactly: %q", rows)
	}
	// ⚠ Three, because the default is one or two, and a setting the job never
	// got would read as the default.
	gitIn(t, c.work, "config", MutateJobsKey, "3")
	if err := os.Remove(filepath.Join(c.common, ProofDirName, gitIn(t, c.work, "rev-parse", "HEAD^{tree}")+".json")); err != nil {
		t.Fatal(err)
	}
	before := len(c.calls(t))
	code, msg = c.run(t, Options{RowsSince: "base"})
	if rows := callWith(c.calls(t)[before:], "mutate "); code != 0 || !strings.Contains(rows, "mutate --jobs 3 --labels-file") {
		t.Fatalf("linux-proof.mutate-jobs 3: exit %d, rows job %q, %q; want mutate --jobs 3", code, rows, msg)
	}
}

// TestALabelWithALineBreakIsRefused before anything runs, because the labels
// file holds one label per line.
func TestALabelWithALineBreakIsRefused(t *testing.T) {
	c := newProofCase(t)
	c.write(t, filepath.ToSlash(TableRel), `{"schema":"repo-mutations/1","mutations":[{"label":"two\nlines","find":"y"},{"label":"old row","find":"x"}]}`)
	gitIn(t, c.work, "commit", "-q", "-a", "-m", "two")
	if code, msg := c.run(t, Options{RowsSince: "base"}); code != 2 || len(c.calls(t)) != 0 || !strings.Contains(msg, "line break") {
		t.Fatalf("a label holding a line break: exit %d, %d run, %q; want 2 and nothing run", code, len(c.calls(t)), msg)
	}
}

// TestNamedGoCachesWhenTheToolHasThem. ⭐ A tool whose run takes --cache keeps
// Go's build and module caches between runs, for the Go jobs alone, and no
// test result is taken from them. ⚠ A tool without it is said once, however
// many trees a push proves.
func TestNamedGoCachesWhenTheToolHasThem(t *testing.T) {
	c := newProofCase(t)
	t.Setenv(fakeCache, "1")
	if code, msg := c.run(t, Options{}); code != 0 || strings.Contains(msg, "has no run --cache") {
		t.Fatalf("a tool with --cache: exit %d, %q", code, msg)
	}
	goJob, sc := callWith(c.calls(t), GoImage), callWith(c.calls(t), ShellCheckImage)
	for _, want := range []string{"--cache\x1flinux-proof-go-build:/var/cache/linux-proof/go-build", "--env\x1fGOCACHE=/var/cache/linux-proof/go-build", "--cache\x1flinux-proof-go-mod:/var/cache/linux-proof/go-mod", "--env\x1fGOMODCACHE=/var/cache/linux-proof/go-mod", "--env\x1fGOFLAGS=-count=1"} {
		if !strings.Contains(goJob, want) {
			t.Errorf("the go job lacks %q: %q", want, goJob)
		}
	}
	if strings.Contains(sc, "--cache") {
		t.Errorf("the shellcheck job, which builds no Go, mounts a cache: %q", sc)
	}

	t.Setenv(fakeCache, "")
	c.write(t, "second", "x\n")
	gitIn(t, c.work, "add", "second")
	gitIn(t, c.work, "commit", "-q", "-m", "another tree")
	before := len(c.calls(t))
	var out bytes.Buffer
	in := "refs/heads/a " + gitIn(t, c.work, "rev-parse", "HEAD") + " refs/heads/a " + zero + "\nrefs/heads/b " + gitIn(t, c.work, "rev-parse", "base") + " refs/heads/b " + zero + "\n"
	if err := os.Remove(filepath.Join(c.common, ProofDirName, gitIn(t, c.work, "rev-parse", "base^{tree}")+".json")); err != nil {
		t.Fatal(err)
	}
	if code := PrePush([]string{"elsewhere"}, strings.NewReader(in), &out, &out, c.tool); code != 0 {
		t.Fatalf("a push of two trees: exit %d, %q", code, out.String())
	}
	if got := c.calls(t)[before:]; len(got) != 4 || strings.Contains(strings.Join(got, "\n"), "--cache") {
		t.Fatalf("a tool without --cache was asked for one, or the trees were not both proved: %q", got)
	}
	if n := strings.Count(out.String(), "has no run --cache"); n != 1 {
		t.Fatalf("a tool without --cache was said %d times over two trees; want once: %q", n, out.String())
	}
	if !HasCacheFlag("  -c string\n  -cache value\n") || HasCacheFlag("  -cache-dir string\n  -gc\n    \tgc --caches --apply removes it\n") {
		t.Fatal("HasCacheFlag does not answer the -cache flag alone")
	}
}

// TestAJobThatCouldNotRunIsExitTwo. ⚠ A job tool that answers 2, as wsl-toolkit
// does for a broken base, or that cannot be started, is "could not run" and
// not a failed check. The push is refused either way. TOOL-33.
func TestAJobThatCouldNotRunIsExitTwo(t *testing.T) {
	c := newProofCase(t)
	tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	t.Setenv(fakeModeEnv, "cannot")
	if code, msg := c.run(t, Options{}); code != 2 || Has(c.common, tree) || !strings.Contains(msg, "exited 2") {
		t.Fatalf("a job tool that could not run: exit %d, proved %v, %q; want 2 and no proof", code, Has(c.common, tree), msg)
	}
	head := gitIn(t, c.work, "rev-parse", "HEAD")
	var out bytes.Buffer
	if code := PrePush([]string{"origin"}, strings.NewReader("refs/heads/main "+head+" refs/heads/main "+zero+"\n"), &out, &out, c.tool); code != 2 || !strings.Contains(out.String(), "Nothing was pushed") {
		t.Fatalf("a push whose job could not run: exit %d, %q; want 2 and refused", code, out.String())
	}
	t.Setenv(fakeModeEnv, "pass")
	c.tool = filepath.Join(c.work, "no-such-wsl-toolkit")
	if code, msg := c.run(t, Options{}); code != 2 || Has(c.common, tree) || !strings.Contains(msg, "could not be started") {
		t.Fatalf("a job tool that cannot start: exit %d, %q; want 2 and no proof", code, msg)
	}
}

// TestAJobWsltoolkit8CouldNotRunIsExitTwo. ⚠ wsl-toolkit 8 answers its tool code
// for a job it could not run, and a value of WSL_TOOLKIT_TOOL_EXIT in the
// caller's shell moves it. The job's own variable is pinned, so the proof still
// reads "could not run" and not a red job. WSL-127.
func TestAJobWsltoolkit8CouldNotRunIsExitTwo(t *testing.T) {
	c := newProofCase(t)
	tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	t.Setenv(fakeModeEnv, "cannot-8")
	t.Setenv("WSL_TOOLKIT_TOOL_EXIT", "7")
	if code, msg := c.run(t, Options{}); code != 2 || Has(c.common, tree) || !strings.Contains(msg, fmt.Sprintf("exited %d", ToolCannot)) {
		t.Fatalf("a wsl-toolkit 8 job that could not run: exit %d, proved %v, %q; want 2 and no proof", code, Has(c.common, tree), msg)
	}
	if got := exitOf([]jobEnd{{ran: true}, {ran: true, code: ToolCannot}}); got != 2 {
		t.Fatalf("a job that answered %d is read as %d", ToolCannot, got)
	}
}

// TestAPayloadsOwn125IsARedJob. ⛔ A job whose payload answered 125, as a nested
// engine that could not run a container does, failed a check. Reading it as
// "could not run" put WSL-127's conflation back in the proof. WSL-127.
func TestAPayloadsOwn125IsARedJob(t *testing.T) {
	c := newProofCase(t)
	tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	t.Setenv(fakeModeEnv, "payload-125")
	if code, msg := c.run(t, Options{}); code != 1 || Has(c.common, tree) || !strings.Contains(msg, "exited 125") {
		t.Fatalf("a job whose payload answered 125: exit %d, proved %v, %q; want 1 and no proof", code, Has(c.common, tree), msg)
	}
}

// TestAJobGetsNoGitRepositoryVariable. ⛔ git exports GIT_DIR to a hook pushed
// from a linked worktree, and a job that inherits it runs its git calls on the
// pushed repository instead of its copy. TOOL-33.
func TestAJobGetsNoGitRepositoryVariable(t *testing.T) {
	c := newProofCase(t)
	tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	t.Setenv("GIT_DIR", c.common)
	t.Setenv("GIT_WORK_TREE", c.work)
	if code, msg := c.run(t, Options{}); code != 0 || !Has(c.common, tree) {
		t.Fatalf("a run with GIT_DIR set: exit %d, %q", code, msg)
	}
	for _, call := range c.calls(t) {
		if strings.Contains(call, "LEAKED ") {
			t.Fatalf("the job inherited git's repository variables: %q", call)
		}
	}
	env := JobEnv([]string{"PATH=/bin", "GIT_DIR=/x", "git_index_file=/y", "GIT_CONFIG_KEY_0=core.hooksPath", "GIT_CONFIG_VALUE_0=/z", "GIT_EXEC_PATH=/e"})
	if strings.Join(env, ",") != "PATH=/bin,GIT_EXEC_PATH=/e" {
		t.Fatalf("JobEnv kept %q; want PATH and GIT_EXEC_PATH alone", env)
	}
}

// TestANativeJobLeavesThePushedRepositoryAlone runs Prove natively from a
// linked worktree with GIT_DIR set, as git sets it for the hook. ⛔ The
// worktree's branch and index stay as they were, and the proof is filed.
func TestANativeJobLeavesThePushedRepositoryAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows host never proves natively")
	}
	for _, tool := range []string{"sh", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s on PATH", tool)
		}
	}
	c := newProofCase(t)
	jobs, needs := jobsFor, nativeNeeds
	t.Cleanup(func() { jobsFor, nativeNeeds = jobs, needs })
	jobsFor = func(Plan) []Job {
		return []Job{
			{Name: "t", Body: "echo checked"},
		}
	}
	nativeNeeds = []string{"sh", "tar", "git"}
	wt := filepath.Join(filepath.Dir(c.work), "wt")
	gitIn(t, c.work, "worktree", "add", "-q", "-b", "side", wt)
	if err := os.WriteFile(filepath.Join(wt, "second"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, wt, "add", "second")
	gitIn(t, wt, "commit", "-q", "-m", "two")
	head := gitIn(t, wt, "rev-parse", "HEAD")
	base := gitIn(t, wt, "rev-parse", "base^{tree}")
	gitDir := gitIn(t, wt, "rev-parse", "--absolute-git-dir")
	t.Chdir(wt)
	t.Setenv("GIT_DIR", gitDir)
	var out bytes.Buffer
	code := Run(Options{Rev: "base"}, &out, &out)
	os.Unsetenv("GIT_DIR")
	if code != 0 || !Has(c.common, base) || !strings.Contains(out.String(), "checked") {
		t.Fatalf("a native proof with GIT_DIR set: exit %d, proved %v, %q", code, Has(c.common, base), out.String())
	}
	if now := gitIn(t, wt, "rev-parse", "HEAD"); now != head {
		t.Fatalf("the native job committed onto the pushed worktree's branch: HEAD %s, was %s", now, head)
	}
	if st := gitIn(t, wt, "status", "--porcelain"); st != "" {
		t.Fatalf("the native job rewrote the pushed worktree's index: %q", st)
	}
}

// TestRowsSinceRunsTheRowsAddedSinceTheRef in a third job.
func TestRowsSinceRunsTheRowsAddedSinceTheRef(t *testing.T) {
	c := newProofCase(t)
	c.write(t, filepath.ToSlash(TableRel), `{"schema":"repo-mutations/1","mutations":[{"label":"a new row","find":"y"},{"label":"old row","find":"x"}]}`)
	gitIn(t, c.work, "commit", "-q", "-a", "-m", "two")
	code, msg := c.run(t, Options{RowsSince: "base"})
	if code != 0 || !strings.Contains(msg, "1 mutation row(s) added since base") {
		t.Fatalf("exit %d, %q", code, msg)
	}
	got := c.calls(t)
	if rows := callWith(got, "mutate "); len(got) != 3 || !strings.HasSuffix(rows, ",labels=a new row|") {
		t.Fatalf("the jobs run were %q; want a third running the new row alone", got)
	}
	if p, _ := Load(c.common, gitIn(t, c.work, "rev-parse", "HEAD^{tree}")); strings.Join(p.Rows, ",") != "a new row" {
		t.Fatalf("the proof does not record the row it ran: %+v", p)
	}
	if code, _ := c.run(t, Options{RowsSince: "no-such-ref"}); code != 2 {
		t.Fatalf("an unknown ref: exit %d, want 2", code)
	}

	// ⭐ With no --rows-since, the rows are counted from the newest commit
	// origin holds, as the hook counts them.
	bare := filepath.Join(filepath.Dir(c.work), "bare.git")
	gitIn(t, c.work, "init", "-q", "--bare", bare)
	gitIn(t, c.work, "remote", "add", "origin", bare)
	gitIn(t, c.work, "push", "-q", "origin", "base:refs/heads/main")
	gitIn(t, c.work, "fetch", "-q", "origin")
	code, msg = c.run(t, Options{})
	if code != 0 || !strings.Contains(msg, "1 mutation row(s) added since "+gitIn(t, c.work, "rev-parse", "base")) {
		t.Fatalf("rows counted from origin: exit %d, %q", code, msg)
	}
}

// TestRevProvesTheNamedCommit rather than HEAD.
func TestRevProvesTheNamedCommit(t *testing.T) {
	c := newProofCase(t)
	first := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	c.write(t, "second", "x\n")
	gitIn(t, c.work, "add", "second")
	gitIn(t, c.work, "commit", "-q", "-m", "two")
	if code, msg := c.run(t, Options{Rev: "base"}); code != 0 || !Has(c.common, first) {
		t.Fatalf("--rev base: exit %d, %q", code, msg)
	}
	if Has(c.common, gitIn(t, c.work, "rev-parse", "HEAD^{tree}")) {
		t.Fatal("--rev base proved HEAD's tree")
	}
	for _, call := range c.calls(t) {
		if strings.Contains(call[strings.Index(call, "\x1e"):], "second") {
			t.Fatalf("the job proving base was handed HEAD's files: %q", call)
		}
	}
}

// TestAProofMadeInAWorktreeServesTheMainCheckout. ⭐ The proof is filed under
// git's COMMON directory, which every worktree of a checkout shares.
func TestAProofMadeInAWorktreeServesTheMainCheckout(t *testing.T) {
	c := newProofCase(t)
	wt := filepath.Join(filepath.Dir(c.work), "wt")
	gitIn(t, c.work, "worktree", "add", "-q", "-b", "side", wt)
	t.Chdir(wt)
	tree := gitIn(t, wt, "rev-parse", "HEAD^{tree}")
	if code, msg := c.run(t, Options{}); code != 0 {
		t.Fatalf("exit %d, %q", code, msg)
	}
	if !Has(c.common, tree) {
		t.Fatalf("the proof made in a worktree is not under the main checkout's %s", c.common)
	}
}

// TestThePushRunsTheJobsForAnUnprovedTree drives the hook's entry point
// against a real repository, a bare remote and the fake wsl-toolkit. ⛔ An
// unproved tree is proved before the push goes, a failure refuses it, and a
// proof that never ran a row the push adds is run again with it.
func TestThePushRunsTheJobsForAnUnprovedTree(t *testing.T) {
	c := newProofCase(t)
	bare := filepath.Join(filepath.Dir(c.work), "bare.git")
	gitIn(t, c.work, "init", "-q", "--bare", bare)
	gitIn(t, c.work, "remote", "add", "origin", bare)
	head := gitIn(t, c.work, "rev-parse", "HEAD")
	tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	push := func(line string) (int, string) {
		var out bytes.Buffer
		code := PrePush([]string{"origin", bare}, strings.NewReader(line+"\n"), &out, &out, c.tool)
		return code, out.String()
	}
	line := "refs/heads/main " + head + " refs/heads/main " + zero

	t.Setenv(fakeModeEnv, "fail")
	if code, msg := push(line); code != 1 || Has(c.common, tree) || !strings.Contains(msg, "Nothing was pushed") {
		t.Fatalf("a failing job: exit %d, %q; want 1 and no proof", code, msg)
	}
	t.Setenv(fakeModeEnv, "pass")
	before := len(c.calls(t))
	// ⚠ Two refs on one tree in one push run the jobs once.
	if code, msg := push(line + "\nrefs/heads/copy " + head + " refs/heads/copy " + zero); code != 0 || !Has(c.common, tree) {
		t.Fatalf("an unproved tree whose jobs pass: exit %d, %q", code, msg)
	}
	if ran := len(c.calls(t)) - before; ran != 2 {
		t.Fatalf("two refs on one tree ran %d jobs; want the two jobs once", ran)
	}
	before = len(c.calls(t))
	if code, msg := push(line); code != 0 || len(c.calls(t)) != before {
		t.Fatalf("a proved tree ran its jobs again: exit %d, %q", code, msg)
	}

	// ⭐ Once main is on the remote, a branch adding a row needs that row run.
	gitIn(t, c.work, "push", "-q", "origin", "HEAD:refs/heads/main")
	gitIn(t, c.work, "fetch", "-q", "origin")
	c.write(t, filepath.ToSlash(TableRel), `{"schema":"repo-mutations/1","mutations":[{"label":"a new row","find":"y"},{"label":"old row","find":"x"}]}`)
	gitIn(t, c.work, "commit", "-q", "-a", "-m", "two")
	two := gitIn(t, c.work, "rev-parse", "HEAD")
	twoTree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	if _, err := Write(c.common, Proof{Tree: twoTree, Commit: two, Jobs: []string{"shellcheck", "go"}}); err != nil {
		t.Fatal(err)
	}
	before = len(c.calls(t))
	if code, msg := push("refs/heads/side " + two + " refs/heads/side " + zero); code != 0 {
		t.Fatalf("a branch adding a row: exit %d, %q", code, msg)
	}
	got := c.calls(t)[before:]
	if len(got) != 3 || !strings.HasSuffix(callWith(got, "mutate "), ",labels=a new row|") {
		t.Fatalf("a proof without the added row was taken as covering it: %q", got)
	}

	// ⭐ A tag on a commit the remote holds needs nothing: this is `repo
	// release --publish` pushing a tag on a pushed main.
	// ⚠ Its tree's proof is removed first, so only the remote's own refs can
	// let it go.
	gitIn(t, c.work, "tag", "-a", "v1", "-m", "v1", head)
	if err := os.Remove(filepath.Join(c.common, ProofDirName, tree+".json")); err != nil {
		t.Fatal(err)
	}
	before = len(c.calls(t))
	if code, msg := push("refs/tags/v1 " + gitIn(t, c.work, "rev-parse", "v1") + " refs/tags/v1 " + zero); code != 0 || len(c.calls(t)) != before {
		t.Fatalf("a tag on a pushed commit: exit %d, %q", code, msg)
	}
	if code := PrePush([]string{"origin"}, strings.NewReader("not a line\n"), &bytes.Buffer{}, &bytes.Buffer{}, c.tool); code != 2 {
		t.Fatalf("unreadable input: exit %d, want 2", code)
	}
}
