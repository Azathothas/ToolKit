// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// -- WSL-112: a workspace copy leaves out what git ignores --------------------

func gitWorkTree(t *testing.T) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git on PATH, and --gitignore reads git's own rules")
	}
	root := t.TempDir()
	if out, err := exec.Command(git, "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	files := map[string]string{
		".gitignore":          "/target/\n*.log\n.env/\n",
		"src/main.rs":         "fn main() {}\n",
		"src/target/keep.txt": "a tracked directory named like an ignored one, not at the root\n",
		"target/debug/huge":   "build output\n",
		"notes/untracked.md":  "untracked and not ignored, so it travels\n",
		"run.log":             "ignored by a glob\n",
		".env/token":          "a credential git keeps out\n",
	}
	for name, body := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command(git, "-C", root, "add", ".gitignore", "src").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	return root
}

func tarNames(t *testing.T, archive []byte) []string {
	t.Helper()
	var names []string
	tr := tar.NewReader(bytes.NewReader(archive))
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, strings.TrimSuffix(h.Name, "/"))
	}
	sort.Strings(names)
	return names
}

// ⭐ git decides, so a directory git ignores at the root does not take a tracked
// one of the same name elsewhere, which a glob in a shell string did.
func TestGitignoreLeavesOutWhatGitIgnoresAndNothingElse(t *testing.T) {
	parallelCase(t)
	root := gitWorkTree(t)
	var buf bytes.Buffer
	up, err := writeWorkspaceTar(&buf, root, DefaultWorkspaceLimits(), WorkspaceFilter{Gitignore: true, Excludes: []string{".git"}})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(tarNames(t, buf.Bytes()), " ")
	for _, want := range []string{".gitignore", "src/main.rs", "src/target/keep.txt", "notes/untracked.md"} {
		if !strings.Contains(" "+got+" ", " "+want+" ") {
			t.Errorf("%s did not travel; the copy holds: %s", want, got)
		}
	}
	for _, gone := range []string{"target", "target/debug/huge", "run.log", ".env", ".env/token"} {
		if strings.Contains(" "+got+" ", " "+gone+" ") {
			t.Errorf("%s travelled, and git ignores it; the copy holds: %s", gone, got)
		}
	}
	if up.Ignored != 3 {
		t.Errorf("the copy reports %d ignored path(s); want 3: target/, run.log and .env/", up.Ignored)
	}
}

// ⛔ A nested repository's own rules apply inside it. git's list for the outer
// tree does not look inside, so sub/secrets.env travelled although sub's own
// .gitignore names it. A safety review found it.
func TestGitignoreAppliesANestedRepositorysOwnRules(t *testing.T) {
	parallelCase(t)
	root := gitWorkTree(t)
	git, _ := exec.LookPath("git")
	sub := filepath.Join(root, "vendor", "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(git, "-C", sub, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	for name, body := range map[string]string{".gitignore": "secrets.env\n", "secrets.env": "TOKEN=x\n", "lib.rs": "pub fn f() {}\n"} {
		if err := os.WriteFile(filepath.Join(sub, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if _, err := writeWorkspaceTar(&buf, root, DefaultWorkspaceLimits(), WorkspaceFilter{Gitignore: true, Excludes: []string{".git"}}); err != nil {
		t.Fatal(err)
	}
	got := " " + strings.Join(tarNames(t, buf.Bytes()), " ") + " "
	if strings.Contains(got, " vendor/sub/secrets.env ") {
		t.Errorf("a file the nested repository ignores travelled; the copy holds:%s", got)
	}
	if !strings.Contains(got, " vendor/sub/lib.rs ") {
		t.Errorf("the nested repository's own file did not travel; the copy holds:%s", got)
	}
}

func TestGitignoreOutsideAGitWorkTreeIsRefused(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git on PATH")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// ⚠ THE TEMPORARY DIRECTORY MAY BE INSIDE A REPOSITORY, as it is under this
	// repository's own .tmp, so git is told where to stop looking.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	var buf bytes.Buffer
	if _, err := writeWorkspaceTar(&buf, root, DefaultWorkspaceLimits(), WorkspaceFilter{Gitignore: true}); err == nil || !strings.Contains(err.Error(), "not in a git work tree") {
		t.Fatalf("--gitignore outside a work tree answered %v; want a refusal, not a copy of everything", err)
	}
}

func TestGitignoreOnAWorkspaceItsRepositoryIgnoresIsRefused(t *testing.T) {
	parallelCase(t)
	root := gitWorkTree(t)
	ws := filepath.Join(root, "target", "debug")
	var buf bytes.Buffer
	if _, err := writeWorkspaceTar(&buf, ws, DefaultWorkspaceLimits(), WorkspaceFilter{Gitignore: true}); err == nil || !strings.Contains(err.Error(), "is itself ignored") {
		t.Fatalf("--gitignore on a directory its repository ignores answered %v", err)
	}
}

func TestAProjectsExcludePatternIsReadAndABadOneRefused(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Jobs.Exclude = []string{"target", ".git"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("a good jobs.exclude was refused: %v", err)
	}
	cfg.Jobs.Exclude = []string{"[target"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "jobs.exclude[0]") {
		t.Fatalf("a pattern path.Match cannot read answered %v", err)
	}
	cfg.Jobs.Exclude = []string{" "}
	if err := cfg.Validate(); err == nil {
		t.Fatal("an empty pattern was accepted")
	}
}
