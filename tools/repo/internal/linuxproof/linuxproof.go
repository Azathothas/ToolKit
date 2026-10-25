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
	"runtime"
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
	// Setup checks what the image pins and installs what it lacks. It runs in
	// a container alone: a Linux host that proves natively already has what it
	// needs, or is refused.
	Setup string
	// Body is the check itself, run in the unpacked tree.
	Body string
	// Go marks a job that builds Go, so it mounts the named Go caches when
	// the tool can.
	Go bool
}

// TarName is the tree's archive in the job's workspace.
const TarName = "tree.tar"

// LabelsName is the file beside the archive that names the rows to prove,
// one exact label per line.
const LabelsName = "labels"

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

// ShellCheckImage is ShellCheck 0.9.0 on Alpine, pinned by its tag.
//
// ⛔ THE VERSION IS CI's. The `checks` job installs ShellCheck from
// ubuntu-latest's apt, which is 0.9.0, and another version finds other things.
// The container checks the version it got and answers 2 for any other,
// because that is not the check CI runs. ⚠ A Linux host proving natively runs
// its own ShellCheck, as before, and the job prints which.
//
// ⭐ IT IS PREINSTALLED, so a run no longer fetches apt's indexes and a 30 MB
// package. Alpine has no git, which the job's tree check needs, and `apk add
// git` takes seconds.
const (
	ShellCheckImage   = "docker.io/koalaman/shellcheck-alpine:v0.9.0"
	ShellCheckVersion = "0.9.0"
)

// GoImage is the image the go job and the rows run in, CI's Go.
const GoImage = "docker.io/library/golang:1.25"

// shellcheckVersionCheck answers 2 for a ShellCheck that is not
// ShellCheckVersion.
var shellcheckVersionCheck = `{ shellcheck --version | grep -qx 'version: ` + ShellCheckVersion + `' || { echo 'linux-proof: this ShellCheck is not ` + ShellCheckVersion + `, the version CI runs' >&2; exit 2; }; }`

// Plan is what one proof runs.
type Plan struct {
	// Rows are the mutation rows the push adds. None leaves the rows job out.
	Rows []string
	// Labels is the file the rows job reads Rows from, as the job sees it.
	Labels string
	// MutateJobs is `repo mutate --jobs` in the rows job. ⛔ Zero is
	// DefaultMutateJobs, never mutate's own default.
	MutateJobs int
}

// DefaultMutateJobs is how many rows run at once on a host with cpus
// processors: half of them, at least one and at most two.
//
// ⛔ AT MOST TWO. mutate's own default is half the processors a job sees, and
// a container sees every thread of the host. That is ten workers on a host of
// twenty, each compiling a module's test binary. Two at once is the shape the
// hook was timed at. mutate measured two workers 1.8 times as fast as one on
// four processors, and four slower than three.
func DefaultMutateJobs(cpus int) int {
	return min(2, max(1, cpus/2))
}

// labelsFileProbe answers 0 when the tree's own `repo mutate` reads
// --labels-file.
var labelsFileProbe = `sh scripts/common/repo.sh mutate -h 2>&1 | grep -q -e '-labels-file'`

// olderMutateSays is what the rows job prints when the tree's mutate does not.
var olderMutateSays = `echo 'linux-proof: the repo mutate in this tree takes no --labels-file, so each row runs alone through --only, which matches part of a label' >&2`

