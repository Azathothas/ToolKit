// SPDX-License-Identifier: 0BSD

package linuxproof

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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
	path, err := Write(common, Proof{Tree: treeA, Commit: treeB, Jobs: []string{"go", "shellcheck"}})
	if err != nil {
		t.Fatal(err)
	}
	if !Has(common, treeA) {
		t.Fatal("the tree just proved was not found")
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
	proved   map[string]bool
}

func (f fakeRepo) Tree(rev string) (string, bool) { t, ok := f.trees[rev]; return t, ok }
func (f fakeRepo) OnRemote(rev, remote string) bool {
	return remote == "origin" && f.onRemote[rev]
}
func (f fakeRepo) Proved(tree string) bool { return f.proved[tree] }

func refs(rs []Refusal) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Ref)
	}
	return strings.Join(out, ",")
}

// TestTheHookRefusesAnUnprovedTree and accepts a proved one.
func TestTheHookRefusesAnUnprovedTree(t *testing.T) {
	c1, c2 := strings.Repeat("1", 40), strings.Repeat("2", 40)
	repo := fakeRepo{trees: map[string]string{c1: treeA, c2: treeB}, proved: map[string]bool{treeA: true}}
	got := Decide([]Update{
		{"refs/heads/a", c1, "refs/heads/a", zero},
		{"refs/heads/b", c2, "refs/heads/b", zero},
	}, "origin", repo)
	if refs(got) != "refs/heads/b" || got[0].Tree != treeB {
		t.Fatalf("Decide = %+v; want refs/heads/b refused for tree B alone", got)
	}
}

