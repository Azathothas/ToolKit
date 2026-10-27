// Package parallelread reads a package's source for the cases that run in
// parallel, and names what each sets that another case could read. TOOL-40.
//
// ⛔ A RACE IS NOT A FAILURE YOU SEE. A parallel case that swaps a package
// variable passes alone, and fails another case now and then. So this reads the
// source, not a run.
//
// ⭐ IT READS go/types OBJECTS, NOT NAMES. A local that shadows a package
// variable is its own object. So is a field of the same name, and an import
// renamed or ending in /v2.
//
// ⛔ A CASE REACHES, TO ANY DEPTH: each function or method of this package it
// names, called or passed as a value, generic or not. Also every method of
// this package with the name of an interface method it calls. Also every
// method of a type of this package it names, which another package may call.
// Also each expression ever stored in a package variable it reads.
//
// ⛔ A CASE SETS: a package variable of any package it assigns, increments,
// takes the address of, or hands to delete, clear, copy or append. Also one it
// calls a method on that may change it, and the environment or working
// directory.
//
// ⚠ WHAT IT DOES NOT SEE. A map, slice or pointer a package variable holds,
// passed to a function or copied into a local and written through that copy.
// A write through what a call returned. A value method of another package that
// writes through what its value holds. A function of another package that
// changes that package's own state, as log.SetOutput. The files this host
// does not build, which a host of the other kind reads.
//
// ⚠ ONLY A PACKAGE'S TEST FILES CALL THIS, and the executable never imports it.
//
// SPDX-License-Identifier: 0BSD

package parallelread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Package is one package's files and its own test files, type-checked as
// `go test` builds them on this host.
type Package struct {
	Fset  *token.FileSet
	Files []*ast.File
	Info  *types.Info
	Types *types.Package

	mu sync.Mutex
	r  *reader
}

// Load type-checks the package in dir with its own test files. Each import is
// read from the export data `go list -export` names.
//
// ⛔ A PACKAGE WITH AN EXTERNAL TEST PACKAGE IS REFUSED. Its cases run in the
// same binary, and this reading would not see them.
func Load(dir string) (*Package, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("no go on PATH, so the package cannot be type-checked: %w", err)
	}
	var meta struct {
		ImportPath, Dir                              string
		GoFiles, CgoFiles, TestGoFiles, XTestGoFiles []string
		Imports, TestImports                         []string
	}
	out, err := goList(goBin, dir, "-json=ImportPath,Dir,GoFiles,CgoFiles,TestGoFiles,XTestGoFiles,Imports,TestImports", ".")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(out, &meta); err != nil {
		return nil, fmt.Errorf("go list answered what is not one package: %w", err)
	}
	if len(meta.XTestGoFiles) > 0 || len(meta.CgoFiles) > 0 {
		return nil, fmt.Errorf("%s has an external test package or cgo files, which this reading does not see", meta.ImportPath)
	}
	seen := map[string]bool{"unsafe": true, "C": true}
	var imports []string
	for _, p := range append(meta.Imports, meta.TestImports...) {
		if !seen[p] {
			seen[p] = true
			imports = append(imports, p)
		}
	}
	exports := map[string]string{}
	if len(imports) > 0 {
		out, err := goList(goBin, dir, append([]string{"-export", "-f", "{{.ImportPath}}\t{{.Export}}"}, imports...)...)
		if err != nil {
			return nil, err
		}
		for _, ln := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
			if p, file, ok := strings.Cut(ln, "\t"); ok {
				exports[p] = file
			}
		}
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range append(meta.GoFiles, meta.TestGoFiles...) {
		f, err := parser.ParseFile(fset, filepath.Join(meta.Dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	info := &types.Info{
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
		Types:      map[ast.Expr]types.TypeAndValue{},
	}
	var errs []error
	conf := types.Config{
		Importer: importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
			file := exports[path]
			if file == "" {
				return nil, fmt.Errorf("go list named no export data for %s", path)
			}
			return os.Open(file)
		}),
		Error: func(err error) { errs = append(errs, err) },
	}
	pkg, _ := conf.Check(meta.ImportPath, fset, files, info)
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s does not type-check, so a reading of it would be partial: %v", meta.ImportPath, errs[0])
	}
	return &Package{Fset: fset, Files: files, Info: info, Types: pkg}, nil
}