// Jobs is what one proof runs, in the order their results are read.
//
// ⭐ THE FIRST TWO ARE CI's LINUX JOBS, the ones
// tools/windows/wsl-toolkit/README.md "Build and local proof" used to list as
// commands to paste: the ShellCheck CI's `checks` job installs, and the `go`
// job over every module. The third, when rows are named, is the `mutations`
// job restricted to the rows this push adds. ⛔ This is now the one home of
// those command lines; the README describes them and points here.
//
// ⭐ THE ROWS ARE ONE `repo mutate` PROCESS, named exactly by --labels-file.
// The hook used to run one process per row. Each rebuilt the tool and ran its
// row's baseline again, and `--only` matches part of a label, so one label
// could select two rows. Pushes waited 370 s, 886 s and 2,651 s.
//
// ⛔ AN OLDER TREE'S mutate TAKES NO --labels-file, and the hook is built from
// the checkout that holds it, not from the pushed tree. A branch cut before
// the flag would exit 2 there, and every push of it would be refused. So the
// job asks the tree's own mutate first. An older one runs each row alone
// through --only, as the hook did before, and says so. Every row runs, and the
// first that did not pass decides the exit. ⚠ Each row reads /dev/null, so a
// row that reads stdin cannot eat the labels after it.
//
// ⚠ IT IS NOT ALL OF CI. The `checks` job also runs the doctor probe and the
// gate, and the sharded `mutations` job runs every row. Those run in CI alone,
// and the README and docs/conventions/git.md say so.
func Jobs(p Plan) []Job {
	jobs := []Job{
		{Name: "shellcheck", Image: ShellCheckImage, Setup: shellcheckVersionCheck + ` && apk add --no-cache git >/dev/null`, Body: `shellcheck --version && git ls-files -z "*.sh" | xargs -0 shellcheck -s sh`},
		{Name: "go", Image: GoImage, Body: `sh scripts/common/check-go.sh`, Go: true},
	}
	if len(p.Rows) > 0 {
		n := p.MutateJobs
		if n <= 0 {
			n = DefaultMutateJobs(runtime.NumCPU())
		}
		labels := ShellQuote(p.Labels)
		exact := fmt.Sprintf("sh scripts/common/repo.sh mutate --jobs %d --labels-file %s", n, labels)
		older := `rc=0; while IFS= read -r l; do sh scripts/common/repo.sh mutate --only "$l" </dev/null || { c=$?; [ "$rc" -ne 0 ] || rc=$c; }; done < ` + labels + `; (exit "$rc")`
		body := fmt.Sprintf("if %s; then %s; else %s; %s; fi", labelsFileProbe, exact, olderMutateSays, older)
		jobs = append(jobs, Job{Name: "mutations", Image: GoImage, Body: body, Go: true})
	}
	return jobs
}

// ContainerTar and ContainerSrc are where a container job finds the archive
// and unpacks it, and ContainerLabels the rows it reads. The workspace is
// copied to /work.
const (
	ContainerTar    = "/work/" + TarName
	ContainerLabels = "/work/" + LabelsName
	ContainerSrc    = "/src"
)

// GoCache is one named cache a Go job mounts, and the variable that points Go
// at it.
type GoCache struct {
	Name, Path, Env string
}

// GoCaches are the build cache and the module cache, kept between runs.
//
// ⭐ A CONTAINER STARTS WITH NEITHER, so every run compiled every package and
// fetched every module again. `run --cache NAME:/path` keeps a podman volume
// in the base between jobs. Go's caches are safe to share between processes,
// so the go and rows jobs mount the same two, even side by side.
//
// ⚠ WHAT THEY SAVE IS NOT MEASURED. Release 7.0.2 has no --cache, so no timed
// run and no real push has mounted them yet.
//
// ⛔ AND NO TEST RESULT IS TAKEN FROM THEM. A kept build cache also keeps test
// results, and a cached pass is not a run on this tree. GOFLAGS=-count=1 makes
// every `go test` run.
var GoCaches = []GoCache{
	{Name: "linux-proof-go-build", Path: "/var/cache/linux-proof/go-build", Env: "GOCACHE"},
	{Name: "linux-proof-go-mod", Path: "/var/cache/linux-proof/go-mod", Env: "GOMODCACHE"},
}

// RunArgs is the wsl-toolkit command line for j over the workspace ws, which
// holds the tree's archive and the rows' labels. caches mounts GoCaches for a
// Go job, and only a tool whose run takes --cache can be asked to.
//
// ⚠ 45 MINUTES, because the go job runs every module's suite in one
// container, and a job cut short proves nothing.
func RunArgs(j Job, ws, tree string, caches bool) []string {
	args := []string{
		"run", "--image", j.Image, "--container-lifecycle", "ephemeral", "--timeout", "45m",
		"--workspace", ws,
	}
	if caches && j.Go {
		for _, c := range GoCaches {
			args = append(args, "--cache", c.Name+":"+c.Path, "--env", c.Env+"="+c.Path)
		}
		args = append(args, "--env", "GOFLAGS=-count=1")
	}
	return append(args, "-c", Script(j, ContainerTar, ContainerSrc, tree, true))
}
