// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/embedswap"
)

// ⭐ A MUTATION ROW ON AN EMBEDDED FILE RUNS ON THE BINARY ITS BASELINE BUILT.
// tools/repo's harness hands this binary the changed bytes instead of compiling
// the package again. A compile of this package and its cases is most of a row.
// internal/embedswap reads the harness's variables. TOOL-40.
func init() {
	// ⚠ NOT FATAL. A binary that could not take the bytes leaves the file as it
	// was, and the harness compiles the row instead.
	if err := embedswap.Take(takeEmbedded); err != nil {
		fmt.Fprintf(os.Stderr, "the embedded file a mutation row named was not taken: %v\n", err)
	}
}

// embeddedBytes is every file this package embeds as a []byte, by its pattern.
// TestEveryEmbeddedFileCanTakeAMutation holds it to the go:embed lines.
func embeddedBytes() map[string]*[]byte {
	return map[string]*[]byte{
		"provision.sh":    &provisionScript,
		"boot.sh":         &bootScript,
		"packages.sh":     &packagesScript,
		"verify.sh":       &verifyScript,
		"repair.sh":       &repairScript,
		"grants.sh":       &grantsScript,
		"mount-helper.sh": &mountHelperScript,
		"binfmt.sh":       &binfmtScript,
		"doors.sh":        &doorsScript,
		"drives.sh":       &drivesScript,
		"engines.sh":      &enginesScript,
		"loginshell.sh":   &loginShellScript,
		"mirrors.sh":      &mirrorsScript,
		"private-net.sh":  &privateNetScript,
		"stale.sh":        &staleScript,
	}
}

// embeddedTrees is every directory this package embeds, by its pattern.
func embeddedTrees() map[string]*embedTree {
	return map[string]*embedTree{
		"adapters": &adapterTree,
		"shipped":  &shippedTree,
	}
}

// takeEmbedded puts a row's bytes in the variable that embeds name: a script's
// own, or a copy of the directory that holds it.
func takeEmbedded(name string, body []byte) error {
	if p, ok := embeddedBytes()[name]; ok {
		*p = body
		return nil
	}
	root, _, _ := strings.Cut(name, "/")
	tree, ok := embeddedTrees()[root]
	if !ok {
		return fmt.Errorf("%s is not a file this package embeds", name)
	}
	copied, err := embedswap.WithFile(*tree, name, body)
	if err != nil {
		return err
	}
	*tree = copied
	return nil
}

// embedDirective is one go:embed line of this package and the variable under it.
type embedDirective struct {
	pattern, variable string
	tree              bool
}

// embedDirectives reads every go:embed line of this package's own files.
func embedDirectives(t *testing.T) []embedDirective {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []embedDirective
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Doc == nil {
				continue
			}
			for _, c := range g.Doc.List {
				pattern, ok := strings.CutPrefix(c.Text, "//go:embed ")
				if !ok {
					continue
				}
				spec := g.Specs[0].(*ast.ValueSpec)
				_, isBytes := spec.Type.(*ast.ArrayType)
				out = append(out, embedDirective{pattern: strings.TrimSpace(pattern), variable: spec.Names[0].Name, tree: !isBytes})
			}
		}
	}
	return out
}

