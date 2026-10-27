// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"sync"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/parallelread"
)

// parallelCase lets a case run beside the other parallel cases of this package.
//
// ⭐ THE PACKAGE'S CASES TOOK 150 s ONE AFTER ANOTHER, and most of a case's time
// is a shell or a child it waits on. TOOL-40.
//
// ⛔ ONLY A CASE THAT SETS NOTHING ANOTHER CASE READS. No t.Setenv, no working
// directory, no variable of this package or of another.
// TestNoParallelCaseSetsWhatAnotherReads reads that from the source.
//
// ⚠ AND NO BOUND A LOADED HOST CAN BREAK. A child that must not stall for 2 s, or
// a watch that must hold within 300 ms, stays serial. Beside 30 other cases a
// fork in Git for Windows' sh can take that long.
func parallelCase(t *testing.T) {
	t.Helper()
	t.Parallel()
}

// guardedShared are package variables a parallel case may set, because each
// guards itself, and methods a parallel case may call on one, because they only
// read. ⛔ A name is added here with the lock that guards it, or the reason.
var guardedShared = map[string]bool{
	// gitShell holds its own mutex, and grantsShell takes it.
	"gitShell": true,
	// loaded is set once, under its own sync.Once.
	"loaded": true,
	// Each tree is an embed.FS, or the copy a mutation row's binary made before
	// any case ran. Neither changes when it is read.
	"adapterTree.ReadFile": true,
	"adapterTree.ReadDir":  true,
	"shippedTree.ReadFile": true,
	"shippedTree.ReadDir":  true,
}

// loaded is this package's source, type-checked once for every case that reads
// it. ⚠ Set under its own sync.Once.
var loaded struct {
	once sync.Once
	p    *parallelread.Package
	err  error
}

// thisPackage answers this package's source, type-checked.
func thisPackage(t *testing.T) *parallelread.Package {
	t.Helper()
	loaded.once.Do(func() { loaded.p, loaded.err = parallelread.Load(".") })
	if loaded.err != nil {
		t.Fatal(loaded.err)
	}
	return loaded.p
}

// TestNoParallelCaseSetsWhatAnotherReads reads every case of this package that
// runs in parallel, and all the code it reaches, with internal/parallelread.
//
// ⚠ WHAT IT DOES NOT SEE is named in parallelread_test.go and the README. It
// misses a write through a copy of a package variable's map, slice or pointer.
// It misses a write through what a call returned. Nor does it see a value
// method of another package that writes through what the value holds, or
// another package's own setters. Files this host does not build are read on
// the other.
func TestNoParallelCaseSetsWhatAnotherReads(t *testing.T) {
	parallelCase(t)
	found, parallel := thisPackage(t).SharedWrites(guardedShared)
	if parallel < 60 {
		t.Fatalf("%d parallel case(s) were read, and this package has about 70, so the reading missed them", parallel)
	}
	for _, f := range found {
		t.Error(f)
	}
}
