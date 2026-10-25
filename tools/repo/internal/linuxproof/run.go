// SPDX-License-Identifier: 0BSD

package linuxproof

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azathothas/ToolKit/tools/repo/internal/gitrepo"
)

// TableRel is the mutation table, relative to the repository root.
const TableRel = "tools/repo/mutations.json"

// ProveCommand is what a message tells the reader to run ahead of a push.
const ProveCommand = "pwsh -NoProfile -File scripts/common/repo.ps1 linux-proof (or sh scripts/common/repo.sh linux-proof)"

// Options is what `repo linux-proof` is handed.
type Options struct {
	// Rev is the commit whose tree is proved. Empty is HEAD.
	Rev string
	// RowsSince counts the added rows from this ref. Empty counts them from
	// the newest commit Remote is known to hold, as the hook does.
	RowsSince string
	Remote    string
	// WSLToolkit is the executable the jobs run through. Empty finds
	// wsl-toolkit on PATH, or on a Linux host runs them natively.
	WSLToolkit string
}

// CommonDir is git's common directory for this checkout, absolute.
func CommonDir(repo *gitrepo.Repo) (string, error) {
	out, err := repo.Git("rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	dir := strings.TrimSpace(out)
	if dir == "" {
		return "", fmt.Errorf("git named no common directory for %s", repo.Root)
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repo.Root, dir)
	}
	return filepath.Clean(dir), nil
}

func treeOf(repo *gitrepo.Repo, rev string) (string, error) {
	out, err := repo.Git("rev-parse", "--verify", "--quiet", rev+"^{tree}")
	if err != nil {
		return "", fmt.Errorf("git cannot name the tree of %s", rev)
	}
	return strings.TrimSpace(out), nil
}

// rowsBetween reads the rows added to the mutation table between base and rev.
func rowsBetween(repo *gitrepo.Repo, base, rev string) ([]string, error) {
	if _, err := repo.Git("rev-parse", "--verify", "--quiet", base+"^{commit}"); err != nil {
		return nil, fmt.Errorf("%s names no commit here", base)
	}
	// ⚠ A TABLE ABSENT AT base IS AN EMPTY ONE, so every row at rev is added.
	before, err := repo.Git("show", base+":"+TableRel)
	if err != nil {
		before = ""
	}
	// ⚠ AND ONE ABSENT AT rev ADDS NOTHING: a repository with no table has no
	// row for the hook to ask for.
	after, err := repo.Git("show", rev+":"+TableRel)
	if err != nil {
		return nil, nil
	}
	return AddedRows([]byte(before), []byte(after))
}

// Native reports whether the jobs run on this host rather than in containers.
//
// ⭐ A LINUX HOST IS ALREADY WHAT CI RUNS ON, and it has no wsl-toolkit, so
// it runs the jobs' checks directly over the unpacked tree. A Windows host
// always runs them through wsl-toolkit.
func Native(goos, tool string) bool {
	return tool == "" && goos != "windows"
}

// Settings are a checkout's own choices for its proofs, from git config.
//
// ⛔ THE ZERO VALUE IS THE SHAPE THAT WAS TIMED, and a checkout with nothing
// set gets it. That is one container at a time, with at most two rows at once
// in it. Parallel Go builds in containers once took a WSL VM to 19.5 GB, and
// three containers at once, each compiling, is that load. No run of it has
// been timed under a memory cap.
type Settings struct {
	// MutateJobs is linux-proof.mutate-jobs: how many rows run at once. Zero
	// or unset is DefaultMutateJobs.
	MutateJobs int
	// Parallel is linux-proof.parallel: the jobs run side by side, a container
	// each at once. ⚠ Unset, they run one after another and stop at the first
	// that does not pass.
	Parallel bool
}

// Setting keys, as `git config` names them.
const (
	MutateJobsKey = "linux-proof.mutate-jobs"
	ParallelKey   = "linux-proof.parallel"
)

