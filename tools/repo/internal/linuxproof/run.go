// SPDX-License-Identifier: 0BSD

package linuxproof

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

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
	Tool      string
}

// Prove runs every job over the commit's tree and files the proof.
//
// Exit codes: 0 proved, 1 a job ran and failed, 2 nothing could be run or
// recorded. ⚠ A job that exits 2 could not run either. wsl-toolkit answers 2
// for a base it cannot use or an image it cannot pull. check-go.sh, ShellCheck
// and `repo mutate` answer 2 for a check they could not run. TOOL-33.
func Prove(req Request, out, errOut io.Writer) int {
	tool := req.Tool
	native := Native(runtime.GOOS, tool)
	if native {
		for _, n := range nativeNeeds {
			if _, err := exec.LookPath(n); err != nil {
				fmt.Fprintf(errOut, "linux-proof: this Linux host proves natively and has no %s on PATH. Nothing was run.\n", n)
				return 2
			}
		}
	} else if tool == "" {
		var err error
		if tool, err = exec.LookPath("wsl-toolkit"); err != nil {
			fmt.Fprintln(errOut, "linux-proof: no wsl-toolkit on PATH, and the jobs run through it. Pass --wsl-toolkit PATH. Nothing was run.")
			return 2
		}
	}
	// ⚠ THE WORKSPACE IS UNDER .tmp, which git ignores, and it holds the
	// archive alone, so the job's copy carries nothing else of the checkout.
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
	jobs := jobsFor(req.Rows)
	var names []string
	for _, j := range jobs {
		var cmd *exec.Cmd
		if native {
			fmt.Fprintf(out, "linux-proof: %s on this host, tree %s\n", j.Name, req.Tree)
			cmd = exec.Command("sh", "-c", Script(j, tarPath, filepath.Join(ws, "src-"+j.Name), req.Tree, false))
			cmd.Dir = ws
		} else {
			fmt.Fprintf(out, "linux-proof: %s in %s, tree %s\n", j.Name, j.Image, req.Tree)
			cmd = exec.Command(tool, RunArgs(j, ws, req.Tree)...)
			cmd.Dir = req.Repo.Root
		}
		cmd.Env = JobEnv(os.Environ())
		cmd.Stdout, cmd.Stderr = out, errOut
		if err := cmd.Run(); err != nil {
			ee, ok := err.(*exec.ExitError)
			if !ok {
				fmt.Fprintf(errOut, "linux-proof: the %s job could not be started: %v. No proof was written for tree %s\n", j.Name, err, req.Tree)
				return 2
			}
			fmt.Fprintf(errOut, "linux-proof: the %s job exited %d, so no proof was written for tree %s\n", j.Name, ee.ExitCode(), req.Tree)
			// ⚠ 2 IS "COULD NOT RUN", NOT A FAILED CHECK. A broken base is
			// not a red job, and the hook's exit code keeps the difference.
			if ee.ExitCode() == 2 {
				return 2
			}
			return 1
		}
		names = append(names, j.Name)
	}
	path, err := Write(req.Common, Proof{Tree: req.Tree, Commit: req.Commit, Jobs: names, RowsSince: req.RowsSince, Rows: req.Rows})
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: every job passed and the proof could not be written: %v\n", err)
		return 2
	}
	fmt.Fprintf(out, "linux-proof: proved tree %s (commit %s): %s\n", req.Tree, req.Commit, path)
	return 0
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
	return Prove(Request{Repo: repo, Common: common, Commit: commit, Tree: tree, Rows: rows, RowsSince: since, Tool: opts.WSLToolkit}, out, errOut)
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
	for _, n := range needs {
		// ⚠ Two refs on one tree are proved once: the first run's proof
		// covers the second when it ran the second's rows.
		if p, ok := g.Proof(n.Tree); ok && len(Unrun(p.Rows, n.Rows)) == 0 {
			continue
		}
		fmt.Fprintf(errOut, "pre-push: %s carries tree %s, which has no Linux proof with its %d added row(s). Running ShellCheck, the go job and the added rows now.\n", n.Ref, n.Tree, len(n.Rows))
		code := Prove(Request{Repo: repo, Common: common, Commit: n.Commit, Tree: n.Tree, Rows: n.Rows, RowsSince: n.Base, Tool: tool}, out, errOut)
		if code != 0 {
			fmt.Fprintf(errOut, "pre-push: %s is refused, because its tree was not proved on Linux. Nothing was pushed.\n", n.Ref)
			fmt.Fprintln(errOut, "  Fix what failed, commit, and push again, or prove it first with:")
			fmt.Fprintln(errOut, "  "+ProveCommand)
			return code
		}
	}
	return 0
}
