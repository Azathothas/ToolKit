// SPDX-License-Identifier: 0BSD

package linuxproof

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	treeA = strings.Repeat("a", 40)
	treeB = strings.Repeat("b", 40)
	zero  = strings.Repeat("0", 40)
)

// TestAProofIsKeyedByAFullTreeName. ⛔ The key becomes a file name, so a name
// that is not a full object name is refused before it is joined to a path.
func TestAProofIsKeyedByAFullTreeName(t *testing.T) {
	common := t.TempDir()
	path, err := ProofPath(common, treeA)
	if err != nil || path != filepath.Join(common, ProofDirName, treeA+".json") {
		t.Fatalf("ProofPath = %q, %v", path, err)
	}
	if _, err := ProofPath(common, strings.Repeat("c", 64)); err != nil {
		t.Fatalf("a SHA-256 tree name was refused: %v", err)
	}
	for _, bad := range []string{"", "HEAD", "../" + treeA[:37], strings.ToUpper(treeA), treeA[:39], treeA + "a"} {
		if _, err := ProofPath(common, bad); err == nil {
			t.Errorf("ProofPath accepted %q as a tree", bad)
		}
	}
}

// TestAProofIsFoundForItsOwnTreeAlone. ⛔ A proof copied under another tree's
// name names the tree it was made for, and that is not this one.
func TestAProofIsFoundForItsOwnTreeAlone(t *testing.T) {
	common := t.TempDir()
	if Has(common, treeA) {
		t.Fatal("a tree with no proof file was reported proved")
	}
	path, err := Write(common, Proof{Tree: treeA, Commit: treeB, Jobs: []string{"go", "shellcheck"}, Rows: []string{"r"}})
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := Load(common, treeA); !ok || strings.Join(p.Rows, ",") != "r" {
		t.Fatalf("the tree just proved was not found with its rows: %+v, %v", p, ok)
	}
	if Has(common, treeB) {
		t.Fatal("a different tree was reported proved")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	forged, err := ProofPath(common, treeB)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(forged, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if Has(common, treeB) {
		t.Fatal("tree A's proof, copied under tree B's name, proved tree B")
	}
	if err := os.WriteFile(forged, []byte(`{"tree":"`+treeB+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if Has(common, treeB) {
		t.Fatal("a file with no proof schema was taken for a proof")
	}
}

type fakeRepo struct {
	trees    map[string]string
	onRemote map[string]bool
	proofs   map[string]Proof
	base     map[string]string
	rows     map[string][]string
}

func (f fakeRepo) Tree(rev string) (string, bool) { t, ok := f.trees[rev]; return t, ok }
func (f fakeRepo) OnRemote(rev, remote string) bool {
	return remote == "origin" && f.onRemote[rev]
}
func (f fakeRepo) Base(rev, remote string) string {
	if remote != "origin" {
		return ""
	}
	return f.base[rev]
}
func (f fakeRepo) AddedRows(base, rev string) ([]string, error) { return f.rows[base+".."+rev], nil }
func (f fakeRepo) Proof(tree string) (Proof, bool) {
	p, ok := f.proofs[tree]
	return p, ok
}

func refs(ns []Need) string {
	var out []string
	for _, n := range ns {
		out = append(out, n.Ref)
	}
	return strings.Join(out, ",")
}

// TestTheHookProvesAnUnprovedTree and lets a proved one go.
func TestTheHookProvesAnUnprovedTree(t *testing.T) {
	c1, c2 := strings.Repeat("1", 40), strings.Repeat("2", 40)
	repo := fakeRepo{trees: map[string]string{c1: treeA, c2: treeB}, proofs: map[string]Proof{treeA: {Tree: treeA}}}
	needs, refused := Decide([]Update{
		{"refs/heads/a", c1, "refs/heads/a", zero},
		{"refs/heads/b", c2, "refs/heads/b", zero},
	}, "origin", repo)
	if len(refused) != 0 || refs(needs) != "refs/heads/b" || needs[0].Tree != treeB || needs[0].Commit != c2 {
		t.Fatalf("Decide = %+v, %+v; want refs/heads/b to prove for tree B alone", needs, refused)
	}
}

// TestAProofWithoutTheAddedRowsDoesNotCoverThem. ⛔ A row the push adds is
// proved on Linux like a case, and the rows are counted from the remote ref,
// or for a new ref from the newest commit the remote holds.
func TestAProofWithoutTheAddedRowsDoesNotCoverThem(t *testing.T) {
	c1, was, base := strings.Repeat("1", 40), strings.Repeat("5", 40), strings.Repeat("6", 40)
	repo := fakeRepo{
		trees:  map[string]string{c1: treeA, was: treeB},
		proofs: map[string]Proof{treeA: {Tree: treeA, Rows: []string{"old"}}},
		base:   map[string]string{c1: base},
		rows:   map[string][]string{was + ".." + c1: {"old", "new"}, base + ".." + c1: {"old"}},
	}
	needs, _ := Decide([]Update{
		{"refs/heads/m", c1, "refs/heads/m", was},
	}, "origin", repo)
	if len(needs) != 1 || strings.Join(needs[0].Rows, ",") != "old,new" || needs[0].Base != was {
		t.Fatalf("a push adding a row the proof never ran: %+v", needs)
	}
	needs, _ = Decide([]Update{
		{"refs/heads/n", c1, "refs/heads/n", zero},
	}, "origin", repo)
	if len(needs) != 0 {
		t.Fatalf("a new ref whose added rows the proof ran: %+v", needs)
	}
	repo.rows[base+".."+c1] = []string{"new"}
	needs, _ = Decide([]Update{
		{"refs/heads/n", c1, "refs/heads/n", zero},
	}, "origin", repo)
	if len(needs) != 1 || needs[0].Base != base || strings.Join(needs[0].Rows, ",") != "new" {
		t.Fatalf("a new ref counted from the remote's newest commit: %+v", needs)
	}
	if got := Unrun([]string{"a", "b"}, []string{"b", "c"}); strings.Join(got, ",") != "c" {
		t.Fatalf("Unrun = %v", got)
	}
}

// TestTheHookLetsADeletedRefGo. A deletion pushes no tree to prove.
func TestTheHookLetsADeletedRefGo(t *testing.T) {
	c1 := strings.Repeat("1", 40)
	needs, refused := Decide([]Update{
		{"(delete)", zero, "refs/heads/gone", c1},
	}, "origin", fakeRepo{})
	if len(needs)+len(refused) != 0 {
		t.Fatalf("a deleted ref was not let go: %+v %+v", needs, refused)
	}
}

// TestTheHookLetsATreeTheRemoteHoldsGo: a tag on a commit the remote already
// has, and an amended commit whose tree the remote ref already carries.
func TestTheHookLetsATreeTheRemoteHoldsGo(t *testing.T) {
	tag, amended, was := strings.Repeat("3", 40), strings.Repeat("4", 40), strings.Repeat("5", 40)
	repo := fakeRepo{
		trees:    map[string]string{tag: treeA, amended: treeB, was: treeB},
		onRemote: map[string]bool{tag: true},
	}
	if got, _ := Decide([]Update{
		{"refs/tags/v1", tag, "refs/tags/v1", zero},
	}, "origin", repo); len(got) != 0 {
		t.Fatalf("a tag on a commit the remote holds needs a proof: %+v", got)
	}
	if got, _ := Decide([]Update{
		{"refs/heads/m", amended, "refs/heads/m", was},
	}, "origin", repo); len(got) != 0 {
		t.Fatalf("an amended commit with the remote ref's own tree needs a proof: %+v", got)
	}
	// ⛔ The same tag pushed to a remote this checkout does not track is new
	// there, and needs its proof.
	if got, _ := Decide([]Update{
		{"refs/tags/v1", tag, "refs/tags/v1", zero},
	}, "elsewhere", repo); len(got) != 1 {
		t.Fatalf("a tag new to another remote went unproved: %+v", got)
	}
}

// TestTheHookRefusesATreeGitCannotName rather than waving it through.
func TestTheHookRefusesATreeGitCannotName(t *testing.T) {
	c1 := strings.Repeat("1", 40)
	_, got := Decide([]Update{
		{"refs/heads/x", c1, "refs/heads/x", zero},
	}, "origin", fakeRepo{proofs: map[string]Proof{"": {}}})
	if len(got) != 1 || !strings.Contains(got[0].Reason, "cannot name") {
		t.Fatalf("an unnamed tree was accepted: %+v", got)
	}
}

// TestTheHookRefusesInputItCannotRead. ⛔ A skipped line is an unproved ref.
func TestTheHookRefusesInputItCannotRead(t *testing.T) {
	c1 := strings.Repeat("1", 40)
	got, err := ParseUpdates(strings.NewReader("refs/heads/a " + c1 + " refs/heads/a " + zero + "\n\n"))
	if err != nil || len(got) != 1 || got[0].LocalSHA != c1 || got[0].RemoteSHA != zero {
		t.Fatalf("ParseUpdates = %+v, %v", got, err)
	}
	if _, err := ParseUpdates(strings.NewReader("refs/heads/a " + c1 + "\n")); err == nil {
		t.Fatal("a two-field line was skipped rather than refused")
	}
	if !Zero(zero) || !Zero(strings.Repeat("0", 64)) || Zero(c1) || Zero("") {
		t.Fatal("Zero does not answer git's all-zeros name alone")
	}
}

// TestAddedRowsAreNewOrChangedRows. ⛔ An edited row is a new claim.
func TestAddedRowsAreNewOrChangedRows(t *testing.T) {
	before := []byte(`{"schema":"repo-mutations/1","mutations":[
		{"label":"kept","find":"x","replace":""},
		{"label":"edited","find":"y","replace":""},
		{"label":"removed","find":"z","replace":""}]}`)
	after := []byte(`{"schema":"repo-mutations/1","mutations":[
		{"label":"new one","find":"w","replace":""},
		{"replace":"","find":"x","label":"kept"},
		{"label":"edited","find":"y2","replace":""}]}`)
	got, err := AddedRows(before, after)
	if err != nil || strings.Join(got, "|") != "new one|edited" {
		t.Fatalf("AddedRows = %q, %v; want the new row and the edited one", got, err)
	}
	all, err := AddedRows(nil, after)
	if err != nil || len(all) != 3 {
		t.Fatalf("a table absent before: %q, %v; want every row", all, err)
	}
	if _, err := AddedRows(nil, []byte(`{"mutations":[{"find":"x"}]}`)); err == nil {
		t.Fatal("a row with no label was accepted, and it could not be selected")
	}
}

// TestTheJobsAreCIsLinuxJobsWithTheAddedRows, the rows in one process that
// reads their labels from a file, and each path one sh word however it is
// spelt.
func TestTheJobsAreCIsLinuxJobsWithTheAddedRows(t *testing.T) {
	if got := Jobs(Plan{}); len(got) != 2 || got[0].Name != "shellcheck" || got[1].Name != "go" || got[0].Go || !got[1].Go {
		t.Fatalf("Jobs(no rows) = %+v; want the shellcheck job and the go job, which builds Go", got)
	}
	jobs := Jobs(Plan{Rows: []string{"a project's row", "plain"}, Labels: "/w/it's labels", MutateJobs: 3})
	if len(jobs) != 3 || jobs[2].Image != GoImage || !jobs[2].Go {
		t.Fatalf("Jobs(rows) = %+v; want a mutations job in %s", jobs, GoImage)
	}
	if want := `then sh scripts/common/repo.sh mutate --jobs 3 --labels-file '/w/it'\''s labels';`; !strings.Contains(jobs[2].Body, want) {
		t.Fatalf("the mutations script is %s; want one process, %s", jobs[2].Body, want)
	}
	if strings.Contains(jobs[1].Body, "mutate") || !strings.Contains(jobs[1].Body, "check-go.sh") {
		t.Fatalf("the go job is not CI's go job: %s", jobs[1].Body)
	}
	ws := filepath.Join("C:", "x", ".tmp", "linux-proof-1")
	args := RunArgs(jobs[0], ws, treeA, true)
	joined := strings.Join(args, " ")
	for _, want := range []string{"--container-lifecycle ephemeral", "--workspace " + ws, "--image " + ShellCheckImage} {
		if !strings.Contains(joined, want) {
			t.Errorf("RunArgs lacks %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "--cache") {
		t.Errorf("the shellcheck job mounts a Go cache: %s", joined)
	}
	if goArgs := strings.Join(RunArgs(jobs[1], ws, treeA, false), " "); strings.Contains(goArgs, "--cache") || strings.Contains(goArgs, "GOFLAGS") {
		t.Errorf("a tool without --cache was handed one: %s", goArgs)
	}
	script := args[len(args)-1]
	if !strings.HasPrefix(script, jobs[0].Setup+" && ") || !strings.Contains(script, ContainerTar) {
		t.Errorf("the container script does not install and unpack first: %s", script)
	}
	if native := Script(jobs[0], "/t/tree.tar", "/t/src", treeA, false); strings.Contains(native, "apk") {
		t.Errorf("a native script installs packages on the host: %s", native)
	}
	if sh, err := exec.LookPath("sh"); err == nil {
		out, err := exec.Command(sh, "-c", "printf %s "+ShellQuote(`it's "x" $HOME`)).Output()
		if err != nil || string(out) != `it's "x" $HOME` {
			t.Fatalf("ShellQuote did not survive sh: %q, %v", out, err)
		}
	}
}

// TestTheRowsRunAtMostTwoAtOnceByDefault. ⛔ mutate's own default is half the
// processors a job sees, and a container sees every thread of the host. With
// nothing set, the rows job asks for half this host's, at least one and at
// most two.
func TestTheRowsRunAtMostTwoAtOnceByDefault(t *testing.T) {
	for cpus, want := range map[int]int{1: 1, 2: 1, 3: 1, 4: 2, 5: 2, 20: 2, 64: 2} {
		if got := DefaultMutateJobs(cpus); got != want {
			t.Errorf("DefaultMutateJobs(%d) = %d, want %d", cpus, got, want)
		}
	}
	body := Jobs(Plan{Rows: []string{"r"}, Labels: "/l"})[2].Body
	if want := fmt.Sprintf("mutate --jobs %d --labels-file '/l'", DefaultMutateJobs(runtime.NumCPU())); !strings.Contains(body, want) {
		t.Fatalf("a plan that names no --jobs runs %s; want %s", body, want)
	}
}

// TestATreeWhoseMutateTakesNoLabelsFileRunsEachRowThroughOnly. ⛔ The hook is
// built from the checkout that holds it, and a tree cut before --labels-file
// has a mutate without it. Its rows run one at a time through --only, every
// one of them, and the first that did not pass decides the exit.
func TestATreeWhoseMutateTakesNoLabelsFileRunsEachRowThroughOnly(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on PATH")
	}
	// fake is a tree's repo.sh. It logs its arguments, reads all of its
	// stdin, answers -h with the flags its mutate takes, and exits 1 or 2 for
	// a label that says so.
	const fake = `printf '%s\n' "$*" >> calls
cat >/dev/null
case "$*" in
"mutate -h") [ -f newer ] && echo '  -labels-file string' >&2; echo '  -only string' >&2; exit 0 ;;
*fails*) exit 1 ;;
*cannot*) exit 2 ;;
esac
`
	run := func(newer bool, rows ...string) (int, []string, string) {
		t.Helper()
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "scripts", "common"), 0o755); err != nil {
			t.Fatal(err)
		}
		files := map[string]string{"scripts/common/repo.sh": fake, LabelsName: strings.Join(rows, "\n") + "\n"}
		if newer {
			files["newer"] = ""
		}
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command(sh, "-c", Jobs(Plan{Rows: rows, Labels: LabelsName, MutateJobs: 2})[2].Body)
		cmd.Dir = dir
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatal(err)
			}
			code = ee.ExitCode()
		}
		calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
		return code, strings.Split(strings.TrimSpace(string(calls)), "\n"), stderr.String()
	}
	if code, calls, _ := run(true, "a row", "it's a row"); code != 0 || strings.Join(calls, "|") != "mutate -h|mutate --jobs 2 --labels-file "+LabelsName {
		t.Fatalf("a tree whose mutate takes --labels-file: exit %d, ran %q; want the rows in one process", code, calls)
	}
	code, calls, said := run(false, "a row", "it's a row")
	if code != 0 || strings.Join(calls, "|") != "mutate -h|mutate --only a row|mutate --only it's a row" {
		t.Fatalf("an older tree: exit %d, ran %q; want each row alone through --only, the second not eaten by the first's stdin", code, calls)
	}
	if !strings.Contains(said, "takes no --labels-file") {
		t.Fatalf("an older tree's rows ran through --only and the job did not say why: %q", said)
	}
	if code, calls, _ := run(false, "this fails", "this cannot", "last"); code != 1 || len(calls) != 4 {
		t.Fatalf("an older tree whose first row fails and second cannot run: exit %d, ran %q; want 1, and every row run", code, calls)
	}
}

// TestTheShellCheckJobRunsCIsVersion. ⛔ The image is pinned by its tag, and
// the job still checks the version it got, because another ShellCheck finds
// other things than CI's. Any other version is "could not run".
func TestTheShellCheckJobRunsCIsVersion(t *testing.T) {
	sc := Jobs(Plan{})[0]
	if !strings.HasSuffix(sc.Image, ":v"+ShellCheckVersion) || !strings.HasPrefix(sc.Setup, shellcheckVersionCheck+" && ") || !strings.Contains(sc.Setup, "apk add --no-cache git") {
		t.Fatalf("the shellcheck job is %+v; want %s pinned, its version checked first, and git added", sc, ShellCheckVersion)
	}
	if native := Script(sc, "/t/tree.tar", "/t/src", treeA, false); strings.Contains(native, "version: ") || !strings.Contains(native, "shellcheck --version") {
		t.Fatalf("a native proof, which runs the host's own ShellCheck, is held to the image's version: %s", native)
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on PATH")
	}
	run := func(version string) (int, string) {
		fake := `shellcheck() { printf 'ShellCheck - shell script analysis tool\nversion: %s\nlicense: GPLv3\n' ` + ShellQuote(version) + `; }; `
		cmd := exec.Command(sh, "-c", fake+shellcheckVersionCheck)
		out, err := cmd.CombinedOutput()
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), string(out)
		}
		return 0, string(out)
	}
	if code, out := run(ShellCheckVersion); code != 0 {
		t.Fatalf("ShellCheck %s: exit %d, %q; want 0", ShellCheckVersion, code, out)
	}
	if code, out := run("0.10.0"); code != 2 || !strings.Contains(out, "the version CI runs") {
		t.Fatalf("ShellCheck 0.10.0: exit %d, %q; want 2", code, out)
	}
}

