// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
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
// runs in parallel, and every function it calls by name.
//
// ⛔ A RACE IS NOT A FAILURE YOU SEE. A parallel case that swaps a package
// variable passes alone, and fails another case now and then. So this is read
// from the source, not from a run.
//
// ⚠ A METHOD IS FOLLOWED BY ITS NAME, to every method of that name. That reads
// more than a call can reach, and a finding there is moved out of the case.
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
	found, parallel := sharedWrites(fset, files, guardedShared)
	if parallel < 60 {
		t.Fatalf("%d parallel case(s) were read, and this package has about 70, so the reading missed them", parallel)
	}
	for _, f := range found {
		t.Error(f)
	}
}

// TestTheParallelReadingFindsWhatACaseSets holds sharedWrites to a package
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
	found, parallel := sharedWrites(fset, []*ast.File{f}, map[string]bool{"guarded": true})
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

// sharedFacts is what one function's own body does.
type sharedFacts struct {
	parallel bool
	sets     []string
	calls    []string
}

// sharedWrites answers each finding as "TestX sets WHAT at FILE:LINE", and how
// many parallel cases it read.
func sharedWrites(fset *token.FileSet, files []*ast.File, allowed map[string]bool) ([]string, int) {
	vars := map[string]bool{}
	for _, f := range files {
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR {
				for _, s := range g.Specs {
					for _, n := range s.(*ast.ValueSpec).Names {
						vars[n.Name] = true
					}
				}
			}
		}
	}
	funcs := map[string][]*sharedFacts{}
	var tests []string
	for _, f := range files {
		imported := map[string]bool{}
		for _, im := range f.Imports {
			name := strings.Trim(im.Path.Value, `"`)
			name = name[strings.LastIndex(name, "/")+1:]
			if im.Name != nil {
				name = im.Name.Name
			}
			imported[name] = true
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			key := fd.Name.Name
			if fd.Recv != nil {
				key = "." + key
			} else if strings.HasPrefix(key, "Test") && key != "TestMain" {
				tests = append(tests, key)
			}
			funcs[key] = append(funcs[key], readFacts(fset, fd, vars, imported, allowed))
		}
	}
	sort.Strings(tests)
	var found []string
	parallel := 0
	for _, name := range tests {
		if !runsInParallel(name, funcs) {
			continue
		}
		parallel++
		seen := map[string]bool{}
		var walk func(key string)
		walk = func(key string) {
			if seen[key] {
				return
			}
			seen[key] = true
			for _, facts := range funcs[key] {
				for _, s := range facts.sets {
					found = append(found, name+" sets "+s)
				}
				for _, c := range facts.calls {
					walk(c)
				}
			}
		}
		walk(name)
	}
	return found, parallel
}

// runsInParallel is whether a case calls t.Parallel, or a helper of this package
// that does.
func runsInParallel(name string, funcs map[string][]*sharedFacts) bool {
	for _, facts := range funcs[name] {
		if facts.parallel {
			return true
		}
		for _, c := range facts.calls {
			for _, helper := range funcs[c] {
				if helper.parallel && !strings.HasPrefix(c, ".") {
					return true
				}
			}
		}
	}
	return false
}

// readFacts reads one function: what it sets that another case could read, and
// what it calls by name.
func readFacts(fset *token.FileSet, fd *ast.FuncDecl, vars, imported, allowed map[string]bool) *sharedFacts {
	facts := &sharedFacts{}
	locals := map[string]bool{}
	at := func(n ast.Node) string {
		p := fset.Position(n.Pos())
		return filepath.Base(p.Filename) + ":" + strconv.Itoa(p.Line)
	}
	set := func(lhs ast.Expr) {
		if sel, ok := lhs.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && imported[id.Name] && !locals[id.Name] && !vars[id.Name] {
				facts.sets = append(facts.sets, id.Name+"."+sel.Sel.Name+" at "+at(lhs))
				return
			}
		}
		e := lhs
		for {
			switch x := e.(type) {
			case *ast.SelectorExpr:
				e = x.X
				continue
			case *ast.IndexExpr:
				e = x.X
				continue
			case *ast.StarExpr:
				e = x.X
				continue
			case *ast.ParenExpr:
				e = x.X
				continue
			}
			break
		}
		if id, ok := e.(*ast.Ident); ok && vars[id.Name] && !locals[id.Name] && !allowed[id.Name] {
			facts.sets = append(facts.sets, id.Name+" at "+at(lhs))
		}
	}
	ast.Inspect(fd, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Field:
			for _, name := range x.Names {
				locals[name.Name] = true
			}
		case *ast.ValueSpec:
			for _, name := range x.Names {
				locals[name.Name] = true
			}
		case *ast.RangeStmt:
			if x.Tok == token.DEFINE {
				for _, e := range []ast.Expr{x.Key, x.Value} {
					if id, ok := e.(*ast.Ident); ok {
						locals[id.Name] = true
					}
				}
			}
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, l := range x.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						locals[id.Name] = true
					}
				}
				return true
			}
			for _, l := range x.Lhs {
				set(l)
			}
		case *ast.IncDecStmt:
			set(x.X)
		case *ast.CallExpr:
			switch fn := x.Fun.(type) {
			case *ast.Ident:
				if fn.Name == "chdir" {
					facts.sets = append(facts.sets, "chdir at "+at(x))
				}
				facts.calls = append(facts.calls, fn.Name)
			case *ast.SelectorExpr:
				switch fn.Sel.Name {
				case "Parallel":
					facts.parallel = true
				case "Setenv", "Unsetenv", "Clearenv", "Chdir":
					facts.sets = append(facts.sets, fn.Sel.Name+" at "+at(x))
				}
				facts.calls = append(facts.calls, "."+fn.Sel.Name)
			}
		}
		return true
	})
	return facts
}
