// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	tree := &Tree{Root: root}
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
