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

// fakeEnv makes this test binary stand in for wsl-toolkit: it appends its
// arguments to the file the variable names and then does what fakeModeEnv says.
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
		f.WriteString(strings.Join(os.Args[1:], "\x1f") + "\n")
		f.Close()
		switch os.Getenv(fakeModeEnv) {
		case "fail":
			os.Exit(3)
		case "dirty":
			os.WriteFile("written-by-the-job", []byte("x\n"), 0o644)
		case "commit":
			// ⚠ Once: the second job finds the commit already made.
			if _, err := os.Stat("committed-by-the-job"); err == nil {
				os.Exit(0)
			}
			os.WriteFile("committed-by-the-job", []byte("x\n"), 0o644)
			for _, args := range [][]string{{"add", "committed-by-the-job"}, {"-c", "user.name=t", "-c", "user.email=t@invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "moved"}} {
				if err := exec.Command("git", args...).Run(); err != nil {
					os.Exit(91)
				}
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type proofCase struct {
	work, common, log, tool string
}

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
	if err := os.WriteFile(table, []byte(`{"schema":"repo-mutations/1","mutations":[{"label":"old row","find":"x"}]}`), 0o644); err != nil {
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

// TestAProofIsFiledForTheTreeEveryJobPassedOn.
func TestAProofIsFiledForTheTreeEveryJobPassedOn(t *testing.T) {
	c := newProofCase(t)
	tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
	code, msg := c.run(t, Options{})
	if code != 0 || !Has(c.common, tree) {
		t.Fatalf("exit %d, proved %v: %s", code, Has(c.common, tree), msg)
	}
	if got := c.calls(t); len(got) != 2 || !strings.Contains(got[0], "golang:1.25") || !strings.Contains(got[1], "ubuntu:24.04") {
		t.Fatalf("the jobs run were %q; want go then shellcheck", got)
	}
}

// TestADirtyTreeRunsNothing. ⛔ The copy would not be HEAD's tree.
func TestADirtyTreeRunsNothing(t *testing.T) {
	c := newProofCase(t)
	if err := os.WriteFile(filepath.Join(c.work, "uncommitted"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, msg := c.run(t, Options{})
	if code != 2 || !strings.Contains(msg, "uncommitted") {
		t.Fatalf("a dirty tree: exit %d, %q; want 2 naming the uncommitted change", code, msg)
	}
	if got := c.calls(t); len(got) != 0 {
		t.Fatalf("a dirty tree still ran %q", got)
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

// TestATreeThatChangedDuringTheRunFilesNoProof: an edit, and a commit, made
// while the jobs ran.
func TestATreeThatChangedDuringTheRunFilesNoProof(t *testing.T) {
	for _, mode := range []string{"dirty", "commit"} {
		t.Run(mode, func(t *testing.T) {
			c := newProofCase(t)
			t.Setenv(fakeModeEnv, mode)
			tree := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
			code, msg := c.run(t, Options{})
			now := gitIn(t, c.work, "rev-parse", "HEAD^{tree}")
			if code != 2 || Has(c.common, tree) || Has(c.common, now) {
				t.Fatalf("%s during the run: exit %d, %q; want 2 and no proof of either tree", mode, code, msg)
			}
		})
	}
}

// TestRowsSinceRunsTheRowsAddedSinceTheRef in a third job.
func TestRowsSinceRunsTheRowsAddedSinceTheRef(t *testing.T) {
	c := newProofCase(t)
	table := filepath.Join(c.work, filepath.FromSlash(TableRel))
	if err := os.WriteFile(table, []byte(`{"schema":"repo-mutations/1","mutations":[{"label":"a new row","find":"y"},{"label":"old row","find":"x"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, c.work, "commit", "-q", "-a", "-m", "two")
	code, msg := c.run(t, Options{RowsSince: "base"})
	if code != 0 || !strings.Contains(msg, "1 mutation row(s) added since base") {
		t.Fatalf("exit %d, %q", code, msg)
	}
	got := c.calls(t)
	if len(got) != 3 || !strings.Contains(got[2], "mutate --only 'a new row'") || strings.Contains(got[2], "old row") {
		t.Fatalf("the jobs run were %q; want a third running the new row alone", got)
	}
	if code, _ := c.run(t, Options{RowsSince: "no-such-ref"}); code != 2 {
		t.Fatalf("an unknown ref: exit %d, want 2", code)
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