// ReadSettings reads the settings through config, which answers a key's raw
// value and false when it is unset.
//
// ⛔ A VALUE THAT DOES NOT READ IS REFUSED, NOT TAKEN AS UNSET. Taken as
// unset, a setting spelt wrong would be dropped without a word, and the run
// would not be the one asked for.
//
// ⚠ A mutate-jobs of 0 IS REFUSED TOO. `repo mutate --jobs 0` is half the
// processors a job sees, while zero here is the capped default. A number says
// what it means.
func ReadSettings(config func(key string) (string, bool)) (Settings, error) {
	var s Settings
	if v, ok := config(MutateJobsKey); ok {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 1 {
			return s, fmt.Errorf("%s is %q, and it takes a whole number of rows, 1 or more", MutateJobsKey, v)
		}
		s.MutateJobs = n
	}
	if v, ok := config(ParallelKey); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "on", "1":
			s.Parallel = true
		case "false", "no", "off", "0":
		default:
			return s, fmt.Errorf("%s is %q, and it takes true or false", ParallelKey, v)
		}
	}
	return s, nil
}

// configOf reads one key of repo's git configuration.
func configOf(repo *gitrepo.Repo) func(string) (string, bool) {
	return func(key string) (string, bool) {
		out, err := repo.Git("config", "--get", key)
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(out), true
	}
}

// Runner is how one command runs its jobs. It is settled once and handed to
// every Prove the command makes, so what it says about the tool is said once.
type Runner struct {
	// Native runs the jobs on this host rather than in containers.
	Native bool
	// Tool is the wsl-toolkit the containers run through.
	Tool string
	// Caches is whether Tool's run takes --cache.
	Caches   bool
	Settings Settings
}

// HasCacheFlag reads `wsl-toolkit run --help` for the --cache flag.
//
// ⚠ ASKED OF THE TOOL, NOT OF A VERSION NUMBER. Release 7.0.2 has no --cache,
// the next one has, and a build from this tree may have it under any version.
func HasCacheFlag(help string) bool {
	return cacheFlag.MatchString(help)
}

var cacheFlag = regexp.MustCompile(`(?m)^\s*--?cache(\s|$)`)

// NewRunner finds the tool the jobs run through and asks it what it can do.
// It answers 2 when nothing could be run.
//
// ⚠ A TOOL WITH NO --cache IS SAID ONCE, and the jobs then run as they did
// before: every Go job starts with an empty build and module cache.
func NewRunner(tool string, s Settings, out, errOut io.Writer) (Runner, int) {
	r := Runner{Tool: tool, Settings: s, Native: Native(runtime.GOOS, tool)}
	if r.Native {
		for _, n := range nativeNeeds {
			if _, err := exec.LookPath(n); err != nil {
				fmt.Fprintf(errOut, "linux-proof: this Linux host proves natively and has no %s on PATH. Nothing was run.\n", n)
				return r, 2
			}
		}
	} else if r.Tool == "" {
		var err error
		if r.Tool, err = exec.LookPath("wsl-toolkit"); err != nil {
			fmt.Fprintln(errOut, "linux-proof: no wsl-toolkit on PATH, and the jobs run through it. Pass --wsl-toolkit PATH. Nothing was run.")
			return r, 2
		}
	}
	if !r.Native {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		help, _ := exec.CommandContext(ctx, r.Tool, "run", "--help").CombinedOutput()
		cancel()
		r.Caches = HasCacheFlag(string(help))
		if !r.Caches {
			fmt.Fprintf(out, "linux-proof: %s has no run --cache, so each Go job starts with an empty build and module cache.\n", r.Tool)
		}
	}
	if s.Parallel {
		fmt.Fprintf(out, "linux-proof: %s is set, so the jobs run side by side, a container each at once.\n", ParallelKey)
	}
	return r, 0
}

// nativeNeeds is what a native proof runs, each checked before anything runs.
// jobsFor is Jobs, and a case swaps it for a check that runs anywhere.
var (
	nativeNeeds = []string{"sh", "tar", "git", "go", "shellcheck"}
	jobsFor     = Jobs
)

