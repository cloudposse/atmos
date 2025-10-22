package linters

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// OsArgsInTestRule checks for os.Args usage in test files.
type OsArgsInTestRule struct{}

func (r *OsArgsInTestRule) Name() string {
	return "os-args-in-test"
}

func (r *OsArgsInTestRule) Doc() string {
	return "Checks for os.Args usage in test files; use cmd.SetArgs() instead"
}

func (r *OsArgsInTestRule) Check(pass *analysis.Pass, file *ast.File) error {
	filename := pass.Fset.Position(file.Pos()).Filename
	if !strings.HasSuffix(filename, "_test.go") {
		return nil // Only check test files.
	}

	// Skip integration tests - they test main() which requires os.Args.
	if strings.HasSuffix(filename, "_integration_test.go") ||
		strings.Contains(filename, "/main_") && strings.HasSuffix(filename, "_integration_test.go") {
		return nil
	}

	// Skip test helper files - they need to snapshot/restore os.Args for test isolation.
	if strings.HasSuffix(filename, "testing_helpers_test.go") ||
		strings.HasSuffix(filename, "testkit_test.go") {
		return nil
	}

	// Skip specific test files that legitimately need os.Args:
	// - Testing functions that directly read os.Args (flag parsers)
	// - Subprocess testing using os.Args[0] for executable path
	// - Testing heatmap flag detection
	// All these use proper save/restore pattern for test isolation.
	if strings.HasSuffix(filename, "cmd/cmd_utils_test.go") ||
		strings.HasSuffix(filename, "cmd/terraform_test.go") ||
		strings.HasSuffix(filename, "errors/error_funcs_test.go") ||
		strings.HasSuffix(filename, "pkg/config/config_test.go") {
		return nil
	}

	// Find benchmark functions to exclude from checks.
	benchmarks := findBenchmarks(file)

	ast.Inspect(file, func(n ast.Node) bool {
		// Skip if we're inside a benchmark function.
		for benchmark := range benchmarks {
			if isInside(n, benchmark.Body) {
				return true
			}
		}

		// Check for os.Args usage (reads or writes).
		if isOsArgsUsage(n) {
			pass.Reportf(n.Pos(),
				"os.Args should not be used in test files; "+
					"use cmd.SetArgs() instead to set command arguments "+
					"(os.Args is allowed in benchmark functions)")
		}

		return true
	})

	return nil
}

// isOsArgsUsage checks if a node is accessing os.Args.
func isOsArgsUsage(n ast.Node) bool {
	// Check for os.Args selector expression (e.g., os.Args, os.Args[0]).
	sel, ok := n.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Args" {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "os"
}
