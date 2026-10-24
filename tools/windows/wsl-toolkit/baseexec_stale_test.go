// SPDX-License-Identifier: 0BSD

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestBaseExecClearsAStaleBootIDBeforeItsCommand reads cmdBaseExec's own syntax
// and holds the order: the stale run state check before the command starts,
// attached or detached. ⚠ A textual rule, as TestBaseShellPrefersBashAsALoginShell
// is: the command reaches a real wsl.exe, which no case here can stand in for, and
// the check itself is held by its own cases in the toolkit package. WSL-137.
func TestBaseExecClearsAStaleBootIDBeforeItsCommand(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "cmd_base_exec.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body *ast.BlockStmt
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "cmdBaseExec" {
			body = fn.Body
		}
	}
	if body == nil {
		t.Fatal("cmd_base_exec.go defines no cmdBaseExec")
	}
	check, detach, attach := token.NoPos, token.NoPos, token.NoPos
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if x, ok := fun.X.(*ast.Ident); ok {
				switch {
				case x.Name == "toolkit" && fun.Sel.Name == "ClearStaleRunState" && check == token.NoPos:
					check = call.Pos()
				case x.Name == "w" && fun.Sel.Name == "Exec" && attach == token.NoPos:
					attach = call.Pos()
				}
			}
		case *ast.Ident:
			if fun.Name == "detachSession" && detach == token.NoPos {
				detach = call.Pos()
			}
		}
		return true
	})
	if check == token.NoPos {
		t.Fatal("base exec starts its command without clearing a stale engine boot id")
	}
	if detach == token.NoPos || attach == token.NoPos || check > detach || check > attach {
		t.Fatalf("the stale check is at %s, the detached start at %s and the attached one at %s",
			fset.Position(check), fset.Position(detach), fset.Position(attach))
	}
}