// goList runs `go list` in dir and answers its standard output.
func goList(goBin, dir string, args ...string) ([]byte, error) {
	cmd := exec.Command(goBin, append([]string{"list"}, args...)...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// facts is what one unit of code does: a function's declaration, or an
// expression stored in a package variable.
type facts struct {
	parallel bool
	sets     []shared
	// next is every unit the code reaches, and calls the ones it runs.
	next, calls []ast.Node
	// uses is every object the code names.
	uses []types.Object
}

// shared is one thing a unit sets: a variable, or a method called on one, or
// a call that changes the process. at is its file and line.
type shared struct {
	what, method, at string
}

// reader holds one package's indexes.
type reader struct {
	p *Package
	// decls is every function and method declared with a body.
	decls map[*types.Func]*ast.FuncDecl
	// byName is every method of this package, by its name, for a call through
	// an interface.
	byName map[string][]*ast.FuncDecl
	// methods is every method of each type this package declares.
	methods map[*types.TypeName][]*ast.FuncDecl
	// stored is every expression put in a package variable of this package:
	// its initializer, and each right-hand side assigned to it.
	stored map[*types.Var][]ast.Node
	memo   map[ast.Node]*facts
}

func newReader(p *Package) *reader {
	r := &reader{p: p, decls: map[*types.Func]*ast.FuncDecl{}, byName: map[string][]*ast.FuncDecl{},
		methods: map[*types.TypeName][]*ast.FuncDecl{}, stored: map[*types.Var][]ast.Node{}, memo: map[ast.Node]*facts{}}
	for _, f := range p.Files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				fn, ok := p.Info.Defs[d.Name].(*types.Func)
				if !ok || d.Body == nil {
					continue
				}
				r.decls[fn] = d
				if d.Recv != nil {
					r.byName[fn.Name()] = append(r.byName[fn.Name()], d)
					if tn := receiverType(fn); tn != nil {
						r.methods[tn] = append(r.methods[tn], d)
					}
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					vs, ok := s.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, n := range vs.Names {
						v, ok := p.Info.Defs[n].(*types.Var)
						if !ok || v == nil {
							continue
						}
						switch {
						case len(vs.Values) == len(vs.Names):
							r.stored[v] = append(r.stored[v], vs.Values[i])
						case len(vs.Values) > 0:
							for _, e := range vs.Values {
								r.stored[v] = append(r.stored[v], e)
							}
						}
					}
				}
			}
		}
	}
	// ⭐ A FUNCTION ASSIGNED TO A VARIABLE ANYWHERE IS FOLLOWED FROM IT.
	for _, f := range p.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || as.Tok == token.DEFINE {
				return true
			}
			for i, l := range as.Lhs {
				id, ok := ast.Unparen(l).(*ast.Ident)
				if !ok {
					continue
				}
				v, ok := p.Info.Uses[id].(*types.Var)
				if !ok || !r.mine(v) {
					continue
				}
				if len(as.Rhs) == len(as.Lhs) {
					r.stored[v] = append(r.stored[v], as.Rhs[i])
				} else {
					for _, e := range as.Rhs {
						r.stored[v] = append(r.stored[v], e)
					}
				}
			}
			return true
		})
	}
	return r
}

// receiverType is the type a method is declared on.
func receiverType(fn *types.Func) *types.TypeName {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return nil
	}
	t := sig.Recv().Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		return named.Origin().Obj()
	}
	return nil
}