// gitRepoVars are git's variables that name a repository, its index or its
// configuration. `git rev-parse --local-env-vars` lists the same set.
var gitRepoVars = []string{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_CONFIG", "GIT_CONFIG_COUNT",
	"GIT_CONFIG_PARAMETERS", "GIT_DIR", "GIT_GRAFT_FILE", "GIT_IMPLICIT_WORK_TREE",
	"GIT_INDEX_FILE", "GIT_INTERNAL_SUPER_PREFIX", "GIT_NO_REPLACE_OBJECTS",
	"GIT_OBJECT_DIRECTORY", "GIT_PREFIX", "GIT_REPLACE_REF_BASE", "GIT_SHALLOW_FILE",
	"GIT_WORK_TREE",
}

// JobEnv is env without git's repository variables. TOOL-33.
//
// ⛔ git EXPORTS GIT_DIR TO A HOOK pushed from a linked worktree, naming that
// worktree's repository. A native job that inherits it runs `git init`, `git
// add` and `git commit` on the checkout being pushed, not on its copy. The same
// tree then fails with "nothing to commit", and another tree is committed onto
// the checked-out branch. Every git call in check-go.sh and `repo mutate`
// would inherit it too. A container job gets no host variable, and the scrub
// costs nothing there.
func JobEnv(env []string) []string {
	var out []string
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		name = strings.ToUpper(name)
		if slices.Contains(gitRepoVars, name) || strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Request is one tree to prove.
type Request struct {
	Repo   *gitrepo.Repo
	Common string
	Commit string
	Tree   string
	Rows   []string
	// RowsSince is recorded in the proof, to say what Rows were counted from.
	RowsSince string
	Runner    Runner
}

// jobEnd is how one job ended. code is -1 for a job that could not be
// started, and ran is false for one a serial run never reached.
type jobEnd struct {
	ran  bool
	code int
	err  error
	took time.Duration
}

func (e jobEnd) passed() bool { return e.ran && e.err == nil && e.code == 0 }

// Prove runs every job over the commit's tree and files the proof.
//
// ⛔ THE JOBS RUN ONE AFTER ANOTHER UNLESS linux-proof.parallel IS SET. One
// after another is the shape that was timed. A run stops at the first job that
// does not pass, and names each job it never reached. Side by side, the push
// waits for the longest job instead of the sum. ⚠ That is not measured, and
// three containers compile at once. With named caches, the go and rows jobs
// also share both cache volumes.
//
// ⭐ Each line a job prints carries its name, so the outputs stay apart.
//
// ⛔ EVERY JOB THAT RAN IS WAITED FOR AND ITS EXIT IS READ. The proof is filed
// only when all of them passed. The first job in Jobs' order that did not pass
// decides the exit code, which is the code a run one after another answers.
//
// Exit codes: 0 proved, 1 a job ran and failed, 2 nothing could be run or
// recorded. ⚠ A job that exits 2 could not run either. wsl-toolkit answers 2
// for a base it cannot use or an image it cannot pull. check-go.sh, ShellCheck
// and `repo mutate` answer 2 for a check they could not run. TOOL-33.
func Prove(req Request, out, errOut io.Writer) int {
	run := req.Runner
	native := run.Native
	tool := run.Tool
	// ⛔ A LABEL IS ONE LINE OF THE LABELS FILE, so a label holding a line
	// break would reach mutate as two labels, neither of them the row.
	for _, label := range req.Rows {
		if strings.ContainsAny(label, "\r\n") {
			fmt.Fprintf(errOut, "linux-proof: the row %q holds a line break, and --labels-file reads one label per line. Nothing was run.\n", label)
			return 2
		}
	}
	// ⚠ THE WORKSPACE IS UNDER .tmp, which git ignores, and it holds the
	// archive and the rows' labels alone, so the job's copy carries nothing
	// else of the checkout.
	parent := filepath.Join(req.Repo.Root, ".tmp")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		fmt.Fprintf(errOut, "linux-proof: %v\n", err)
		return 2
	}
	ws, err := os.MkdirTemp(parent, "linux-proof-")
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: %v\n", err)
		return 2
	}
	defer os.RemoveAll(ws)
	tarPath := filepath.Join(ws, TarName)
	if _, err := req.Repo.Git("archive", "--format=tar", "-o", tarPath, req.Commit); err != nil {
		fmt.Fprintf(errOut, "linux-proof: git archive of %s: %v. Nothing was run.\n", req.Commit, err)
		return 2
	}
	plan := Plan{Rows: req.Rows, Labels: ContainerLabels, MutateJobs: run.Settings.MutateJobs}
	if native {
		plan.Labels = filepath.Join(ws, LabelsName)
	}
	if len(req.Rows) > 0 {
		if err := os.WriteFile(filepath.Join(ws, LabelsName), []byte(strings.Join(req.Rows, "\n")+"\n"), 0o644); err != nil {
			fmt.Fprintf(errOut, "linux-proof: %v. Nothing was run.\n", err)
			return 2
		}
	}
	jobs := jobsFor(plan)
	width := 0
	for _, j := range jobs {
		width = max(width, len(j.Name))
	}
	// ⚠ ONE LOCK FOR BOTH STREAMS, because a caller may hand the same writer
	// as out and errOut.
	var mu sync.Mutex
	say := func(w io.Writer, format string, a ...any) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(w, format, a...)
	}
	ends := make([]jobEnd, len(jobs))
	runJob := func(i int) {
		j := jobs[i]
		var cmd *exec.Cmd
		if native {
			say(out, "linux-proof: %s on this host, tree %s\n", j.Name, req.Tree)
			cmd = exec.Command("sh", "-c", Script(j, tarPath, filepath.Join(ws, "src-"+j.Name), req.Tree, false))
			cmd.Dir = ws
		} else {
			say(out, "linux-proof: %s in %s, tree %s\n", j.Name, j.Image, req.Tree)
			cmd = exec.Command(tool, RunArgs(j, ws, req.Tree, run.Caches)...)
			cmd.Dir = req.Repo.Root
		}
		cmd.Env = JobEnv(os.Environ())
		prefix := fmt.Sprintf("%-*s | ", width, j.Name)
		stdout := &lineWriter{mu: &mu, w: out, prefix: prefix}
		stderr := &lineWriter{mu: &mu, w: errOut, prefix: prefix}
		cmd.Stdout, cmd.Stderr = stdout, stderr
		began := time.Now()
		err := cmd.Run()
		stdout.Close()
		stderr.Close()
		end := jobEnd{ran: true, took: time.Since(began).Round(time.Second)}
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				end.code = ee.ExitCode()
			} else {
				end.code, end.err = -1, err
			}
		}
		ends[i] = end
		switch {
		case end.err != nil:
			say(errOut, "linux-proof: the %s job could not be started: %v. No proof was written for tree %s\n", j.Name, end.err, req.Tree)
		case end.code != 0:
			say(errOut, "linux-proof: the %s job exited %d after %s, so no proof was written for tree %s\n", j.Name, end.code, end.took, req.Tree)
		default:
			say(out, "linux-proof: the %s job passed in %s\n", j.Name, end.took)
		}
	}
	if run.Settings.Parallel {
		var wg sync.WaitGroup
		for i := range jobs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				runJob(i)
			}()
		}
		wg.Wait()
	} else {
		for i := range jobs {
			runJob(i)
			if !ends[i].passed() {
				break
			}
		}
	}
	var names []string
	for i, j := range jobs {
		if !ends[i].ran {
			say(errOut, "linux-proof: the %s job was not run, because a job before it did not pass\n", j.Name)
			continue
		}
		if ends[i].passed() {
			names = append(names, j.Name)
		}
	}
	if code := exitOf(ends); code != 0 {
		return code
	}
	path, err := Write(req.Common, Proof{Tree: req.Tree, Commit: req.Commit, Jobs: names, RowsSince: req.RowsSince, Rows: req.Rows})
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: every job passed and the proof could not be written: %v\n", err)
		return 2
	}
	fmt.Fprintf(out, "linux-proof: proved tree %s (commit %s): %s\n", req.Tree, req.Commit, path)
	return 0
}

