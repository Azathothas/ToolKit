// SPDX-License-Identifier: 0BSD

package parallelread

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestTheParallelReadingFindsWhatACaseSets holds SharedWrites to a package
// written to break each rule once.
func TestTheParallelReadingFindsWhatACaseSets(t *testing.T) {
	const src = `package p

import (
	"os"
	"sync"
	"testing"
)

var seam = 1

var guarded struct {
	mu sync.Mutex
	v  int
}

func helper(t *testing.T) { t.Setenv("X", "1") }
func swap()               { seam = 2 }
func par(t *testing.T)    { t.Parallel() }
func move()               { chdir(nil, "") }
func chdir(t *testing.T, dir string) func() { return nil }

func TestEnv(t *testing.T)    { par(t); helper(t) }
func TestSwap(t *testing.T)   { t.Parallel(); swap() }
func TestStdout(t *testing.T) { par(t); os.Stdout = nil }
func TestMove(t *testing.T)   { par(t); move() }
func TestClean(t *testing.T) {
	t.Parallel()
	seam := 3
	seam++
	guarded.v = seam
}
func TestSerial(t *testing.T) { helper(t); swap() }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	found, parallel := SharedWrites(fset, []*ast.File{f}, map[string]bool{"guarded": true})
	if parallel != 5 {
		t.Errorf("%d parallel case(s) were read, want 5", parallel)
	}
	got := strings.Join(found, "\n")
	for _, want := range []string{"TestEnv sets Setenv", "TestSwap sets seam", "TestStdout sets os.Stdout", "TestMove sets chdir"} {
		if !strings.Contains(got, want) {
			t.Errorf("the reading did not find %q:\n%s", want, got)
		}
	}
	for _, clean := range []string{"TestClean", "TestSerial"} {
		if strings.Contains(got, clean+" ") {
			t.Errorf("the reading named %s, which sets nothing shared or is not parallel:\n%s", clean, got)
		}
	}
}
