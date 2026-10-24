// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// TestTheHelperRoutePassesAStaleBootIDToTheJob holds the helper route's ensure.
// ⛔ The helper's unflagged ensure answers a stale boot id with a refusal, and
// returning it stopped `run` and `matrix` before the helper's Runner.Run, which
// is where the state is cleared, so after `wsl --shutdown` the route still exited
// 2. The refusal is the one the helper's Ensure produces, sent back as text the
// way the client receives it. Any other failure still stops the job. WSL-137.
func TestTheHelperRoutePassesAStaleBootIDToTheJob(t *testing.T) {
	podman := errors.New("verify: Error: current system boot ID differs from cached boot ID; an unhandled reboot has occurred. Please delete directories \"/tmp/wsl-toolkit-run-1000/containers\" and \"/tmp/wsl-toolkit-run-1000/libpod/tmp\" and re-run Podman")
	r, ok := toolkit.StaleRunStateRemediation(podman.Error())
	if !ok {
		t.Fatal("the engine's own refusal is not read as a stale run state")
	}
	refusal := errors.New(toolkit.StaleRefusalError(r, podman).Error())
	ensureAnswering := func(err error) func(context.Context, bool) (toolkit.BaseState, error) {
		return func(_ context.Context, force bool) (toolkit.BaseState, error) {
			if force {
				t.Error("the helper route asked for a forced ensure")
			}
			return toolkit.BaseState{}, err
		}
	}
	ctx := context.Background()
	if err := helperEnsureBase(ctx, ensureAnswering(refusal)); err != nil {
		t.Errorf("a stale boot id stopped the job before the helper could clear it: %v", err)
	}
	if err := helperEnsureBase(ctx, ensureAnswering(nil)); err != nil {
		t.Errorf("an ensure that passed: %v", err)
	}
	other := errors.New("the base is not registered")
	if err := helperEnsureBase(ctx, ensureAnswering(other)); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Errorf("an ensure that failed for another reason was passed: %v", err)
	}
}

// TestRunAndMatrixAskTheHelperThroughThePassingEnsure reads cmdRun's and
// cmdMatrix's own syntax. ⛔ On the helper route each asks its ensure through
// helperEnsureBase and never calls the client's BaseEnsure itself, so neither
// stops on the refusal the helper clears. ⚠ A textual rule, as
// TestBaseExecClearsAStaleBootIDBeforeItsCommand is: the route reaches a helper
// that no case here stands in for. WSL-137.
func TestRunAndMatrixAskTheHelperThroughThePassingEnsure(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "cmd_run.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cmdRun", "cmdMatrix"} {
		var body *ast.BlockStmt
		for _, d := range file.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == name {
				body = fn.Body
			}
		}
		if body == nil {
			t.Fatalf("cmd_run.go defines no %s", name)
		}
		passing, direct := 0, 0
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				if fun.Name == "helperEnsureBase" && len(call.Args) == 2 {
					if sel, ok := call.Args[1].(*ast.SelectorExpr); ok && sel.Sel.Name == "BaseEnsure" {
						passing++
					}
				}
			case *ast.SelectorExpr:
				if x, ok := fun.X.(*ast.Ident); ok && x.Name == "c" && fun.Sel.Name == "BaseEnsure" {
					direct++
				}
			}
			return true
		})
		if passing != 1 || direct != 0 {
			t.Errorf("%s asks the helper's ensure through helperEnsureBase %d times and directly %d times; want once and never", name, passing, direct)
		}
	}
}