// TestEveryEmbeddedFileCanTakeAMutation holds the two tables to the go:embed
// lines, so a file embedded later is not one whose rows compile again.
//
// ⛔ EACH ENTRY IS CHECKED AGAINST ITS FILE'S BYTES, not its name alone. An
// entry that pointed at another pattern's variable would hand a row's bytes to
// the wrong script, and its case would go red for that.
func TestEveryEmbeddedFileCanTakeAMutation(t *testing.T) {
	scripts, trees := embeddedBytes(), embeddedTrees()
	seen := map[string]bool{}
	for _, d := range embedDirectives(t) {
		seen[d.pattern] = true
		if !d.tree {
			p, ok := scripts[d.pattern]
			if !ok {
				t.Errorf("%s is embedded in %s and a mutation row cannot hand it to this binary", d.pattern, d.variable)
				continue
			}
			want, err := os.ReadFile(d.pattern)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(*p, want) {
				t.Errorf("the entry for %s is not %s's bytes", d.pattern, d.variable)
			}
			continue
		}
		tree, ok := trees[d.pattern]
		if !ok {
			t.Errorf("the directory %s is embedded in %s and a mutation row cannot hand a file of it to this binary", d.pattern, d.variable)
			continue
		}
		var held []string
		err := fs.WalkDir(*tree, d.pattern, func(p string, e fs.DirEntry, err error) error {
			if err == nil && !e.IsDir() {
				held = append(held, p)
			}
			return err
		})
		if err != nil || len(held) == 0 {
			t.Errorf("the entry for %s holds no file of it: %v", d.pattern, err)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no go:embed line was read, so nothing was checked")
	}
	var extra []string
	for name := range scripts {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	for name := range trees {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("the tables name %v, which no go:embed line embeds", extra)
	}
}

// TestAMutationRowsBytesReachTheVariableAndTheFile is what the harness relies on
// when it runs a row on the binary its baseline built.
func TestAMutationRowsBytesReachTheVariableAndTheFile(t *testing.T) {
	savedVerify, savedTree := verifyScript, adapterTree
	t.Cleanup(func() { verifyScript, adapterTree = savedVerify, savedTree })
	dir := t.TempDir()
	hand := func(name string, body []byte) string {
		t.Helper()
		with := filepath.Join(dir, "with-"+path.Base(name))
		file := filepath.Join(dir, path.Base(name))
		if err := os.WriteFile(with, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("as it was\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(embedswap.NameEnv, name)
		t.Setenv(embedswap.WithEnv, with)
		t.Setenv(embedswap.FileEnv, file)
		return file
	}
	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	mutated := []byte("#!/bin/sh\necho the row's verify.sh\n")
	file := hand("verify.sh", mutated)
	if err := embedswap.Take(takeEmbedded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(verifyScript, mutated) {
		t.Error("verify.sh's variable did not take the row's bytes, so its cases ran on the old script")
	}
	if got := read(file); got != string(mutated) {
		t.Errorf("the file was left %q, so a case that reads it saw the old script", got)
	}

	probe := []byte("#!/bin/sh\necho the row's probe.sh\n")
	file = hand("adapters/muse/probe.sh", probe)
	if err := embedswap.Take(takeEmbedded); err != nil {
		t.Fatal(err)
	}
	if got, err := adapterScript("muse", "probe.sh"); err != nil || !bytes.Equal(got, probe) {
		t.Errorf("muse's probe.sh read %q, %v, not the row's bytes", got, err)
	}
	if got := read(file); got != string(probe) {
		t.Errorf("the file was left %q, so a case that reads it saw the old script", got)
	}
	// ⛔ THE REST OF THE TREE IS THE EMBEDDED ONE. A copy that lost a file would
	// fail an adapter's case for a reason the row did not cause.
	for _, name := range []string{"install.sh", "herdr-agent-state.sh"} {
		want, err := savedTree.ReadFile("adapters/muse/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := adapterTree.ReadFile("adapters/muse/" + name); err != nil || !bytes.Equal(got, want) {
			t.Errorf("muse's %s changed when probe.sh was handed in: %v", name, err)
		}
	}
	was, _ := savedTree.ReadDir("adapters/muse")
	now, err := adapterTree.ReadDir("adapters/muse")
	if err != nil || len(now) != len(was) {
		t.Errorf("muse's directory lists %d entries after the swap and %d before: %v", len(now), len(was), err)
	}

	// ⛔ A NAME THIS PACKAGE DOES NOT EMBED WRITES NO FILE, so the harness sees
	// the row was not taken and compiles it.
	file = hand("nothing-embeds-this.sh", []byte("x\n"))
	if err := embedswap.Take(takeEmbedded); err == nil {
		t.Error("a file this package does not embed was taken")
	}
	if got := read(file); got != "as it was\n" {
		t.Errorf("a file this package does not embed was written: %q", got)
	}
}