// exitOf is the exit code of a proof whose jobs ended so: 0 when every one
// passed, and otherwise the code of the first in order that did not.
//
// ⚠ 2 IS "COULD NOT RUN", NOT A FAILED CHECK. A broken base is not a red job,
// and the hook's exit code keeps the difference. A job that could not be
// started could not run either.
func exitOf(ends []jobEnd) int {
	for _, e := range ends {
		if e.passed() {
			continue
		}
		if e.err != nil || e.code == 2 {
			return 2
		}
		return 1
	}
	return 0
}

// lineWriter hands each whole line one job writes to w with the job's name in
// front, under a lock every job's writers share.
//
// ⛔ A LINE IS WRITTEN WHOLE OR NOT YET. A job's output arrives in pieces of
// any size, and a piece written as it came would split another job's line.
type lineWriter struct {
	mu     *sync.Mutex
	w      io.Writer
	prefix string
	buf    []byte
}

// maxLine is where a line that never ends is written anyway, so a job that
// prints no newline cannot hold its output back without bound.
const maxLine = 64 << 10

func (l *lineWriter) Write(p []byte) (int, error) {
	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 && len(l.buf) < maxLine {
			return len(p), nil
		}
		if i < 0 {
			i = len(l.buf) - 1
		}
		l.emit(l.buf[:i+1])
		l.buf = l.buf[i+1:]
	}
}

