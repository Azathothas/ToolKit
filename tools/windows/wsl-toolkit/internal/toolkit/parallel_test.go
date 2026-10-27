// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/parallelread"
)

// parallelCase lets a case run beside the other parallel cases of this package.
//
// ⭐ THE PACKAGE'S CASES TOOK 150 s ONE AFTER ANOTHER, and most of a case's time
// is a shell or a child it waits on. TOOL-40.
//
// ⛔ ONLY A CASE THAT SETS NOTHING ANOTHER CASE READS. No t.Setenv, no working
// directory, no variable of this package or of another, through any helper it
// calls. TestNoParallelCaseSetsWhatAnotherReads reads that from the source.
//
// ⚠ AND NO BOUND A LOADED HOST CAN BREAK. A child that must not stall for 2 s, or
// a watch that must hold within 300 ms, stays serial. Beside 30 other cases a
// fork in Git for Windows' sh can take that long.
func parallelCase(t *testing.T) {
	t.Helper()
	t.Parallel()
}

// guardedShared are package variables a parallel case may set, because each
// guards itself. ⛔ A name is added here with the lock that guards it.
var guardedShared = map[string]bool{
	// gitShell holds its own mutex, and grantsShell takes it.
	"gitShell": true,
}

// TestNoParallelCaseSetsWhatAnotherReads reads every case of this package that
// runs in parallel, and every function it calls by name, with
// internal/parallelread.
func TestNoParallelCaseSetsWhatAnotherReads(t *testing.T) {
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range names {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	found, parallel := parallelread.SharedWrites(fset, files, guardedShared)
	if parallel < 60 {
		t.Fatalf("%d parallel case(s) were read, and this package has about 70, so the reading missed them", parallel)
	}
	for _, f := range found {
		t.Error(f)
	}
}
