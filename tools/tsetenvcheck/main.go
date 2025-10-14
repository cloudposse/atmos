package main

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"
)

var Analyzer = &analysis.Analyzer{
	Name: "tsetenvcheck",
	Doc:  "checks that t.Setenv is not called inside defer or t.Cleanup blocks",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			// Look for defer statements.
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
				call, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}

				// Check if this is a call to t.Setenv.
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				if sel.Sel.Name == "Setenv" {
					// Check if the receiver is 't' or '*testing.T'.
					if ident, ok := sel.X.(*ast.Ident); ok && (ident.Name == "t" || ident.Name == "b") {
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

		// Also check for t.Cleanup calls containing t.Setenv.
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Check if this is a call to t.Cleanup.
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			if sel.Sel.Name == "Cleanup" {
				// Check if argument is a function literal.
				if len(call.Args) > 0 {
					if funcLit, ok := call.Args[0].(*ast.FuncLit); ok {
						// Inspect the cleanup function for t.Setenv.
						ast.Inspect(funcLit.Body, func(inner ast.Node) bool {
							innerCall, ok := inner.(*ast.CallExpr)
							if !ok {
								return true
							}

							innerSel, ok := innerCall.Fun.(*ast.SelectorExpr)
							if !ok {
								return true
							}

							if innerSel.Sel.Name == "Setenv" {
								if ident, ok := innerSel.X.(*ast.Ident); ok && (ident.Name == "t" || ident.Name == "b") {
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
			}

			return true
		})
	}

	return nil, nil
}

func main() {
	singlechecker.Main(Analyzer)
}
