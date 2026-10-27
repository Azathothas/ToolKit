// SPDX-License-Identifier: 0BSD

package toolkitsource

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/parallelread"
)

// toolkitDir is internal/toolkit, from this package's directory.
const toolkitDir = "../toolkit"

// guardedShared are package variables a parallel case may set, because each
// guards itself. It also names methods a parallel case may call on one, because
// they only read. ⛔ A name is added here with the lock that guards it, or the
// reason.
var guardedShared = map[string]bool{
	// gitShell holds its own mutex, and grantsShell takes it.
	"gitShell": true,
	// Each tree is an embed.FS, or the copy a mutation row's binary made before
	// any case ran. Neither changes when it is read.
	"adapterTree.ReadFile": true,
	"adapterTree.ReadDir":  true,
	"shippedTree.ReadFile": true,
	"shippedTree.ReadDir":  true,
}

// loaded is internal/toolkit's source, type-checked once for every case that
// reads it.
var loaded struct {
	once sync.Once
	p    *parallelread.Package
	err  error
}

// toolkitPackage answers internal/toolkit's source, type-checked.
func toolkitPackage(t *testing.T) *parallelread.Package {
	t.Helper()
	loaded.once.Do(func() { loaded.p, loaded.err = parallelread.Load(toolkitDir) })
	if loaded.err != nil {
		t.Fatal(loaded.err)
	}
	return loaded.p
}

// TestNoParallelCaseSetsWhatAnotherReads reads every case of internal/toolkit
// that runs in parallel, and all the code it reaches, with internal/parallelread.
//
// ⚠ WHAT IT DOES NOT SEE is named in parallelread_test.go and the README. It
// misses a write through a copy of a package variable's map, slice or pointer.
// It misses a write through what a call returned. Nor does it see a value
// method of another package that writes through what the value holds, or
// another package's own setters. Files this host does not build are read on
// the other.
func TestNoParallelCaseSetsWhatAnotherReads(t *testing.T) {
	t.Parallel()
	found, parallel := toolkitPackage(t).SharedWrites(guardedShared)
	if parallel < 60 {
		t.Fatalf("%d parallel case(s) were read, and internal/toolkit has about 70, so the reading missed them", parallel)
	}
	for _, f := range found {
		t.Error(f)
	}
}

// TestTheEmbedTablesMatchTheGoEmbedLines holds internal/toolkit's two tables to
// its go:embed lines, so a file embedded later is not one whose rows compile
// again.
//
// ⛔ NO PACKAGE VARIABLE IS SET FROM AN EMBEDDED ONE. Go may set it before
// takeMutationRow runs, and it would keep the old bytes. So no initializer may
// run code that names an embedded variable or a tree's interface, through any
// function it calls. An embed.FS is named only where its interface is set.
func TestTheEmbedTablesMatchTheGoEmbedLines(t *testing.T) {
	t.Parallel()
	for _, f := range readEmbeds(toolkitPackage(t), toolkitDir).found {
		t.Error(f)
	}
}

// embedDirective is one go:embed pattern of a package and the variable under
// it.
type embedDirective struct {
	pattern string
	v       *types.Var
	tree    bool
}

// embedReading is what TestTheEmbedTablesMatchTheGoEmbedLines reads of a
// package's source.
type embedReading struct {
	p          *parallelread.Package
	dir        string
	found      []string
	directives []embedDirective
	byPattern  map[string]*types.Var
	// iface is the interface variable each embed.FS is read through.
	iface map[*types.Var]*types.Var
}

func (e *embedReading) bad(format string, a ...any) {
	e.found = append(e.found, fmt.Sprintf(format, a...))
}

