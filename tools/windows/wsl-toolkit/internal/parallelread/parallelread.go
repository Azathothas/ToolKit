// Package parallelread reads a package's source for the cases that run in
// parallel, and names what each sets that another case could read. TOOL-40.
//
// ⛔ A RACE IS NOT A FAILURE YOU SEE. A parallel case that swaps a package
// variable passes alone, and fails another case now and then. So this reads the
// source, not a run.
//
// ⚠ A METHOD IS FOLLOWED BY ITS NAME, to every method of that name. That reads
// more than a call can reach, and a finding there is moved out of the case.
//
// ⚠ ONLY A PACKAGE'S TEST FILES CALL THIS, and the executable never imports it.
//
// SPDX-License-Identifier: 0BSD

package parallelread

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// sharedFacts is what one function's own body does.
type sharedFacts struct {
	parallel bool
	sets     []string
	calls    []string
}

// SharedWrites answers each finding as "TestX sets WHAT at FILE:LINE", and how
// many parallel cases it read.
func SharedWrites(fset *token.FileSet, files []*ast.File, allowed map[string]bool) ([]string, int) {
	vars := map[string]bool{}
	for _, f := range files {
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR {
				for _, s := range g.Specs {
					// ⚠ NOT THE BLANK IDENTIFIER. `var _ = f()` names no
					// variable, and every `_ = x` would read as setting it.
					for _, n := range s.(*ast.ValueSpec).Names {
						if n.Name != "_" {
							vars[n.Name] = true
						}
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
