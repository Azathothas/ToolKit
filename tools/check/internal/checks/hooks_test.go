// hooks_test.go - the checkout is running the hook that the gate cannot be.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"path/filepath"
	"testing"
)

// TestSameHookPathAcceptsEverySpellingOfTheRightAnswer.
//
// ⚠ git ACCEPTS ALL OF THESE. A string comparison against ".githooks" would
// refuse three spellings of a correct setting, and a check that refuses a correct
// answer is one people turn off.
func TestSameHookPathAcceptsEverySpellingOfTheRightAnswer(t *testing.T) {
	root := t.TempDir()
	abs := filepath.Join(root, HooksDir)
	good := []string{
		HooksDir,
		"./" + HooksDir,
		abs,
		filepath.ToSlash(abs),
	}
	for _, g := range good {
		if !sameHookPath(root, g) {
			t.Errorf("sameHookPath refused %q, which git accepts", g)
		}
	}
}

// TestSameHookPathRefusesSomebodyElsesHooks. ⛔ An unset path and a path
// somewhere else are the same outcome: this checkout is not running the rule.
func TestSameHookPathRefusesSomebodyElsesHooks(t *testing.T) {
	root := t.TempDir()
	bad := []string{
		"",
		"hooks",
		filepath.Join(root, "somewhere-else"),
		filepath.Join(root, HooksDir, "deeper"),
	}
	for _, b := range bad {
		if sameHookPath(root, b) {
			t.Errorf("sameHookPath accepted %q, which is not the tracked hooks directory", b)
		}
	}
}

// TestHooksRefusesATreeWithNoHook covers the half that is a property of the
// TREE rather than of the checkout, so it holds in CI as well as on a laptop.
func TestHooksRefusesATreeWithNoHook(t *testing.T) {
	tree := &Tree{Root: t.TempDir(), Files: []string{"README.md"}, cache: map[string][]byte{}}
	got := Hooks(tree)
	if got.Problems == 0 {
		t.Fatal("a tree that ships no commit-msg hook was reported as fine")
	}
	if got.Extra["installed"] != false {
		t.Fatalf("installed = %v, want false", got.Extra["installed"])
	}
}
