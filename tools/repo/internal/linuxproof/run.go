// SPDX-License-Identifier: 0BSD

package linuxproof

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Azathothas/ToolKit/tools/repo/internal/gitrepo"
)

// TableRel is the mutation table, relative to the repository root.
const TableRel = "tools/repo/mutations.json"

// ProveCommand is what a refusal tells the reader to run, on either host.
const ProveCommand = "pwsh -NoProfile -File scripts/common/repo.ps1 linux-proof (or sh scripts/common/repo.sh linux-proof)"

// Options is what `repo linux-proof` is handed.
type Options struct {
	RowsSince  string
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

// rowsSince reads the rows added to the mutation table between ref and HEAD.
func rowsSince(repo *gitrepo.Repo, ref string) ([]string, error) {
	if _, err := repo.Git("rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return nil, fmt.Errorf("--rows-since %s names no commit here", ref)
	}
	// ⚠ A TABLE ABSENT AT ref IS AN EMPTY ONE, so every row at HEAD is added.
	before, err := repo.Git("show", ref+":"+TableRel)
	if err != nil {
		before = ""
	}
	after, err := repo.Git("show", "HEAD:"+TableRel)
	if err != nil {
		return nil, fmt.Errorf("HEAD holds no %s: %w", TableRel, err)
	}
	return AddedRows([]byte(before), []byte(after))
}

// Run is `repo linux-proof`.
//
// ⛔ A CLEAN TREE OR NOTHING. The proof is keyed by HEAD's tree, and the copy
// a job runs is the working tree. With an uncommitted change the two differ,
// and a proof of one filed under the other is a proof of nothing.
func Run(opts Options, out, errOut io.Writer) int {
	repo, err := gitrepo.Open()
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: %v\n", err)
		return 2
	}
	if dirty, err := repo.Dirty(); err != nil {
		fmt.Fprintf(errOut, "linux-proof: git status: %v\n", err)
		return 2
	} else if dirty {
		fmt.Fprintln(errOut, "linux-proof: the working tree has uncommitted changes, so what the jobs would check is not HEAD's tree.")
		fmt.Fprintln(errOut, "  Commit first (git-sync --no-push), then run this again. Nothing was run.")
		return 2
	}
	tree, err := treeOf(repo, "HEAD")
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: %v\n", err)
		return 2
	}
	commit, err := repo.Git("rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: %v\n", err)
		return 2
	}
	commit = strings.TrimSpace(commit)
	common, err := CommonDir(repo)
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: %v\n", err)
		return 2
	}
	var rows []string
	if opts.RowsSince != "" {
		rows, err = rowsSince(repo, opts.RowsSince)
		if err != nil {
			fmt.Fprintf(errOut, "linux-proof: %v\n", err)
			return 2
		}
		fmt.Fprintf(out, "linux-proof: %d mutation row(s) added since %s\n", len(rows), opts.RowsSince)
	}
	tool := opts.WSLToolkit
	if tool == "" {
		tool, err = exec.LookPath("wsl-toolkit")
		if err != nil {
			fmt.Fprintln(errOut, "linux-proof: no wsl-toolkit on PATH, and the jobs run through it. Pass --wsl-toolkit PATH. Nothing was run.")
			return 2
		}
	}
	jobs := Jobs(rows)
	var names []string
	for _, j := range jobs {
		fmt.Fprintf(out, "linux-proof: %s in %s, tree %s\n", j.Name, j.Image, tree)
		cmd := exec.Command(tool, RunArgs(j)...)
		cmd.Dir = repo.Root
		cmd.Stdout, cmd.Stderr = out, errOut
		if err := cmd.Run(); err != nil {
			code := 2
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
			fmt.Fprintf(errOut, "linux-proof: the %s job exited %d, so no proof was written for tree %s\n", j.Name, code, tree)
			return 1
		}
		names = append(names, j.Name)
	}
	// ⛔ THE TREE IS READ AGAIN BEFORE THE PROOF IS FILED. The jobs take minutes,
	// and a commit or an edit made meanwhile would file this run's proof under a
	// tree it never copied.
	if dirty, err := repo.Dirty(); err != nil || dirty {
		fmt.Fprintln(errOut, "linux-proof: the working tree changed while the jobs ran, so no proof was written. Run this again.")
		return 2
	}
	if now, err := treeOf(repo, "HEAD"); err != nil || now != tree {
		fmt.Fprintf(errOut, "linux-proof: HEAD moved from tree %s while the jobs ran, so no proof was written. Run this again.\n", tree)
		return 2
	}
	path, err := Write(common, Proof{Tree: tree, Commit: commit, Jobs: names, RowsSince: opts.RowsSince, Rows: rows})
	if err != nil {
		fmt.Fprintf(errOut, "linux-proof: every job passed and the proof could not be written: %v\n", err)
		return 2
	}
	fmt.Fprintf(out, "linux-proof: proved tree %s (commit %s): %s\n", tree, commit, path)
	return 0
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

// OnRemote asks for any commit of rev that no ref tracked for remote reaches.
// ⚠ A remote given as a URL tracks nothing, so the answer is false and the
// proof decides.
func (g gitRepo) OnRemote(rev, remote string) bool {
	if remote == "" || strings.ContainsAny(remote, `/\:`) {
		return false
	}
	out, err := g.repo.Git("rev-list", "-n", "1", rev, "--not", "--remotes="+remote)
	return err == nil && strings.TrimSpace(out) == ""
}

func (g gitRepo) Proved(tree string) bool { return Has(g.common, tree) }

// PrePush is `repo pre-push REMOTE URL`, with git's update lines on stdin.
//
// Exit codes: 0 every ref may go, 1 one is refused, 2 the hook could not
// decide. ⛔ git refuses the push on either non-zero answer, so a hook that
// cannot read its input fails closed.
func PrePush(args []string, in io.Reader, errOut io.Writer) int {
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
	refusals := Decide(updates, args[0], gitRepo{repo: repo, common: common})
	if len(refusals) == 0 {
		return 0
	}
	for _, r := range refusals {
		fmt.Fprintf(errOut, "pre-push: %s is refused: %s.\n", r.Ref, r.Reason)
	}
	fmt.Fprintln(errOut, "Nothing was pushed. CI runs the Go tests and ShellCheck on Linux; prove this tree there first:")
	fmt.Fprintln(errOut, "  "+ProveCommand)
	fmt.Fprintln(errOut, "then push again.")
	return 1
}
