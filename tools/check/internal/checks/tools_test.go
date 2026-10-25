// SPDX-License-Identifier: 0BSD

package checks

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestEachScriptIsLintedAlone is the batch hole. shellcheck follows a `.` only
// to a file that is also on its command line, and reports SC1091 when it is
// not. The fake lint below does the same, so a.sh passes beside lib/vars.sh and
// fails alone. a.sh is named with its lone run's text whatever the slot count.
func TestEachScriptIsLintedAlone(t *testing.T) {
	files := []string{"a.sh", "lib/vars.sh", "z.sh"}
	failed := errors.New("exit status 1")
	lint := func(batch []string) (string, error) {
		if slices.Contains(batch, "a.sh") && !slices.Contains(batch, "lib/vars.sh") {
			return "In a.sh line 2:\n  ^-- SC1091 (info): Not following: ./lib/vars.sh was not specified as input (see shellcheck -x).\n", failed
		}
		return "", nil
	}
	// ⛔ THE CASE PROVES ITS OWN SETUP. Beside its target, a.sh must pass, or a
	// batch would be named too and the case would guard nothing.
	if out, err := lint(files); err != nil {
		t.Fatalf("a.sh failed beside its target with %q; the fake lint does not model shellcheck", out)
	}
	want := []string{"a.sh: In a.sh line 2:"}
	for _, n := range []int{1, 2, 3, 4} {
		withSlots(t, n)
		if got := lintEach(files, lint); !reflect.DeepEqual(got, want) {
			t.Fatalf("with %d slots, a script that sources another was reported as %q, want %q: it was linted beside its target", n, got, want)
		}
	}
}

// requireShellcheck skips a case that needs the real tool on a host without it.
// ⛔ ITS OWN FUNCTION, as requirePwsh is, so the skip is reported as one.
func requireShellcheck(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("shellcheck"); err != nil {
		t.Skip("shellcheck is not on PATH, so the check under test cannot be driven")
	}
}

// TestTheShellcheckCheckFailsAScriptThatSourcesASibling is the batch hole with
// the real tool and the real check. a.sh sources b.sh and reads a name b.sh
// sets. Alone, a.sh fails with SC1091 and SC2154. Beside b.sh on one command
// line, it passes. The check must name a.sh alone, whatever the slot count.
func TestTheShellcheckCheckFailsAScriptThatSourcesASibling(t *testing.T) {
	requireShellcheck(t)
	tree := treeOf(t, map[string]string{
		"a.sh": "#!/bin/sh\n. ./b.sh\necho \"$greeting\"\n",
		"b.sh": "#!/bin/sh\n# shellcheck disable=SC2034\ngreeting=hi\n",
	})
	// ⛔ THE CASE PROVES ITS OWN SETUP WITH THE SAME TOOL. A shellcheck that
	// passes a.sh alone, or fails it beside b.sh, cannot show the hole. That is
	// a fact about this host's tool, so it skips with the reason.
	sc := func(files ...string) error {
		cmd := exec.Command("shellcheck", append([]string{"-s", "sh"}, files...)...)
		cmd.Dir = tree.Root
		return cmd.Run()
	}
	if sc("a.sh") == nil {
		t.Skip("this shellcheck passes a.sh alone, so it follows the source by itself and there is no hole to guard")
	}
	if err := sc("a.sh", "b.sh"); err != nil {
		t.Skipf("this shellcheck fails a.sh beside b.sh too (%v), so a batch would not hide it", err)
	}
	for _, n := range []int{1, 2, 4} {
		withSlots(t, n)
		r := Shellcheck(tree)
		if r.Problems != 1 || !strings.HasPrefix(strings.Join(r.Detail, "\n"), "a.sh: ") {
			t.Fatalf("with %d slots the check found %d problem(s) %q; a.sh fails alone, so it must be named and nothing else", n, r.Problems, r.Detail)
		}
	}
}

// TestEveryFailingScriptIsNamedWithItsOwnFinding is the attribution. Two of four
// scripts fail, each with its own text. Each is named by its own name with
// that text, in file order, and no clean script is named.
func TestEveryFailingScriptIsNamedWithItsOwnFinding(t *testing.T) {
	withSlots(t, 2)
	failed := errors.New("exit status 1")
	found := lintEach([]string{"a.sh", "bad.sh", "c.sh", "worse.sh"}, func(files []string) (string, error) {
		var out string
		for _, f := range files {
			if f == "bad.sh" || f == "worse.sh" {
				out += "In " + f + " line 3:\n  ^-- SC1009 (error): " + f + "'s own finding\n"
			}
		}
		if out == "" {
			return "", nil
		}
		return out, failed
	})
	want := []string{
		"bad.sh: ^-- SC1009 (error): bad.sh's own finding",
		"worse.sh: ^-- SC1009 (error): worse.sh's own finding",
	}
	if !reflect.DeepEqual(found, want) {
		t.Fatalf("two failing scripts were reported as %q, want %q", found, want)
	}
}

