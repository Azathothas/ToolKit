// SPDX-License-Identifier: 0BSD

package forksafe

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestWriteFileWaitsForAFork holds the lock a fork holds, and finds the write
// waiting for it. WSL-205.
func TestWriteFileWaitsForAFork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stand-in")
	syscall.ForkLock.Lock()
	done := make(chan error, 1)
	go func() { done <- WriteFile(path, []byte("#!/bin/sh\n"), 0o755) }()
	select {
	case err := <-done:
		syscall.ForkLock.Unlock()
		t.Fatalf("the file was written while a fork held the lock: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if _, err := os.Stat(path); err == nil {
		syscall.ForkLock.Unlock()
		t.Fatal("the file was opened while a fork held the lock")
	}
	syscall.ForkLock.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "#!/bin/sh\n" {
		t.Fatalf("the file holds %q, %v", got, err)
	}
}

// TestEveryTestFileWritesThroughForkSafe reads each test file of this module for
// a call to os.WriteFile. ⛔ Any file a case writes may be a stand-in another
// case runs, so none is written outside the lock. WSL-205.
func TestEveryTestFileWritesThroughForkSafe(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("the module root is not two directories up: %v", err)
	}
	read := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			return err
		}
		read++
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "WriteFile" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "os" {
				t.Errorf("%s writes with os.WriteFile; write through forksafe.WriteFile", set.Position(sel.Pos()))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if read < 100 {
		t.Errorf("read %d test files; the walk no longer reaches the module", read)
	}
}
