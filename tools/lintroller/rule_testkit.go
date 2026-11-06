package linters

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// TestKitRule checks that cmd package tests using RootCmd call NewTestKit for cleanup.
type TestKitRule struct{}

func (r *TestKitRule) Name() string {
	return "testkit-required"
}

func (r *TestKitRule) Doc() string {
	return "Checks that cmd package tests call NewTestKit when using RootCmd, Execute, or SetArgs to ensure proper cleanup"
}

func (r *TestKitRule) Check(pass *analysis.Pass, file *ast.File) error {
	filename := pass.Fset.Position(file.Pos()).Filename
	if !strings.HasSuffix(filename, "_test.go") {
		return nil // Only check test files.
	}

	// Only check files in the cmd package (by package name, not path).
	if file.Name.Name != "cmd" {
		return nil
	}

	// Find all test functions.
	testFunctions := findTestFunctions(file)

	// For each test function, check if it modifies RootCmd and if it calls NewTestKit.
	for _, testFunc := range testFunctions {
		modifiesRootCmd := false
		callsTestKit := false

		// Check if the function modifies RootCmd state.
		ast.Inspect(testFunc.Body, func(n ast.Node) bool {
			// Check for RootCmd.Execute() or RootCmd.SetArgs() calls specifically.
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "RootCmd" {
						// Methods that modify RootCmd state.
						switch sel.Sel.Name {
						case "Execute", "ExecuteC", "SetArgs", "ParseFlags":
							modifiesRootCmd = true
						}
					}
				}
			}

			// Check for flag modifications: RootCmd.PersistentFlags().Set(...) or RootCmd.Flags().Set(...).
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if sel.Sel.Name == "Set" {
						// Check if Set is called on RootCmd.PersistentFlags() or RootCmd.Flags().
						if innerCall, ok := sel.X.(*ast.CallExpr); ok {
							if innerSel, ok := innerCall.Fun.(*ast.SelectorExpr); ok {
								if ident, ok := innerSel.X.(*ast.Ident); ok && ident.Name == "RootCmd" {
									if innerSel.Sel.Name == "PersistentFlags" || innerSel.Sel.Name == "Flags" {
										modifiesRootCmd = true
									}
								}
							}
						}
					}
				}
			}

			// Check for NewTestKit call.
			if call, ok := n.(*ast.CallExpr); ok {
				if ident, ok := call.Fun.(*ast.Ident); ok {
					if ident.Name == "NewTestKit" {
						callsTestKit = true
					}
				}
			}

			return true
		})

		// Report if RootCmd is modified without TestKit.
		if modifiesRootCmd && !callsTestKit {
			pass.Reportf(testFunc.Pos(),
				"test function %s modifies RootCmd state but does not call NewTestKit; "+
					"use _ = NewTestKit(t) to ensure proper RootCmd state cleanup "+
					"(only needed for Execute/SetArgs/ParseFlags/flag modifications, not read-only access)",
				testFunc.Name.Name)
		}
	}

	return nil
}

// findTestFunctions returns all test function declarations (Test*, Benchmark*).
func findTestFunctions(file *ast.File) []*ast.FuncDecl {
	var testFuncs []*ast.FuncDecl
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}
		if strings.HasPrefix(funcDecl.Name.Name, "Test") || strings.HasPrefix(funcDecl.Name.Name, "Benchmark") {
			testFuncs = append(testFuncs, funcDecl)
		}
		return true
	})
	return testFuncs
}
