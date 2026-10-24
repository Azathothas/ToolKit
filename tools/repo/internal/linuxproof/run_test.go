// SPDX-License-Identifier: 0BSD

package linuxproof

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// fakeEnv makes this test binary stand in for wsl-toolkit: it appends its
// arguments and the names in the workspace's archive to the file the variable
// names, then does what fakeModeEnv says.
const (
	fakeEnv     = "LINUXPROOF_FAKE_LOG"
	fakeModeEnv = "LINUXPROOF_FAKE_MODE"
)

// TestMain lets the test binary answer as the fake. ⚠ It acts only when the
// variable is set, which no ordinary `go test` run does.
func TestMain(m *testing.M) {
	if log := os.Getenv(fakeEnv); log != "" {
		f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			os.Exit(90)
		}
		args := os.Args[1:]
		// ⚠ A git repository variable the job inherited is logged with the
		// arguments, so a case can say it reached the job. TOOL-33.
		for _, v := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"} {
			if val, ok := os.LookupEnv(v); ok {
				args = append(args, "LEAKED "+v+"="+val)
			}
		}
		f.WriteString(strings.Join(args, "\x1f") + "\x1e" + strings.Join(archived(os.Args[1:]), ",") + "\n")
		f.Close()
		switch os.Getenv(fakeModeEnv) {
		case "fail":
			os.Exit(3)
		case "cannot":
			// wsl-toolkit's answer for a base it cannot use.
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// archived lists the files in the workspace's archive, as the job would see
// them, and names a workspace that holds anything else.
func archived(args []string) []string {
	ws := ""
	for i, a := range args {
		if a == "--workspace" && i+1 < len(args) {
			ws = args[i+1]
		}
	}
	entries, err := os.ReadDir(ws)
	if err != nil || len(entries) != 1 || entries[0].Name() != TarName {
		return []string{"WORKSPACE-NOT-THE-ARCHIVE-ALONE"}
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
	return names
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
	if len(got) != 2 || !strings.Contains(got[0], "ubuntu:24.04") || !strings.Contains(got[1], "golang:1.25") {
		t.Fatalf("the jobs run were %q; want shellcheck then go", got)
	}
	if !strings.Contains(got[1], tree) {
		t.Fatalf("the job does not compare its copy with tree %s: %q", tree, got[1])
	}
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

// TestAFailedJobFilesNoProof.
func TestAFailedJobFilesNoProof(t *testing.T) {
	c := newProofCase(t)
	t.Setenv(fakeModeEnv, "fail")
	tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	code, msg := c.run(t, Options{})
	if code != 1 || Has(c.common, tree) || !strings.Contains(msg, "exited 3") {
		t.Fatalf("a failed job: exit %d, proved %v, %q", code, Has(c.common, tree), msg)
	}
	if got := c.calls(t); len(got) != 1 {
		t.Fatalf("the jobs after a failed one still ran: %q", got)
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
	jobsFor = func([]string) []Job { return []Job{{Name: "t", Body: "echo checked"}} }
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
	if len(got) != 3 || !strings.Contains(got[2], "mutate --only 'a new row'") || strings.Contains(got[2], "old row") {
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
	if len(got) != 3 || !strings.Contains(got[2], "mutate --only 'a new row'") {
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
