package linters

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// TSetenvInDeferRule checks for t.Setenv called inside defer or t.Cleanup blocks.
type TSetenvInDeferRule struct{}

func (r *TSetenvInDeferRule) Name() string {
	return "tsetenv-in-defer"
}

func (r *TSetenvInDeferRule) Doc() string {
	return "Checks for t.Setenv in defer/t.Cleanup blocks (t.Setenv handles cleanup automatically)"
}

func (r *TSetenvInDeferRule) Check(pass *analysis.Pass, file *ast.File) error {
	// Check defer statements.
	ast.Inspect(file, func(n ast.Node) bool {
		deferStmt, ok := n.(*ast.DeferStmt)
		if !ok {
			return true
		}

		// Check if the deferred call is a function literal.
		funcLit, ok := deferStmt.Call.Fun.(*ast.FuncLit)
		if !ok {
			return true
		}

		// Inspect the function body for t.Setenv calls.
		ast.Inspect(funcLit.Body, func(inner ast.Node) bool {
			if call, ok := inner.(*ast.CallExpr); ok {
				if isTSetenvCall(call) {
					pass.Reportf(call.Pos(),
						"t.Setenv should not be called inside defer blocks; "+
							"t.Setenv handles cleanup automatically. "+
							"Use os.Setenv for manual restoration in defer")
				}
			}
			return true
		})

		return true
	})

	// Check t.Cleanup calls.
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Cleanup" {
			return true
		}

		// Check if argument is a function literal.
		if len(call.Args) > 0 {
			if funcLit, ok := call.Args[0].(*ast.FuncLit); ok {
				ast.Inspect(funcLit.Body, func(inner ast.Node) bool {
					if innerCall, ok := inner.(*ast.CallExpr); ok {
						if isTSetenvCall(innerCall) {
							pass.Reportf(innerCall.Pos(),
								"t.Setenv should not be called inside t.Cleanup; "+
									"t.Setenv handles cleanup automatically. "+
									"Use os.Setenv for manual restoration in cleanup functions")
						}
					}
					return true
				})
			}
		}

		return true
	})

	return nil
}

// isTSetenvCall checks if a call expression is t.Setenv or b.Setenv.
func isTSetenvCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Setenv" {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	return ok && (ident.Name == "t" || ident.Name == "b")
}
