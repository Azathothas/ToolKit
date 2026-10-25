// SPDX-License-Identifier: 0BSD

package checks

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// TestABatchThatPassesIsAskedOnce is the point of batching shellcheck: a clean
// batch costs one process, and no clean file is asked about again on its own.
func TestABatchThatPassesIsAskedOnce(t *testing.T) {
	withSlots(t, 2)
	var calls atomic.Int64
	found := lintInBatches([]string{"a.sh", "b.sh", "c.sh", "d.sh"}, []int64{1, 1, 1, 1},
		func([]string) (string, error) {
			calls.Add(1)
			return "", nil
		})
	if len(found) != 0 {
		t.Fatalf("a clean tree produced findings: %q", found)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("four clean files in two batches cost %d processes, want 2", n)
	}
}

// TestAFailingBatchNamesOnlyItsFailingFile is the attribution half. A batch's
// output runs every file's findings together, so each file in a failing batch
// is asked again alone. Only a file that fails alone is named, with the text
// its own run gave.
func TestAFailingBatchNamesOnlyItsFailingFile(t *testing.T) {
	withSlots(t, 2)
	failed := errors.New("exit status 1")
	found := lintInBatches([]string{"a.sh", "bad.sh", "c.sh", "d.sh"}, []int64{1, 1, 1, 1},
		func(files []string) (string, error) {
			if !slices.Contains(files, "bad.sh") {
				return "", nil
			}
			if len(files) > 1 {
				return "In a.sh line 1:\n  ^-- SC1000 (error): the batch's output, run together\n", failed
			}
			return "In bad.sh line 3:\n  ^-- SC1009 (error): bad.sh's own finding\n", failed
		})
	want := []string{"bad.sh: ^-- SC1009 (error): bad.sh's own finding"}
	if !reflect.DeepEqual(found, want) {
		t.Fatalf("a failing batch was reported as %q, want %q", found, want)
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