// readEmbeds reads a package's go:embed lines, its two tables, and what its
// package variables' initializers run.
//
// ⛔ EACH ENTRY IS THE DIRECTIVE'S OWN VARIABLE, read as a go/types object. An
// entry that pointed at another pattern's variable would hand a row's bytes to
// the wrong script.
//
// ⛔ ONE VARIABLE FOR EACH FILE. A second variable that embeds the same file,
// by its own pattern or a directory's, would keep the old bytes.
func readEmbeds(p *parallelread.Package, dir string) *embedReading {
	e := &embedReading{p: p, dir: dir, byPattern: map[string]*types.Var{}, iface: map[*types.Var]*types.Var{}}
	e.readDirectives()
	if len(e.directives) == 0 {
		e.bad("no go:embed line was read, so nothing was checked")
		return e
	}
	scripts, trees := e.table("embeddedBytes"), e.table("embeddedTrees")
	byFile := map[string]*types.Var{}
	for _, d := range e.directives {
		if was, ok := e.byPattern[d.pattern]; ok && was != d.v {
			e.bad("%s is embedded by both %s and %s", d.pattern, was.Name(), d.v.Name())
		}
		e.byPattern[d.pattern] = d.v
		for _, file := range e.files(d.pattern) {
			if was, ok := byFile[file]; ok && was != d.v {
				e.bad("%s is embedded by both %s and %s", file, was.Name(), d.v.Name())
			}
			byFile[file] = d.v
		}
		if !d.tree {
			if scripts[d.pattern] != d.v {
				e.bad("%s is embedded in %s, and the table does not hand a row of it there", d.pattern, d.v.Name())
			}
			continue
		}
		// ⚠ ONE PATH ELEMENT, because takeEmbedded finds a tree by a row's first.
		if strings.Contains(d.pattern, "/") {
			e.bad("the directory %s is more than one path element, and takeEmbedded finds a tree by the first", d.pattern)
		}
		e.iface[d.v] = e.interfaceOf(d.v)
		if e.iface[d.v] == nil || trees[d.pattern] != e.iface[d.v] {
			e.bad("the directory %s is embedded in %s, and the table does not hand a row of it there", d.pattern, d.v.Name())
		}
	}
	var extra []string
	for name := range scripts {
		if _, ok := e.byPattern[name]; !ok {
			extra = append(extra, name)
		}
	}
	for name := range trees {
		if _, ok := e.byPattern[name]; !ok {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		e.bad("the tables name %v, which no go:embed line embeds", extra)
	}
	e.initializers()
	return e
}

// readDirectives reads every go:embed line of the package's own files, from a
// declaration's comment or from a spec's in a var block.
//
// ⛔ A []byte AND AN embed.FS ARE THE TWO KINDS A ROW CAN BE HANDED. A string
// is refused, because its table entry would have to be a *string.
func (e *embedReading) readDirectives() {
	for _, f := range e.p.Files {
		if strings.HasSuffix(e.p.Fset.Position(f.Pos()).Filename, "_test.go") {
			continue
		}
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.VAR {
				continue
			}
			for _, s := range g.Specs {
				spec := s.(*ast.ValueSpec)
				doc := spec.Doc
				if doc == nil && !g.Lparen.IsValid() {
					doc = g.Doc
				}
				if doc == nil {
					continue
				}
				for _, c := range doc.List {
					rest, ok := strings.CutPrefix(c.Text, "//go:embed ")
					if !ok {
						continue
					}
					v, _ := e.p.Info.Defs[spec.Names[0]].(*types.Var)
					if len(spec.Names) != 1 || v == nil {
						e.bad("a go:embed line stands over %d names", len(spec.Names))
						continue
					}
					tree := false
					switch typ := v.Type().(type) {
					case *types.Named:
						tree = typ.Obj().Pkg() != nil && typ.Obj().Pkg().Path() == "embed" && typ.Obj().Name() == "FS"
						if !tree {
							e.bad("%s embeds into a %s, which a mutation row cannot be handed", v.Name(), typ)
						}
					case *types.Slice:
						if !types.Identical(typ, types.NewSlice(types.Typ[types.Byte])) {
							e.bad("%s embeds into a %s, which a mutation row cannot be handed", v.Name(), typ)
						}
					default:
						e.bad("%s embeds into a %s, which a mutation row cannot be handed", v.Name(), typ)
					}
					for _, pattern := range strings.Fields(rest) {
						if u, err := strconv.Unquote(pattern); err == nil {
							pattern = u
						}
						e.directives = append(e.directives, embedDirective{pattern: strings.TrimPrefix(pattern, "all:"), v: v, tree: tree})
					}
				}
			}
		}
	}
}

// table reads the composite literal a table function returns: each pattern and
// the variable whose address it holds.
func (e *embedReading) table(fn string) map[string]*types.Var {
	out := map[string]*types.Var{}
	for _, f := range e.p.Files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Name.Name != fn {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				kv, ok := n.(*ast.KeyValueExpr)
				if !ok {
					return true
				}
				lit, _ := kv.Key.(*ast.BasicLit)
				addr, _ := kv.Value.(*ast.UnaryExpr)
				var ident *ast.Ident
				if addr != nil && addr.Op == token.AND {
					ident, _ = addr.X.(*ast.Ident)
				}
				if lit == nil || ident == nil {
					e.bad("%s holds an entry that is not a pattern and an address", fn)
					return false
				}
				key, err := strconv.Unquote(lit.Value)
				if err != nil {
					e.bad("%s holds a key that is not a quoted pattern: %s", fn, lit.Value)
					return false
				}
				out[key], _ = e.p.Info.Uses[ident].(*types.Var)
				return false
			})
		}
	}
	if len(out) == 0 {
		e.bad("%s holds no entry, so nothing was read", fn)
	}
	return out
}

// files is every file a pattern embeds, by its slash path under the package.
func (e *embedReading) files(pattern string) []string {
	matches, err := filepath.Glob(filepath.Join(e.dir, filepath.FromSlash(pattern)))
	if err != nil || len(matches) == 0 {
		e.bad("the pattern %s matches no file: %v", pattern, err)
		return nil
	}
	var out []string
	for _, m := range matches {
		err := filepath.WalkDir(m, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(e.dir, p)
			out = append(out, filepath.ToSlash(rel))
			return err
		})
		if err != nil {
			e.bad("the files of %s could not be read: %v", pattern, err)
		}
	}
	return out
}