// packageLevel is whether v is a variable of some package's own scope.
func packageLevel(v *types.Var) bool {
	return v != nil && v.Pkg() != nil && v.Parent() == v.Pkg().Scope()
}

// mine is whether v is a package-level variable of the package being read.
func (r *reader) mine(v *types.Var) bool {
	return packageLevel(v) && v.Pkg() == r.p.Types
}

// root is the package-level variable an expression is part of, through
// fields, indexes, slices and pointers. ⚠ Not through a call.
func (r *reader) root(e ast.Expr) *types.Var {
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		case *ast.SliceExpr:
			e = x.X
		case *ast.SelectorExpr:
			if sel, ok := r.p.Info.Selections[x]; ok {
				if sel.Kind() != types.FieldVal {
					return nil
				}
				e = x.X
				continue
			}
			// A qualified identifier: another package's own variable.
			v, _ := r.p.Info.Uses[x.Sel].(*types.Var)
			if !packageLevel(v) {
				return nil
			}
			return v
		case *ast.Ident:
			v, _ := r.p.Info.Uses[x].(*types.Var)
			if !packageLevel(v) {
				return nil
			}
			return v
		default:
			return nil
		}
	}
}

// named is how a finding names a variable: bare for this package's own, and
// qualified for another's.
func (r *reader) named(v *types.Var) string {
	if v.Pkg() == r.p.Types {
		return v.Name()
	}
	return v.Pkg().Name() + "." + v.Name()
}

func (r *reader) at(n ast.Node) string {
	p := r.p.Fset.Position(n.Pos())
	return filepath.Base(p.Filename) + ":" + strconv.Itoa(p.Line)
}

// processState names the calls of other packages that change what every case
// of the process reads: its environment, and its working directory.
var processState = map[string]bool{"Setenv": true, "Unsetenv": true, "Clearenv": true, "Chdir": true}

// facts reads one unit's own code: what it sets, whether it calls t.Parallel,
// and what code of this package it reaches.
func (r *reader) facts(n ast.Node) *facts {
	if f, ok := r.memo[n]; ok {
		return f
	}
	f := &facts{}
	r.memo[n] = f
	info := r.p.Info
	set := func(e ast.Expr, at ast.Node) {
		if v := r.root(e); v != nil {
			f.sets = append(f.sets, shared{what: r.named(v), at: r.at(at)})
		}
	}
	follow := func(obj types.Object, into *[]ast.Node) {
		switch o := obj.(type) {
		case *types.Func:
			o = o.Origin()
			if d, ok := r.decls[o]; ok {
				*into = append(*into, d)
				return
			}
			// ⚠ A CALL THROUGH AN INTERFACE reaches every method of this
			// package with that name.
			if sig, ok := o.Type().(*types.Signature); ok && sig.Recv() != nil && types.IsInterface(sig.Recv().Type()) {
				for _, d := range r.byName[o.Name()] {
					*into = append(*into, d)
				}
			}
		case *types.Var:
			if r.mine(o) {
				*into = append(*into, r.stored[o]...)
			}
		case *types.TypeName:
			// ⚠ A TYPE THE CODE NAMES BRINGS EVERY METHOD OF IT. Another
			// package may call one through an interface, as String or Write.
			if o.Pkg() == r.p.Types {
				for _, d := range r.methods[o] {
					*into = append(*into, d)
				}
			}
		}
	}
	ast.Inspect(n, func(node ast.Node) bool {
		switch x := node.(type) {
		case *ast.Ident:
			if obj := info.Uses[x]; obj != nil {
				f.uses = append(f.uses, obj)
				follow(obj, &f.next)
			}
		case *ast.AssignStmt:
			if x.Tok != token.DEFINE {
				for _, l := range x.Lhs {
					set(l, l)
				}
			}
		case *ast.RangeStmt:
			if x.Tok == token.ASSIGN {
				for _, e := range []ast.Expr{x.Key, x.Value} {
					if e != nil {
						set(e, e)
					}
				}
			}
		case *ast.IncDecStmt:
			set(x.X, x.X)
		case *ast.UnaryExpr:
			// ⚠ AN ADDRESS TAKEN CAN BE WRITTEN THROUGH, wherever it goes.
			if x.Op == token.AND {
				set(x.X, x)
			}
		case *ast.SelectorExpr:
			sel, ok := info.Selections[x]
			if !ok || sel.Kind() != types.MethodVal {
				return true
			}
			if fn, ok := sel.Obj().(*types.Func); ok && fn.Pkg() != nil && fn.Pkg().Path() == "testing" && fn.Name() == "Parallel" {
				f.parallel = true
			}
			// ⛔ A METHOD CALLED ON A PACKAGE VARIABLE SETS IT, unless it is
			// one that only reads. A method value taken from it is the same.
			if v := r.root(x.X); v != nil && !r.readsOnly(sel) {
				f.sets = append(f.sets, shared{what: r.named(v), method: x.Sel.Name, at: r.at(x)})
			}
		case *ast.CallExpr:
			if obj := callee(info, x); obj != nil {
				if _, isType := obj.(*types.TypeName); !isType {
					follow(obj, &f.calls)
				}
			}
			switch fn := callee(info, x).(type) {
			case *types.Builtin:
				switch fn.Name() {
				case "delete", "clear", "copy", "append":
					if len(x.Args) > 0 {
						set(x.Args[0], x)
					}
				}
			case *types.Func:
				if fn.Pkg() != nil && fn.Pkg() != r.p.Types && processState[fn.Name()] {
					f.sets = append(f.sets, shared{what: fn.Name(), at: r.at(x)})
				}
			}
		}
		return true
	})
	return f
}

