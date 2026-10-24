// hooks_test.go - the checkout is running the hook that the gate cannot be.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"path/filepath"
	"strings"
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

// TestHooksRefusesATreeWithNoPrePushHook. ⛔ The pre-push hook is the one place
// a push is refused for a tree CI's Linux jobs never ran on, and one that does
// not call `repo pre-push` holds its own copy of that rule.
func TestHooksRefusesATreeWithNoPrePushHook(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(HooksDir+"/commit-msg", "\"$BIN\" commit-msg \"$1\"\n")
	mentions := func(got Result, word string) bool {
		for _, d := range got.Detail {
			if strings.Contains(d, word) {
				return true
			}
		}
		return false
	}

	missing := Hooks(&Tree{Root: root, Files: []string{HooksDir + "/commit-msg"}, cache: map[string][]byte{}})
	if !mentions(missing, "pre-push is not tracked") {
		t.Fatalf("a tree with no pre-push hook was not refused for it: %v", missing.Detail)
	}

	write(HooksDir+"/pre-push", "#!/bin/sh\nexit 0\n")
	own := Hooks(&Tree{Root: root, Files: []string{HooksDir + "/commit-msg", HooksDir + "/pre-push"}, cache: map[string][]byte{}})
	if !mentions(own, "repo pre-push") {
		t.Fatalf("a pre-push hook deciding by itself was not refused: %v", own.Detail)
	}

	write(HooksDir+"/pre-push", "exec \"$BIN\" pre-push \"$@\"\n")
	good := Hooks(&Tree{Root: root, Files: []string{HooksDir + "/commit-msg", HooksDir + "/pre-push"}, cache: map[string][]byte{}})
	if mentions(good, "pre-push") {
		t.Fatalf("a pre-push hook that calls repo pre-push was refused: %v", good.Detail)
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