// Close writes a last line that ended with no newline.
func (l *lineWriter) Close() {
	if len(l.buf) > 0 {
		l.emit(l.buf)
		l.buf = nil
	}
}

// emit writes one line, and ends it when the job did not.
func (l *lineWriter) emit(line []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	io.WriteString(l.w, l.prefix)
	l.w.Write(line)
	if line[len(line)-1] != '\n' {
		io.WriteString(l.w, "\n")
	}
}

// runnerFor reads the checkout's settings and settles how its jobs run.
func runnerFor(repo *gitrepo.Repo, tool string, out, errOut io.Writer) (Runner, int) {
	s, err := ReadSettings(configOf(repo))
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: %v. Nothing was run.\n", err)
		return Runner{}, 2
	}
	return NewRunner(tool, s, out, errOut)
}

// Run is `repo linux-proof`.
//
// ⭐ AN UNCOMMITTED CHANGE IS NOT AN ERROR, because the jobs check the
// commit's tree from `git archive` and never the working tree. It is named, so
// nobody reads the proof as covering it.
func Run(opts Options, out, errOut io.Writer) int {
	repo, err := gitrepo.Open()
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: %v\n", err)
		return 2
	}
	rev := opts.Rev
	if rev == "" {
		rev = "HEAD"
	}
	commit, err := repo.Git("rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: %s names no commit here\n", rev)
		return 2
	}
	commit = strings.TrimSpace(commit)
	tree, err := treeOf(repo, commit)
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: %v\n", err)
		return 2
	}
	common, err := CommonDir(repo)
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: %v\n", err)
		return 2
	}
	if dirty, err := repo.Dirty(); err == nil && dirty {
		fmt.Fprintf(out, "linux-proof: the working tree has uncommitted changes, and this proves commit %s alone.\n", commit)
	}
	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}
	since := opts.RowsSince
	if since == "" {
		since = gitRepo{repo: repo, common: common}.Base(commit, remote)
	}
	var rows []string
	if since != "" {
		rows, err = rowsBetween(repo, since, commit)
		if err != nil {
			fmt.Fprintf(errOut, "linux-proof: %v\n", err)
			return 2
		}
		fmt.Fprintf(out, "linux-proof: %d mutation row(s) added since %s\n", len(rows), since)
	}
	runner, code := runnerFor(repo, opts.WSLToolkit, out, errOut)
	if code != 0 {
		return code
	}
	return Prove(Request{Repo: repo, Common: common, Commit: commit, Tree: tree, Rows: rows, RowsSince: since, Runner: runner}, out, errOut)
}

// gitRepo answers Repo from a real checkout.
type gitRepo struct {
	repo   *gitrepo.Repo
	common string
}

func (g gitRepo) Tree(rev string) (string, bool) {
	t, err := treeOf(g.repo, rev)
	return t, err == nil && ValidObject(t)
}

// tracked reports whether remote is a name whose refs this checkout tracks.
// ⚠ A remote given as a URL or a path tracks nothing.
func tracked(remote string) bool {
	return remote != "" && !strings.ContainsAny(remote, `/\:`)
}

