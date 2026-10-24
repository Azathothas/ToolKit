// Package linuxproof runs ShellCheck, CI's go job and the added mutation rows
// on Linux before a push, and answers the pre-push hook's one question: was
// this tree proved? ⚠ It is not all of CI: the gate, the doctor probe and every
// existing row run in CI alone. TOOL-33.
//
// ⛔ WHY IT EXISTS. CI runs the Go tests and the mutation table on Linux, and a
// session here runs them on Windows. The container runs that copy CI's Linux
// jobs were a step a session could skip, and three cases reached main green
// here and red on Linux. TOOL-33.
//
// ⭐ THE HOOK RUNS THE JOBS ITSELF when a pushed tree has no proof, so the step
// cannot be skipped by forgetting it. `repo linux-proof` makes the same proof
// ahead of the push, and the hook then only looks it up.
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

// Load reads the proof of tree, and false when it has none.
//
// ⛔ THE FILE'S OWN tree FIELD MUST NAME THE TREE IT IS FILED UNDER. A file
// copied or renamed into place is not a run of this tree, and the name alone
// is the cheapest thing to forge by accident.
func Load(commonDir, tree string) (Proof, bool) {
	path, err := ProofPath(commonDir, tree)
	if err != nil {
		return Proof{}, false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return Proof{}, false
	}
	var p Proof
	if err := json.Unmarshal(body, &p); err != nil {
		return Proof{}, false
	}
	return p, p.Schema == Schema && p.Tree == tree
}

// Has reports whether tree has a proof.
func Has(commonDir, tree string) bool {
	_, ok := Load(commonDir, tree)
	return ok
}

// Unrun is every row in need that the proof's run did not run.
//
// ⛔ A PROOF MADE WITHOUT THE ROWS A PUSH ADDS DOES NOT COVER THEM. CI's
// mutations job runs every row on Linux, and a row whose case passes only on
// Windows is the same defect as a case that does.
func Unrun(ran, need []string) []string {
	have := map[string]bool{}
	for _, r := range ran {
		have[r] = true
	}
	var out []string
	for _, n := range need {
		if !have[n] {
			out = append(out, n)
		}
	}
	return out
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
	// Base is the newest commit under rev that a ref tracked for remote
	// reaches, and empty when there is none.
	Base(rev, remote string) string
	// AddedRows is every mutation row rev holds that base does not.
	AddedRows(base, rev string) ([]string, error)
	// Proof is tree's proof, and false when it has none.
	Proof(tree string) (Proof, bool)
}

// Refusal is one ref the hook refuses, and why.
type Refusal struct {
	Ref    string
	Tree   string
	Reason string
}

// Need is one pushed commit whose tree has to be proved before the push goes.
type Need struct {
	Ref, Commit, Tree string
	// Base is what Rows were counted from, empty when the remote holds nothing
	// of this commit's history here.
	Base string
	Rows []string
}

