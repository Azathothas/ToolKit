// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

// TestAProjectPathRefusesADirectoryThatIsNotAProject holds the second half of
// the answer to issue 29's working-directory trap.
//
// ⛔ `--workspace .` RESOLVES AGAINST THE PROCESS'S WORKING DIRECTORY, which a
// sandbox can reset to a drive root or a home directory without the caller
// knowing. The anchor covers a tree that has a configuration in it; this covers
// the rest, and a refusal is the only safe answer because guessing which
// directory was meant is how a tool acts on the wrong tree and reports success.
func TestAProjectPathRefusesADirectoryThatIsNotAProject(t *testing.T) {
	refused := []string{"/", string(filepath.Separator)}
	if runtime.GOOS == "windows" {
		refused = append(refused, `C:\`, `C:\Users`, `C:\Windows`, `D:/`)
	} else {
		refused = append(refused, "/home")
	}
	for _, dir := range refused {
		err := AssertProjectPath("--workspace resolved to", dir)
		if err == nil {
			t.Errorf("%q was accepted as a project directory", dir)
			continue
		}
		if !errors.Is(err, ErrWorkspaceRefused) {
			t.Errorf("%q was refused with an error outside the refusal family: %v", dir, err)
		}
	}
}

// TestAProjectPathAcceptsAnOrdinaryTree is the other half, because a guard that
// refuses everything is not a guard.
func TestAProjectPathAcceptsAnOrdinaryTree(t *testing.T) {
	ok := []string{t.TempDir(), filepath.Join(t.TempDir(), "nested", "project")}
	if runtime.GOOS == "windows" {
		ok = append(ok, `C:\Users\somebody\Downloads\project`, `C:\src`)
	} else {
		ok = append(ok, "/home/runner/project", "/srv/src")
	}
	for _, dir := range ok {
		if err := AssertProjectPath("--workspace resolved to", dir); err != nil {
			t.Errorf("%q was refused: %v", dir, err)
		}
	}
	// An empty value means the flag was not passed at all.
	if err := AssertProjectPath("--workspace resolved to", ""); err != nil {
		t.Errorf("an unset path was refused: %v", err)
	}
}

// TestTheHomeDirectoryItselfIsRefusedAndItsChildrenAreNot draws the line where
// the trap actually is.
//
// ⚠ The tool is installed under the home directory, so a session whose working
// directory was reset lands ON the home directory rather than under it.
// Refusing everything below it would refuse most real projects on this host.
func TestTheHomeDirectoryItselfIsRefusedAndItsChildrenAreNot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	if err := AssertProjectPath("--workspace resolved to", home); err == nil {
		t.Error("the home directory itself was accepted")
	}
	child := filepath.Join(home, "Downloads", "ToolKit")
	if err := AssertProjectPath("--workspace resolved to", child); err != nil {
		t.Errorf("a project under the home directory was refused: %v", err)
	}
}