// interfaceOf is the one package variable set from an embed.FS, which is the
// only place that names it.
func (e *embedReading) interfaceOf(fsVar *types.Var) *types.Var {
	var uses []*ast.Ident
	for id, obj := range e.p.Info.Uses {
		if obj == fsVar {
			uses = append(uses, id)
		}
	}
	if len(uses) != 1 {
		e.bad("%s is named %d times, and only its interface variable may name it", fsVar.Name(), len(uses))
		return nil
	}
	for _, f := range e.p.Files {
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.VAR {
				continue
			}
			for _, s := range g.Specs {
				spec := s.(*ast.ValueSpec)
				if len(spec.Names) == 1 && len(spec.Values) == 1 && spec.Values[0] == ast.Expr(uses[0]) {
					v, _ := e.p.Info.Defs[spec.Names[0]].(*types.Var)
					return v
				}
			}
		}
	}
	e.bad("%s is named somewhere other than the initializer of its interface variable", fsVar.Name())
	return nil
}

// initializers refuses a package variable whose initializer runs code that
// names an embedded variable, or a tree's interface variable.
//
// ⛔ GO MAY SET IT BEFORE takeMutationRow RUNS, and it would keep the old
// bytes. ⚠ Three initializers may: takeMutationRow's own, and each interface
// variable's, which takeMutationRow then replaces.
func (e *embedReading) initializers() {
	held := map[types.Object]bool{}
	for _, d := range e.directives {
		held[d.v] = true
	}
	for _, v := range e.iface {
		held[v] = true
	}
	takes := 0
	for _, f := range e.p.Files {
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.VAR {
				continue
			}
			for _, s := range g.Specs {
				spec := s.(*ast.ValueSpec)
				for i, value := range spec.Values {
					name := spec.Names[min(i, len(spec.Names)-1)]
					if call, ok := value.(*ast.CallExpr); ok && name.Name == "_" {
						if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "takeMutationRow" {
							takes++
							continue
						}
					}
					if v, ok := e.p.Info.Defs[name].(*types.Var); ok && held[v] {
						continue
					}
					var names []string
					for obj := range e.p.Runs(value) {
						if held[obj] {
							names = append(names, obj.Name())
						}
					}
					sort.Strings(names)
					for _, n := range names {
						at := e.p.Fset.Position(value.Pos())
						e.bad("%s is set from %s at %s:%d, maybe before takeMutationRow hands it a row's bytes", name.Name, n, filepath.Base(at.Filename), at.Line)
					}
				}
			}
		}
	}
	if takes != 1 {
		e.bad("takeMutationRow is a package variable's initializer %d times, want once", takes)
	}
}

// embedFixture is a package that breaks each rule of readEmbeds once.
var embedFixture = map[string]string{
	"go.mod":   "module example.test\n\ngo 1.25.0\n",
	"q/a.sh":   "a\n",
	"q/b.sh":   "b\n",
	"q/d/x.sh": "x\n",
	"q/s.txt":  "s\n",
	"q/q.go": `package q

import (
	"embed"
	"io/fs"
	"strings"
)

var (
	//go:embed a.sh
	a []byte
)

//go:embed b.sh
var b1 []byte

//go:embed b.sh
var b2 []byte

//go:embed d
var dFiles embed.FS

//go:embed d/x.sh
var dx []byte

//go:embed s.txt
var s string

var dTree fs.FS = dFiles

var early = strings.ToUpper(string(b1))

var later = load()

func load() string { return string(dx) }

func peek() { _ = dFiles }

var _ = takeMutationRow()

func takeMutationRow() bool { return true }

func embeddedBytes() map[string]*[]byte {
	return map[string]*[]byte{"b.sh": &b1, "d/x.sh": &dx}
}

func embeddedTrees() map[string]*fs.FS { return map[string]*fs.FS{"d": &dTree} }
`,
}

// TestTheEmbedReadingRefusesWhatWouldKeepOldBytes: each shape here kept a
// row's old bytes somewhere, and the reading before TOOL-40's review missed it.
//
// ⛔ A go:embed IN A var BLOCK, a pattern two variables embed, and a file a
// directory's pattern embeds too. Also a variable set from an embedded one,
// directly or through a function, and an embed.FS named beside its interface.
func TestTheEmbedReadingRefusesWhatWouldKeepOldBytes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for name, body := range embedFixture {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := forksafe.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(root, "q")
	p, err := parallelread.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(readEmbeds(p, dir).found, "\n")
	for _, want := range []string{
		"a.sh is embedded in a, and the table does not hand a row of it there",
		"b.sh is embedded by both b1 and b2",
		"d/x.sh is embedded by both dFiles and dx",
		"s embeds into a string",
		"early is set from b1",
		"later is set from dx",
		"dFiles is named 2 times",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the reading did not find %q", want)
		}
	}
	if t.Failed() {
		t.Logf("the reading found:\n%s", got)
	}
}
