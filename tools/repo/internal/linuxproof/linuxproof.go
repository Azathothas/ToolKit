// Package linuxproof runs CI's Linux checks in containers before a push, and
// answers the pre-push hook's one question: was this tree proved?
//
// ⛔ WHY IT EXISTS. CI runs the Go tests and the mutation table on Linux, and a
// session here runs them on Windows. The container runs that copy CI's Linux
// jobs were a step a session could skip, and three cases reached main green
// here and red on Linux. TOOL-33.
//
// ⭐ THE PROOF IS KEYED BY TREE, NOT BY COMMIT. What CI checks is the content
// of a commit, so an amended message, a rebase that changed nothing or a tag on
// a proved commit carries the same tree and needs no second run. A changed
// byte is a different tree and does.
//
// ⚠ THE PROOF LIVES UNDER git's COMMON DIRECTORY, not in the tree. A file in
// the tree would change the tree it describes, and the common directory is
// the one every worktree of a checkout shares, so a proof made in one worktree
// lets the same tree be pushed from another.
//
// SPDX-License-Identifier: 0BSD
package linuxproof

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Schema is the proof file's shape.
const Schema = "repo-linux-proof/1"

// ProofDirName is the directory under git's common directory proofs are kept in.
const ProofDirName = "linux-proof"

// Proof is what one successful run records.
type Proof struct {
	Schema    string   `json:"schema"`
	Tree      string   `json:"tree"`
	Commit    string   `json:"commit"`
	Jobs      []string `json:"jobs"`
	RowsSince string   `json:"rows_since,omitempty"`
	Rows      []string `json:"rows,omitempty"`
}