// callee is the object a call names, when it names one.
func callee(info *types.Info, call *ast.CallExpr) types.Object {
	fun := ast.Unparen(call.Fun)
	switch x := fun.(type) {
	case *ast.IndexExpr:
		fun = x.X
	case *ast.IndexListExpr:
		fun = x.X
	}
	switch x := fun.(type) {
	case *ast.Ident:
		return info.Uses[x]
	case *ast.SelectorExpr:
		return info.Uses[x.Sel]
	}
	return nil
}

// readOnly names the methods of other packages' types that leave their value
// as it was, and are safe beside each other. A type is named by its import
// path and name. ⛔ A type is added here with its documentation's word.
var readOnly = map[string]func(method string) bool{
	// "A Regexp is safe for concurrent use by multiple goroutines, except for
	// configuration methods, such as Longest."
	"regexp.Regexp": func(m string) bool { return m != "Longest" },
	// An Encoding is set when it is made, and its methods only read it.
	"encoding/base64.Encoding": func(string) bool { return true },
	// A lazy DLL and procedure load once, under their own lock.
	"syscall.LazyDLL":  func(string) bool { return true },
	"syscall.LazyProc": func(string) bool { return true },
	// "Clients are safe for concurrent use by multiple goroutines."
	"net/http.Client": func(m string) bool { return m == "Do" || m == "Get" || m == "Head" || m == "Post" || m == "PostForm" },
	// An atomic value's Load reads it, and nothing else of it does.
	"sync/atomic.Bool":    isLoad,
	"sync/atomic.Int32":   isLoad,
	"sync/atomic.Int64":   isLoad,
	"sync/atomic.Uint32":  isLoad,
	"sync/atomic.Uint64":  isLoad,
	"sync/atomic.Value":   isLoad,
	"sync/atomic.Pointer": isLoad,
}

func isLoad(m string) bool { return m == "Load" }

