// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// -- a .git the caller did not write never runs a program ---------------------

// plantedHook is a git work tree whose own configuration names a core.fsmonitor
// hook, and the file that hook writes when git runs it.
func plantedHook(t *testing.T) (root, marker string) {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git on PATH")
	}
	root = t.TempDir()
	marker = filepath.Join(t.TempDir(), "fsmonitor-ran")
	if out, err := exec.Command(git, "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	hook := filepath.Join(t.TempDir(), "hook.sh")
	body := "#!/bin/sh\nprintf ran > '" + filepath.ToSlash(marker) + "'\n"
	if err := os.WriteFile(hook, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(git, "-C", root, "config", "core.fsmonitor", filepath.ToSlash(hook)).CombinedOutput(); err != nil {
		t.Fatalf("git config: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(root, "run.sh"), []byte("echo hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(git, "-C", root, "add", "run.sh").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	_ = os.Remove(marker)
	return root, marker
}

// ⛔ A TREE A CONTAINER OR A HELPER CLIENT WROTE IS NEVER GIVEN TO GIT. A job
// that wrote /out/.git, or a sandboxed client that uploaded one, ran a program on
// this machine through the helper's copy. A safety review found it.
func TestGitIsNotRunInATreeAContainerOrAClientWrote(t *testing.T) {
	parallelCase(t)
	root, marker := plantedHook(t)
	var buf bytes.Buffer
	if _, err := writeWorkspaceTar(&buf, root, DefaultWorkspaceLimits(), WorkspaceFilter{Untrusted: true, Exec: map[string]bool{"run.sh": true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the copy of an untrusted tree ran the hook its .git names")
	}
	if mode := tarMode(t, buf.Bytes(), "run.sh"); mode != 0o755 {
		t.Errorf("run.sh, named executable by its archive, arrived as %o", mode)
	}
	if _, err := writeWorkspaceTar(&bytes.Buffer{}, root, DefaultWorkspaceLimits(), WorkspaceFilter{Untrusted: true, Gitignore: true}); err == nil {
		t.Error("--gitignore was applied to an untrusted tree, which asks git")
	}
}

// ⛔ THE CALLER'S OWN TREE IS READ WITH core.fsmonitor OFF, on every git call the
// copy makes, so a hook its configuration names does not run either.
func TestTheCallersOwnTreeIsReadWithTheHookOff(t *testing.T) {
	if runtime.GOOS != "windows" && os.Getuid() == 0 {
		t.Skip("git refuses a repository another user owns, and root owns everything here")
	}
	root, marker := plantedHook(t)
	if _, err := writeWorkspaceTar(&bytes.Buffer{}, root, DefaultWorkspaceLimits(), WorkspaceFilter{Gitignore: true}); err != nil {
		t.Fatal(err)
	}
	// ⚠ A hook git starts is not waited for by every version, so the file is
	// given a moment to appear before its absence is believed.
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a git call of the workspace copy ran the core.fsmonitor hook")
	}
}

// ⭐ An upload keeps the executable bits its archive carried, which is what the
// helper hands on in place of asking git.
func TestAnUploadRecordsTheFilesItsArchiveMarkedExecutable(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, mode := range map[string]int64{"bin/run": 0o755, "README": 0o644} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: mode, Size: 2, ModTime: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte("x\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := extractInto(&buf, t.TempDir(), DefaultWorkspaceLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exec["bin/run"] || got.Exec["README"] || len(got.Exec) != 1 {
		t.Fatalf("the upload recorded %v as executable; want bin/run alone", got.Exec)
	}
}

func tarMode(t *testing.T, archive []byte, name string) int64 {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(archive))
	for {
		h, err := tr.Next()
		if err != nil {
			t.Fatalf("%s is not in the archive: %v", name, err)
		}
		if h.Name == name {
			return h.Mode & 0o777
		}
	}
}
