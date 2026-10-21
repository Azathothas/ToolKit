// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-118: a package's own copy of the state directory is named ------------

func fakeLocalAppData(t *testing.T) (local, home, copyPath string) {
	t.Helper()
	local = t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	home = filepath.Join(local, "wsl-toolkit")
	copyPath = filepath.Join(local, "Packages", "Claude_test", "LocalCache", "Local", "wsl-toolkit")
	for _, dir := range []string{home, filepath.Join(copyPath, "jobs")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join(home, "config.json"), filepath.Join(copyPath, "config.json")} {
		if err := os.WriteFile(f, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return local, home, copyPath
}

// ⛔ A process outside the package sees another directory, and is told which
// package keeps the copy and that it does not write there.
func TestACopyThisProcessDoesNotWriteIsNamedAsAnothers(t *testing.T) {
	_, home, copyPath := fakeLocalAppData(t)
	got := StateCopies(home)
	if len(got) != 1 || got[0].Package != "Claude_test" || got[0].Path != copyPath || got[0].Entries != 2 {
		t.Fatalf("the copies read as %+v", got)
	}
	if got[0].ThisProcess {
		t.Error("a process whose home holds other files was read as writing into the copy")
	}
	if note := StateCopyNote(got[0]); !strings.Contains(note, "this one does not") || !strings.Contains(note, "WSL_TOOLKIT_HOME") {
		t.Errorf("the note reads %q", note)
	}
}

// ⭐ Where home IS the copy, as it is through a redirected view, the files are
// the same files, and the note says this process writes there.
func TestACopyThisProcessWritesIsNamedAsItsOwn(t *testing.T) {
	_, _, copyPath := fakeLocalAppData(t)
	// A second root stands in for the redirected view: its package copy and its
	// wsl-toolkit are both links to one directory, as both names are one place
	// to a process Windows redirects.
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	local := filepath.Join(root, "Packages", "Claude_test", "LocalCache", "Local")
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatal(err)
	}
	linkDirForTest(t, copyPath, filepath.Join(local, "wsl-toolkit"))
	linkDirForTest(t, copyPath, filepath.Join(root, "wsl-toolkit"))
	got := StateCopies(filepath.Join(root, "wsl-toolkit"))
	if len(got) != 1 || !got[0].ThisProcess {
		t.Fatalf("a home that is the copy read as %+v", got)
	}
	if note := StateCopyNote(got[0]); !strings.Contains(note, "THIS process writes there") {
		t.Errorf("the note reads %q", note)
	}
}

func TestAHomeOutsideLocalAppDataHasNoCopy(t *testing.T) {
	fakeLocalAppData(t)
	if got := StateCopies(t.TempDir()); got != nil {
		t.Errorf("a home outside %%LOCALAPPDATA%% read as copied: %+v", got)
	}
	t.Setenv("LOCALAPPDATA", "")
	if got := StateCopies(filepath.Join(t.TempDir(), "wsl-toolkit")); got != nil {
		t.Errorf("with no %%LOCALAPPDATA%% a copy was found: %+v", got)
	}
}
