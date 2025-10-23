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

	// For each test function, check if it uses RootCmd and if it calls NewTestKit.
	for _, testFunc := range testFunctions {
		usesRootCmd := false
		callsTestKit := false

		// Check if the function uses RootCmd directly.
		ast.Inspect(testFunc.Body, func(n ast.Node) bool {
			// Check for RootCmd identifier usage.
			if ident, ok := n.(*ast.Ident); ok {
				if ident.Name == "RootCmd" {
					usesRootCmd = true
				}
			}

			// Check for RootCmd.Execute() or RootCmd.SetArgs() calls specifically.
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					// Only flag Execute/SetArgs if called on RootCmd.
					if sel.Sel.Name == "Execute" || sel.Sel.Name == "SetArgs" {
						if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "RootCmd" {
							usesRootCmd = true
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

		// Report if RootCmd is used without TestKit.
		if usesRootCmd && !callsTestKit {
			pass.Reportf(testFunc.Pos(),
				"test function %s uses RootCmd but does not call NewTestKit; "+
					"use t := NewTestKit(t) to ensure proper RootCmd state cleanup",
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