// TestTheHookLetsADeletedRefGo. A deletion pushes no tree to prove.
func TestTheHookLetsADeletedRefGo(t *testing.T) {
	c1 := strings.Repeat("1", 40)
	got := Decide([]Update{{"(delete)", zero, "refs/heads/gone", c1}}, "origin", fakeRepo{})
	if len(got) != 0 {
		t.Fatalf("a deleted ref was refused: %+v", got)
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
	if got := Decide([]Update{{"refs/tags/v1", tag, "refs/tags/v1", zero}}, "origin", repo); len(got) != 0 {
		t.Fatalf("a tag on a commit the remote holds was refused: %+v", got)
	}
	if got := Decide([]Update{{"refs/heads/m", amended, "refs/heads/m", was}}, "origin", repo); len(got) != 0 {
		t.Fatalf("an amended commit with the remote ref's own tree was refused: %+v", got)
	}
	// ⛔ The same tag pushed to a remote this checkout does not track is new
	// there, and needs its proof.
	if got := Decide([]Update{{"refs/tags/v1", tag, "refs/tags/v1", zero}}, "elsewhere", repo); len(got) != 1 {
		t.Fatalf("a tag new to another remote went unproved: %+v", got)
	}
}

// TestTheHookRefusesATreeGitCannotName rather than waving it through.
func TestTheHookRefusesATreeGitCannotName(t *testing.T) {
	c1 := strings.Repeat("1", 40)
	got := Decide([]Update{{"refs/heads/x", c1, "refs/heads/x", zero}}, "origin", fakeRepo{proved: map[string]bool{"": true}})
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

// TestTheJobsAreCIsLinuxJobsWithTheAddedRows, each label one sh word however
// it is spelt.
func TestTheJobsAreCIsLinuxJobsWithTheAddedRows(t *testing.T) {
	if got := Jobs(nil); len(got) != 2 || got[0].Name != "go" || got[1].Name != "shellcheck" {
		t.Fatalf("Jobs(nil) = %+v; want the go and shellcheck jobs", got)
	}
	jobs := Jobs([]string{"a project's row", "plain"})
	if len(jobs) != 3 || jobs[2].Image != "docker.io/library/golang:1.25" {
		t.Fatalf("Jobs(rows) = %+v; want a mutations job in golang:1.25", jobs)
	}
	if !strings.Contains(jobs[2].Script, `mutate --only 'a project'\''s row' && sh scripts/common/repo.sh mutate --only 'plain'`) {
		t.Fatalf("the mutations script does not select each row as one word: %s", jobs[2].Script)
	}
	if strings.Contains(jobs[0].Script, "mutate") || !strings.Contains(jobs[0].Script, "check-go.sh") {
		t.Fatalf("the go job is not CI's go job: %s", jobs[0].Script)
	}
	args := strings.Join(RunArgs(jobs[0]), " ")
	for _, want := range []string{"--container-lifecycle ephemeral", "--gitignore", "--exclude .git ", "--exclude .claude", "--workspace ."} {
		if !strings.Contains(args, want) {
			t.Errorf("RunArgs lacks %q: %s", want, args)
		}
	}
	if sh, err := exec.LookPath("sh"); err == nil {
		out, err := exec.Command(sh, "-c", "printf %s "+ShellQuote(`it's "x" $HOME`)).Output()
		if err != nil || string(out) != `it's "x" $HOME` {
			t.Fatalf("ShellQuote did not survive sh: %q, %v", out, err)
		}
	}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@invalid", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, errBuf.String())
	}
	return strings.TrimSpace(out.String())
}

// TestPrePushAnswersFromARealCheckout drives the hook's own entry point
// against a real repository and a bare remote, so the git half of each
// decision is read from git rather than from a fake.
func TestPrePushAnswersFromARealCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH, and the hook reads git")
	}
	root := t.TempDir()
	work, bare := filepath.Join(root, "work"), filepath.Join(root, "bare.git")
	gitIn(t, root, "init", "-q", "--bare", bare)
	gitIn(t, root, "init", "-q", work)
	if err := os.WriteFile(filepath.Join(work, "f"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "add", "f")
	gitIn(t, work, "commit", "-q", "-m", "one")
	gitIn(t, work, "remote", "add", "origin", bare)
	head := gitIn(t, work, "rev-parse", "HEAD")
	tree := gitIn(t, work, "rev-parse", "HEAD^{tree}")
	t.Chdir(work)

	push := func(line string) (int, string) {
		var errOut bytes.Buffer
		code := PrePush([]string{"origin", bare}, strings.NewReader(line+"\n"), &errOut)
		return code, errOut.String()
	}
	line := "refs/heads/main " + head + " refs/heads/main " + zero
	code, msg := push(line)
	if code != 1 || !strings.Contains(msg, tree) || !strings.Contains(msg, "linux-proof") {
		t.Fatalf("an unproved tree: exit %d, %q; want 1 naming the tree and the command", code, msg)
	}
	common := gitIn(t, work, "rev-parse", "--absolute-git-dir")
	if _, err := Write(common, Proof{Tree: tree, Commit: head}); err != nil {
		t.Fatal(err)
	}
	if code, msg := push(line); code != 0 {
		t.Fatalf("a proved tree: exit %d, %q", code, msg)
	}
	if err := os.Remove(filepath.Join(common, ProofDirName, tree+".json")); err != nil {
		t.Fatal(err)
	}
	// ⭐ Once the commit is on the remote's tracked refs, a tag on it needs no
	// proof: this is `repo release --publish` pushing a tag on a pushed main.
	gitIn(t, work, "push", "-q", "origin", "HEAD:refs/heads/main")
	gitIn(t, work, "fetch", "-q", "origin")
	gitIn(t, work, "tag", "-a", "v1", "-m", "v1")
	tag := gitIn(t, work, "rev-parse", "v1")
	if code, msg := push("refs/tags/v1 " + tag + " refs/tags/v1 " + zero); code != 0 {
		t.Fatalf("a tag on a pushed commit: exit %d, %q", code, msg)
	}
	if code := PrePush([]string{"origin"}, strings.NewReader("not a line\n"), &bytes.Buffer{}); code != 2 {
		t.Fatalf("unreadable input: exit %d, want 2", code)
	}
}