// ValidObject accepts a full object name, SHA-1 or SHA-256, lower-case hex.
//
// ⛔ THE KEY IS A FILE NAME, so anything else is refused before it is joined
// to a path. A tree named `../x` would write a proof somewhere a lookup for a
// real tree could never find, or overwrite a file that is not a proof.
func ValidObject(name string) bool {
	if len(name) != 40 && len(name) != 64 {
		return false
	}
	for _, c := range name {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// ProofPath is where the proof of tree lives.
func ProofPath(commonDir, tree string) (string, error) {
	if !ValidObject(tree) {
		return "", fmt.Errorf("%q is not a full tree object name, so it cannot key a proof", tree)
	}
	return filepath.Join(commonDir, ProofDirName, tree+".json"), nil
}

// Write records p for its tree, through a temporary file and a rename, so a
// killed run leaves either no proof or a whole one.
func Write(commonDir string, p Proof) (string, error) {
	path, err := ProofPath(commonDir, p.Tree)
	if err != nil {
		return "", err
	}
	p.Schema = Schema
	body, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp := path + ".partial"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// Has reports whether tree has a proof.
//
// ⛔ THE FILE'S OWN tree FIELD MUST NAME THE TREE IT IS FILED UNDER. A file
// copied or renamed into place is not a run of this tree, and the name alone
// is the cheapest thing to forge by accident.
func Has(commonDir, tree string) bool {
	path, err := ProofPath(commonDir, tree)
	if err != nil {
		return false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var p Proof
	if err := json.Unmarshal(body, &p); err != nil {
		return false
	}
	return p.Schema == Schema && p.Tree == tree
}

// Update is one line git hands a pre-push hook on stdin.
type Update struct {
	LocalRef, LocalSHA, RemoteRef, RemoteSHA string
}

// ParseUpdates reads the hook's stdin: `<local ref> <local sha> <remote ref>
// <remote sha>`, one per ref being pushed.
//
// ⛔ A LINE OF ANY OTHER SHAPE IS REFUSED, not skipped. A hook that skipped
// what it could not read would let that ref through unproved.
func ParseUpdates(r io.Reader) ([]Update, error) {
	var out []Update
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 4 {
			return nil, fmt.Errorf("git handed the hook %q, which is not four fields", line)
		}
		out = append(out, Update{LocalRef: f[0], LocalSHA: f[1], RemoteRef: f[2], RemoteSHA: f[3]})
	}
	return out, sc.Err()
}

// Zero reports git's all-zeros object name: a ref being deleted on the local
// side, or one the remote does not have yet on the remote side.
func Zero(sha string) bool {
	return sha != "" && strings.Trim(sha, "0") == ""
}

// Repo is what the hook asks git. A fake answers it in the cases.
type Repo interface {
	// Tree is rev's tree, and false when the object is not here.
	Tree(rev string) (string, bool)
	// OnRemote reports whether rev is already reachable from a ref this
	// checkout tracks for remote.
	OnRemote(rev, remote string) bool
	// Proved reports whether tree has a proof.
	Proved(tree string) bool
}

// Refusal is one ref the hook refuses, and why.
type Refusal struct {
	Ref    string
	Tree   string
	Reason string
}

// Decide is the hook's whole judgement over one push.
//
// ⭐ THREE THINGS NEED NO PROOF. A deleted ref pushes no tree. A commit the
// remote already holds, such as a release tag on a pushed main, adds nothing CI
// has not seen. And a ref whose remote value already carries the same tree, such
// as an amended message, changes no byte CI checks.
//
// ⛔ EVERYTHING ELSE NEEDS ONE, and a tree git cannot name is refused rather
// than waved through. This runs nothing slow: it reads git and one file per ref.
func Decide(updates []Update, remote string, repo Repo) []Refusal {
	var out []Refusal
	for _, u := range updates {
		if Zero(u.LocalSHA) {
			continue
		}
		tree, ok := repo.Tree(u.LocalSHA)
		if !ok {
			out = append(out, Refusal{Ref: u.RemoteRef, Reason: "git cannot name the tree of " + u.LocalSHA})
			continue
		}
		if repo.OnRemote(u.LocalSHA, remote) {
			continue
		}
		if !Zero(u.RemoteSHA) {
			if was, ok := repo.Tree(u.RemoteSHA); ok && was == tree {
				continue
			}
		}
		if repo.Proved(tree) {
			continue
		}
		out = append(out, Refusal{Ref: u.RemoteRef, Tree: tree, Reason: "no Linux proof is recorded for tree " + tree})
	}
	return out
}

// row is one mutation row, read as JSON and compared whole.
type row = map[string]any

// AddedRows is every row of after that before does not hold byte for byte,
// by label.
//
// ⛔ A CHANGED ROW COUNTS AS ADDED. A row whose find or run was edited is a new
// claim about a guard, and it is proved like one.
func AddedRows(before, after []byte) ([]string, error) {
	parse := func(body []byte) ([]row, error) {
		if len(body) == 0 {
			return nil, nil
		}
		var t struct {
			Mutations []row `json:"mutations"`
		}
		if err := json.Unmarshal(body, &t); err != nil {
			return nil, err
		}
		return t.Mutations, nil
	}
	old, err := parse(before)
	if err != nil {
		return nil, fmt.Errorf("the mutation table before is not readable: %w", err)
	}
	now, err := parse(after)
	if err != nil {
		return nil, fmt.Errorf("the mutation table at HEAD is not readable: %w", err)
	}
	seen := map[string]bool{}
	for _, r := range old {
		b, _ := json.Marshal(r)
		seen[string(b)] = true
	}
	var out []string
	for _, r := range now {
		b, _ := json.Marshal(r)
		if seen[string(b)] {
			continue
		}
		label, _ := r["label"].(string)
		if label == "" {
			return nil, fmt.Errorf("a row added to the mutation table has no label, so it cannot be selected")
		}
		out = append(out, label)
	}
	return out, nil
}

// ShellQuote is s as one POSIX sh word.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Job is one container run.
type Job struct {
	Name   string
	Image  string
	Script string
}

// prelude makes the copy a git repository of exactly the tree being proved.
//
// ⛔ .git IS LEFT OUT OF THE COPY AND A FRESH REPOSITORY IS MADE INSIDE. In a
// worktree .git is a file naming a Windows path the container cannot reach,
// so every git call in the checks failed; and in a main checkout it carried
// the whole object store into a job that reads none of it. The copy holds the
// tracked files of a clean tree with their index modes, which is the tree this
// proof is keyed by, and the checks read only that.
const prelude = `git config --global --add safe.directory "*" && cd /work && git init -q && git add -A && git -c user.name=linux-proof -c user.email=linux-proof@invalid commit -q -m linux-proof`

// Jobs is what one proof runs, in order.
//
// ⭐ THE FIRST TWO ARE CI's LINUX JOBS, the ones
// tools/windows/wsl-toolkit/README.md "Build and local proof" used to list as
// commands to paste: the `go` job on ubuntu over every module, and the
// ShellCheck the `checks` job installs. The third, when rows are named, is the
// `mutations` job restricted to the rows this push adds. ⛔ This is now the one
// home of those command lines; the README describes them and points here.
func Jobs(rows []string) []Job {
	jobs := []Job{
		{Name: "go", Image: "docker.io/library/golang:1.25", Script: prelude + ` && sh scripts/common/check-go.sh`},
		{Name: "shellcheck", Image: "docker.io/library/ubuntu:24.04", Script: `apt-get update -qq && apt-get install -y -qq shellcheck git >/dev/null && ` + prelude + ` && shellcheck --version && git ls-files -z "*.sh" | xargs -0 shellcheck -s sh`},
	}
	if len(rows) > 0 {
		var parts []string
		for _, label := range rows {
			parts = append(parts, "sh scripts/common/repo.sh mutate --only "+ShellQuote(label))
		}
		jobs = append(jobs, Job{Name: "mutations", Image: "docker.io/library/golang:1.25", Script: prelude + " && " + strings.Join(parts, " && ")})
	}
	return jobs
}

// RunArgs is the wsl-toolkit command line for j, from the repository root.
//
// ⚠ --gitignore AND --exclude .claude are the two flags the README's list did
// not carry: the first keeps what git ignores out of the copy, and the second
// keeps out the worktrees a session's harness makes under .claude.
func RunArgs(j Job) []string {
	return []string{
		"run", "--image", j.Image, "--container-lifecycle", "ephemeral", "--timeout", "30m",
		"--workspace", ".", "--gitignore",
		"--exclude", ".git", "--exclude", ".tmp", "--exclude", ".codegraph", "--exclude", ".claude",
		"-c", j.Script,
	}
}