// Decide is the hook's whole judgement over one push.
//
// ⭐ THREE THINGS NEED NO PROOF. A deleted ref pushes no tree. A commit the
// remote already holds, such as a release tag on a pushed main, adds nothing CI
// has not seen. And a ref whose remote value already carries the same tree, such
// as an amended message, changes no byte CI checks.
//
// ⛔ EVERYTHING ELSE NEEDS A PROOF OF ITS TREE that ran every mutation row the
// push adds, and a tree git cannot name is refused rather than waved through.
// The rows are counted from the remote ref's own value, or for a new ref from
// the newest commit the remote is known to hold. This runs nothing slow.
func Decide(updates []Update, remote string, repo Repo) ([]Need, []Refusal) {
	var needs []Need
	var refusals []Refusal
	for _, u := range updates {
		if Zero(u.LocalSHA) {
			continue
		}
		tree, ok := repo.Tree(u.LocalSHA)
		if !ok {
			refusals = append(refusals, Refusal{Ref: u.RemoteRef, Reason: "git cannot name the tree of " + u.LocalSHA})
			continue
		}
		if repo.OnRemote(u.LocalSHA, remote) {
			continue
		}
		base := ""
		if !Zero(u.RemoteSHA) {
			if was, ok := repo.Tree(u.RemoteSHA); ok {
				if was == tree {
					continue
				}
				base = u.RemoteSHA
			}
		}
		if base == "" {
			base = repo.Base(u.LocalSHA, remote)
		}
		var rows []string
		if base != "" {
			var err error
			rows, err = repo.AddedRows(base, u.LocalSHA)
			if err != nil {
				refusals = append(refusals, Refusal{Ref: u.RemoteRef, Tree: tree, Reason: err.Error()})
				continue
			}
		}
		if p, ok := repo.Proof(tree); ok && len(Unrun(p.Rows, rows)) == 0 {
			continue
		}
		needs = append(needs, Need{Ref: u.RemoteRef, Commit: u.LocalSHA, Tree: tree, Base: base, Rows: rows})
	}
	return needs, refusals
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

// Job is one run of CI's Linux checks.
type Job struct {
	Name  string
	Image string
	// Setup installs what the image lacks. It runs in a container alone: a
	// Linux host that proves natively already has what it needs, or is refused.
	Setup string
	// Body is the check itself, run in the unpacked tree.
	Body string
}

// TarName is the tree's archive in the job's workspace.
const TarName = "tree.tar"

// Script is the sh script one job runs: unpack the tree from tar, make it a
// git repository, prove it IS the tree, then run the check.
//
// ⛔ THE JOB CHECKS THE COMMITTED TREE, NEVER THE WORKING TREE. The archive is
// `git archive` of the pushed commit, so an uncommitted edit cannot reach the
// copy, and the copy is not the checkout, so a worktree's .git file naming a
// Windows path never reaches the container.
//
// ⛔ AND THE COPY'S TREE IS COMPARED WITH THE ONE THE PROOF IS FILED UNDER. A
// line ending converted or a file left out on the way would be a proof of a
// different tree, and it fails here instead. `add -f`, because a tracked file
// a .gitignore pattern also matches is still in the tree.
func Script(j Job, tarPath, src, tree string, container bool) string {
	var b strings.Builder
	if container && j.Setup != "" {
		b.WriteString(j.Setup + " && ")
	}
	b.WriteString("mkdir -p " + ShellQuote(src) + " && tar -xf " + ShellQuote(tarPath) + " -C " + ShellQuote(src) + " && cd " + ShellQuote(src))
	b.WriteString(" && git init -q && git add -A -f")
	b.WriteString(" && git -c user.name=linux-proof -c user.email=linux-proof@invalid -c commit.gpgsign=false -c core.hooksPath=/dev/null commit -q -m linux-proof")
	b.WriteString(` && test "$(git rev-parse 'HEAD^{tree}')" = ` + ShellQuote(tree))
	b.WriteString(" && " + j.Body)
	return b.String()
}

// Jobs is what one proof runs, in order.
//
// ⭐ THE FIRST TWO ARE CI's LINUX JOBS, the ones
// tools/windows/wsl-toolkit/README.md "Build and local proof" used to list as
// commands to paste: the ShellCheck CI's `checks` job installs, and the `go`
// job over every module. ShellCheck goes first because it takes a minute and
// the go job takes many. The third, when rows are named, is the `mutations`
// job restricted to the rows this push adds. ⛔ This is now the one home of
// those command lines; the README describes them and points here.
//
// ⚠ IT IS NOT ALL OF CI. The `checks` job also runs the doctor probe and the
// gate, and the sharded `mutations` job runs every row. Those run in CI alone,
// and the README and docs/conventions/git.md say so.
func Jobs(rows []string) []Job {
	jobs := []Job{
		{Name: "shellcheck", Image: "docker.io/library/ubuntu:24.04", Setup: `apt-get update -qq && apt-get install -y -qq shellcheck git >/dev/null`, Body: `shellcheck --version && git ls-files -z "*.sh" | xargs -0 shellcheck -s sh`},
		{Name: "go", Image: "docker.io/library/golang:1.25", Body: `sh scripts/common/check-go.sh`},
	}
	if len(rows) > 0 {
		var parts []string
		for _, label := range rows {
			parts = append(parts, "sh scripts/common/repo.sh mutate --only "+ShellQuote(label))
		}
		jobs = append(jobs, Job{Name: "mutations", Image: "docker.io/library/golang:1.25", Body: strings.Join(parts, " && ")})
	}
	return jobs
}

// ContainerTar and ContainerSrc are where a container job finds the archive
// and unpacks it. The workspace is copied to /work.
const (
	ContainerTar = "/work/" + TarName
	ContainerSrc = "/src"
)

// RunArgs is the wsl-toolkit command line for j over the workspace ws, which
// holds the tree's archive alone.
//
// ⚠ 45 MINUTES, because the go job runs every module's suite in one
// container, and a job cut short proves nothing.
func RunArgs(j Job, ws, tree string) []string {
	return []string{
		"run", "--image", j.Image, "--container-lifecycle", "ephemeral", "--timeout", "45m",
		"--workspace", ws,
		"-c", Script(j, ContainerTar, ContainerSrc, tree, true),
	}
}