// OnRemote asks for any commit of rev that no ref tracked for remote reaches.
// ⚠ A remote given as a URL tracks nothing, so the answer is false and the
// proof decides.
func (g gitRepo) OnRemote(rev, remote string) bool {
	if !tracked(remote) {
		return false
	}
	out, err := g.repo.Git("rev-list", "-n", "1", rev, "--not", "--remotes="+remote)
	return err == nil && strings.TrimSpace(out) == ""
}

// Base is the newest boundary commit between rev and the refs tracked for
// remote: the last commit of rev's history the remote is known to hold.
func (g gitRepo) Base(rev, remote string) string {
	if !tracked(remote) {
		return ""
	}
	out, err := g.repo.Git("rev-list", "--boundary", rev, "--not", "--remotes="+remote)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if b, ok := strings.CutPrefix(strings.TrimSpace(line), "-"); ok && ValidObject(b) {
			return b
		}
	}
	return ""
}

func (g gitRepo) AddedRows(base, rev string) ([]string, error) {
	return rowsBetween(g.repo, base, rev)
}

func (g gitRepo) Proof(tree string) (Proof, bool) { return Load(g.common, tree) }

// PrePush is `repo pre-push REMOTE URL`, with git's update lines on stdin.
//
// ⛔ A PUSHED TREE WITH NO PROOF IS PROVED HERE, by running the jobs, and the
// push waits for them. The proof a run files makes the next push of the same
// tree immediate. tool is the wsl-toolkit to run them through, empty to find it.
//
// Exit codes: 0 every ref may go, 1 one is refused or its jobs failed, 2 the
// hook could not decide or could not run them. ⛔ git refuses the push on
// either non-zero answer, so a hook that cannot read its input fails closed.
func PrePush(args []string, in io.Reader, out, errOut io.Writer, tool string) int {
	if len(args) < 1 {
		fmt.Fprintln(errOut, "pre-push: git passes the remote's name as the first argument and did not")
		return 2
	}
	repo, err := gitrepo.Open()
	if err != nil {
		fmt.Fprintf(errOut, "pre-push: %v\n", err)
		return 2
	}
	common, err := CommonDir(repo)
	if err != nil {
		fmt.Fprintf(errOut, "pre-push: %v\n", err)
		return 2
	}
	updates, err := ParseUpdates(in)
	if err != nil {
		fmt.Fprintf(errOut, "pre-push: %v\n", err)
		return 2
	}
	g := gitRepo{repo: repo, common: common}
	needs, refusals := Decide(updates, args[0], g)
	if len(refusals) > 0 {
		for _, r := range refusals {
			fmt.Fprintf(errOut, "pre-push: %s is refused: %s.\n", r.Ref, r.Reason)
		}
		fmt.Fprintln(errOut, "Nothing was pushed.")
		return 1
	}
	// ⚠ THE RUNNER IS SETTLED ON THE FIRST TREE THAT NEEDS A RUN, so a push
	// that needs none never looks for wsl-toolkit, and two trees hear about it
	// once.
	var runner *Runner
	for _, n := range needs {
		// ⚠ Two refs on one tree are proved once: the first run's proof
		// covers the second when it ran the second's rows.
		if p, ok := g.Proof(n.Tree); ok && len(Unrun(p.Rows, n.Rows)) == 0 {
			continue
		}
		fmt.Fprintf(errOut, "pre-push: %s carries tree %s, which has no Linux proof with its %d added row(s). Running ShellCheck, the go job and the added rows now.\n", n.Ref, n.Tree, len(n.Rows))
		code := 0
		if runner == nil {
			r, c := runnerFor(repo, tool, out, errOut)
			if code = c; c == 0 {
				runner = &r
			}
		}
		if runner != nil {
			code = Prove(Request{Repo: repo, Common: common, Commit: n.Commit, Tree: n.Tree, Rows: n.Rows, RowsSince: n.Base, Runner: *runner}, out, errOut)
		}
		if code != 0 {
			fmt.Fprintf(errOut, "pre-push: %s is refused, because its tree was not proved on Linux. Nothing was pushed.\n", n.Ref)
			fmt.Fprintln(errOut, "  Fix what failed, commit, and push again, or prove it first with:")
			fmt.Fprintln(errOut, "  "+ProveCommand)
			return code
		}
	}
	return 0
}