// TestEveryPowerShellSessionIsRead is the merge. Two sessions shared three
// scripts, and what each wrote has to reach the result. Every finding arrives,
// in the order one session gave them, and every count is added.
func TestEveryPowerShellSessionIsRead(t *testing.T) {
	scripts := []string{"a.ps1", "b.ps1", "c.ps1"}
	first := "COUNT|2\r\nFILE|a.ps1\r\nFILE|c.ps1\r\nLINT|c.ps1:4|PSAvoidX: c's finding\r\n"
	second := "PARSE|b.ps1|Missing closing '}'\r\nCOUNT|1\r\nFILE|b.ps1\r\nLINT|b.ps1:1|PSAvoidY: b's finding\r\n"
	found, parsed, noAnalyzer := psRead(scripts, linesOf([][]byte{[]byte(first), []byte(second)}))
	want := []string{
		"b.ps1 does not parse: Missing closing '}'",
		"b.ps1:1 PSAvoidY: b's finding",
		"c.ps1:4 PSAvoidX: c's finding",
	}
	if !reflect.DeepEqual(found, want) {
		t.Fatalf("the sessions were read as %q, want %q", found, want)
	}
	if parsed != "3" {
		t.Fatalf("two sessions parsed 2 and 1 scripts and the check says %s; every count is added", parsed)
	}
	if noAnalyzer {
		t.Fatal("no session said NOANALYZER and the check says the analyzer is missing")
	}
}

// ⛔ ITS OWN TEST FUNCTION, ON PURPOSE. Both cases below need a real PowerShell,
// and a host without one must report SKIPPED rather than pass. A skip folded into
// a shared function reads as a pass for the cases that did run, which is the
// shape CI failed on once already. WSL-90.
func requirePwsh(t *testing.T) {
	t.Helper()
	if findPwsh() == "" {
		t.Skip("no PowerShell on PATH, so the check under test cannot be driven")
	}
}

// treeOf writes the given files under a temporary root and returns a Tree naming
// them, so the check reads a tree this test controls rather than the repository.
func treeOf(t *testing.T, files map[string]string) *Tree {
	t.Helper()
	root := t.TempDir()
	tree := &Tree{Root: root, cache: map[string][]byte{}}
	for name, body := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		tree.Files = append(tree.Files, name)
	}
	return tree
}

// TestThePowerShellCheckRefusesAScriptThatDoesNotParse is the case this check did
// not have, and its absence is why the check could not fail. Until 2026-09-16 it
// read its child's output for "PARSE\t" while the child wrote "PARSE|", AND it
// passed the file list as arguments after -Command, which does not reach $args at
// all - so it parsed nothing and reported ok over every broken script there was.
func TestThePowerShellCheckRefusesAScriptThatDoesNotParse(t *testing.T) {
	requirePwsh(t)
	tree := treeOf(t, map[string]string{
		"good.ps1": "param([string]$A)\nWrite-Output $A\n",
		"bad.ps1":  "if ($true {\n",
	})
	r := PowerShell(tree)
	if r.Problems == 0 {
		t.Fatalf("a script that does not parse was reported clean: %+v", r.Extra)
	}
	if !strings.Contains(strings.Join(r.Detail, "\n"), "bad.ps1") {
		t.Errorf("the failure did not name bad.ps1: %v", r.Detail)
	}
}

// TestThePowerShellCheckRefusesASessionThatParsedNothing is the guard for the
// defect itself rather than for a broken script: a session that read no file list
// parses zero files and finds zero problems, which is indistinguishable from a
// clean tree unless the count is asserted.
func TestThePowerShellCheckRefusesASessionThatParsedNothing(t *testing.T) {
	requirePwsh(t)
	tree := treeOf(t, map[string]string{
		"a.ps1": "Write-Output 1\n",
		"b.ps1": "Write-Output 2\n",
	})
	r := PowerShell(tree)
	if r.Problems != 0 {
		t.Fatalf("two valid scripts were reported as problems: %v", r.Detail)
	}
	if got := r.Extra["parsed"]; got != "2" {
		t.Fatalf("the check reported parsing %v of 2 scripts; it must report what it actually read", got)
	}
}

// ⛔ A failed case is reported with what it said, not only its name.
func TestAFailedGoTestCarriesTheCaseMessage(t *testing.T) {
	out := "--- FAIL: TestX (1.59s)\r\n    x_test.go:12: the base was removed\r\n        and its disk with it\r\nFAIL\r\nFAIL\tmod/pkg\t2.1s\r\n"
	got := testFailure(out)
	want := "--- FAIL: TestX (1.59s) | x_test.go:12: the base was removed | and its disk with it"
	if got != want {
		t.Fatalf("testFailure said %q, want %q", got, want)
	}
	hung := "panic: test timed out after 10m0s\r\n\trunning tests:\r\n\t\tTestAHang (9m59s)\r\n\r\ngoroutine 1 [running]:\r\n"
	if got := testFailure(hung); got != "panic: test timed out after 10m0s | running tests: | TestAHang (9m59s)" {
		t.Fatalf("a timeout was reported as %q, want the panic and the case still running", got)
	}
	if got := testFailure("ok\nsomething else failed\n"); got != "something else failed" {
		t.Fatalf("with no failed case it said %q, want firstFinding's answer", got)
	}
}