// readsOnly is whether a method called on a package variable leaves it as it
// was. That is a method readOnly names, or a value receiver of another
// package's type that is not an interface.
//
// ⚠ A METHOD OF THIS PACKAGE IS NEVER READ AS A READ. Its body writes through
// its receiver, which this reading does not tie to the variable.
func (r *reader) readsOnly(sel *types.Selection) bool {
	fn, ok := sel.Obj().(*types.Func)
	if !ok || fn.Pkg() == r.p.Types {
		return false
	}
	recv := sel.Recv()
	if ptr, ok := recv.(*types.Pointer); ok {
		recv = ptr.Elem()
	}
	if named, ok := recv.(*types.Named); ok {
		obj := named.Origin().Obj()
		if obj.Pkg() != nil {
			if reads, ok := readOnly[obj.Pkg().Path()+"."+obj.Name()]; ok && reads(fn.Name()) {
				return true
			}
		}
	}
	sig := fn.Type().(*types.Signature)
	if sig.Recv() == nil || types.IsInterface(sig.Recv().Type()) {
		return false
	}
	_, ptr := sig.Recv().Type().(*types.Pointer)
	return !ptr
}

// reader answers the package's reader, made once and held under its lock.
func (p *Package) reader() (*reader, func()) {
	p.mu.Lock()
	if p.r == nil {
		p.r = newReader(p)
	}
	return p.r, p.mu.Unlock
}

// Runs answers every object named by the code that runs when n is evaluated.
// That is n itself, and each function of this package it calls, to any depth.
//
// ⚠ ONLY WHAT IS CALLED. A function n names and does not call, or a type whose
// methods n does not call, runs later or never. A call through an interface
// runs every method of this package with that name. A call through a variable
// runs everything ever stored in it.
func (p *Package) Runs(n ast.Node) map[types.Object]bool {
	r, unlock := p.reader()
	defer unlock()
	out := map[types.Object]bool{}
	seen := map[ast.Node]bool{n: true}
	order := []ast.Node{n}
	for i := 0; i < len(order); i++ {
		f := r.facts(order[i])
		for _, obj := range f.uses {
			out[obj] = true
		}
		for _, c := range f.calls {
			if !seen[c] {
				seen[c] = true
				order = append(order, c)
			}
		}
	}
	return out
}

// SharedWrites answers each finding as "TestX sets WHAT at FILE:LINE", and how
// many parallel cases it read. allowed names what a parallel case may set. A
// variable is the package's own name, or pkg.Name for another package's. One
// method called on it is name.Method.
func (p *Package) SharedWrites(allowed map[string]bool) ([]string, int) {
	r, unlock := p.reader()
	defer unlock()
	type test struct {
		name string
		decl *ast.FuncDecl
	}
	var tests []test
	for fn, d := range r.decls {
		if d.Recv == nil && strings.HasPrefix(fn.Name(), "Test") && fn.Name() != "TestMain" {
			tests = append(tests, test{fn.Name(), d})
		}
	}
	sort.Slice(tests, func(a, b int) bool { return tests[a].name < tests[b].name })
	var found []string
	parallel := 0
	for _, tc := range tests {
		reached := r.reach(tc.decl)
		isParallel := false
		for _, n := range reached {
			if r.facts(n).parallel {
				isParallel = true
				break
			}
		}
		if !isParallel {
			continue
		}
		parallel++
		for _, n := range reached {
			for _, s := range r.facts(n).sets {
				what := s.what
				if s.method != "" {
					what += "." + s.method
				}
				if !allowed[s.what] && !allowed[what] {
					found = append(found, tc.name+" sets "+what+" at "+s.at)
				}
			}
		}
	}
	return found, parallel
}

// reach is every unit a case reaches, to any depth, in the order first met.
func (r *reader) reach(from ast.Node) []ast.Node {
	seen := map[ast.Node]bool{from: true}
	order := []ast.Node{from}
	for i := 0; i < len(order); i++ {
		for _, n := range r.facts(order[i]).next {
			if !seen[n] {
				seen[n] = true
				order = append(order, n)
			}
		}
	}
	return order
}