// TestALinuxHostProvesNatively and Windows always through wsl-toolkit.
func TestALinuxHostProvesNatively(t *testing.T) {
	if !Native("linux", "") || Native("windows", "") || Native("linux", "/x/wsl-toolkit") {
		t.Fatal("Native does not answer a Linux host with no wsl-toolkit named, alone")
	}
}

// TestTheScriptChecksTheTreeItUnpacked runs Script with sh over a real
// archive. ⛔ A copy whose tree is not the one the proof is filed under fails
// before the check runs, so it cannot file a proof of a different tree.
func TestTheScriptChecksTheTreeItUnpacked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the script runs on Linux alone, and git on Windows records no executable bit from the unpacked files")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on PATH")
	}
	for _, tool := range []string{"git", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s on PATH", tool)
		}
	}
	root := t.TempDir()
	work := filepath.Join(root, "work")
	gitIn(t, root, "init", "-q", work)
	if err := os.WriteFile(filepath.Join(work, "run.sh"), []byte("echo hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".gitignore"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "ignored"), []byte("tracked anyway\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "add", "-A")
	gitIn(t, work, "add", "-f", "ignored")
	gitIn(t, work, "update-index", "--chmod=+x", "run.sh")
	gitIn(t, work, "commit", "-q", "-m", "one")
	tree := gitIn(t, work, "rev-parse", "HEAD^{tree}")
	tarPath := filepath.Join(root, TarName)
	gitIn(t, work, "archive", "--format=tar", "-o", tarPath, "HEAD")
	run := func(src, want string) (string, error) {
		job := Job{Name: "t", Body: "echo checked"}
		// ⚠ Relative paths, because GNU tar on Windows reads `C:` as a host.
		cmd := exec.Command(sh, "-c", Script(job, TarName, src, want, false))
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run("src-good", tree); err != nil || !strings.Contains(out, "checked") {
		t.Fatalf("the archive of the tree did not unpack to that tree: %v\n%s", err, out)
	}
	if out, err := run("src-bad", treeA); err == nil || strings.Contains(out, "checked") {
		t.Fatalf("a copy checked against another tree ran the check: %v\n%s", err, out)
	}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@invalid", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "core.hooksPath=/dev/null", "-c", "core.autocrlf=false"}, args...)...)
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, errBuf.String())
	}
	return strings.TrimSpace(out.String())
}
