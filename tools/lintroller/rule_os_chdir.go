package linters

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// OsChdirInTestRule checks for os.Chdir in test files.
type OsChdirInTestRule struct{}

func (r *OsChdirInTestRule) Name() string {
	return "os-chdir-in-test"
}

func (r *OsChdirInTestRule) Doc() string {
	return "Checks for os.Chdir in test files; use t.Chdir instead for automatic cleanup"
}

func (r *OsChdirInTestRule) Check(pass *analysis.Pass, file *ast.File) error {
	filename := pass.Fset.Position(file.Pos()).Filename
	if !strings.HasSuffix(filename, "_test.go") {
		return nil // Only check test files.
	}

	// Allow os.Chdir in test files that are explicitly testing chdir functionality.
	if strings.Contains(filename, "chdir_test.go") {
		return nil
	}

	// Find benchmark functions to exclude from checks.
	benchmarks := findBenchmarksForChdir(file)

	// Check os.Chdir calls, skipping benchmarks.
	ast.Inspect(file, func(n ast.Node) bool {
		// Skip if we're inside a benchmark function.
		for benchmark := range benchmarks {
			if isInside(n, benchmark.Body) {
				return true
			}
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if isOsChdirCall(call) {
			pass.Reportf(call.Pos(),
				"os.Chdir should not be used in test files; "+
					"use t.Chdir instead for automatic cleanup "+
					"(os.Chdir is allowed in benchmark functions)")
		}

		return true
	})

	return nil
}

// findBenchmarksForChdir returns a set of function declarations that are benchmarks.
func findBenchmarksForChdir(file *ast.File) map[*ast.FuncDecl]bool {
	benchmarks := make(map[*ast.FuncDecl]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}
		if strings.HasPrefix(funcDecl.Name.Name, "Benchmark") {
			benchmarks[funcDecl] = true
		}
		return true
	})
	return benchmarks
}

// isOsChdirCall checks if a call expression is os.Chdir.
func isOsChdirCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Chdir" {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "os"
}
