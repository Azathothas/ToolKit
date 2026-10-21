// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// -- WSL-117: a tree Go's removal cannot open is removed entry by entry --------

// failingRemoveTree makes the first removal fail without removing anything, as
// Go's does on a directory Windows redirected into a package's own copy.
func failingRemoveTree(t *testing.T) {
	t.Helper()
	previous := removeTree
	t.Cleanup(func() { removeTree = previous })
	removeTree = func(path string) error {
		return &os.PathError{Op: "openfdat", Path: path, Err: errors.New("too many levels of symbolic links")}
	}
}

// ⛔ `gc --apply` left every job's host directory behind on a redirected state
// directory, both on 6.0.0 and on the build that found it.
func TestRemoveInsideRemovesATreeTheFirstRemovalCannotOpen(t *testing.T) {
	failingRemoveTree(t)
	root := t.TempDir()
	job := filepath.Join(root, "jobs", "0123456789abcdef")
	for _, f := range []string{"owner.lock", "result.json", "work/deep/a.txt"} {
		p := filepath.Join(job, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveInside(root, job); err != nil {
		t.Fatalf("a tree the first removal could not open was not removed: %v", err)
	}
	if _, err := os.Lstat(job); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the job directory is still on disk: %v", err)
	}
}

// ⛔ The fallback removes a link and never what it points at.
func TestTheFallbackRemovesALinkAndKeepsItsTarget(t *testing.T) {
	failingRemoveTree(t)
	root := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	job := filepath.Join(root, "job")
	if err := os.MkdirAll(job, 0o700); err != nil {
		t.Fatal(err)
	}
	linkDirForTest(t, outside, filepath.Join(job, "link"))
	if err := RemoveInside(root, job); err != nil {
		t.Fatalf("a tree holding a link was not removed: %v", err)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("the removal followed the link and took what it points at: %v", err)
	}
}

// linkDirForTest links link to the directory target, as a symbolic link where
// this process may make one and as a junction otherwise.
func linkDirForTest(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err == nil {
		return
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if err == nil {
			return
		}
		t.Logf("mklink /J: %v: %s", err, out)
	}
	t.Skip("no link to a directory can be made here")
}
