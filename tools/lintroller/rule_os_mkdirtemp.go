package linters

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// OsMkdirTempInTestRule checks for os.MkdirTemp in test files.
type OsMkdirTempInTestRule struct{}

func (r *OsMkdirTempInTestRule) Name() string {
	return "os-mkdirtemp-in-test"
}

func (r *OsMkdirTempInTestRule) Doc() string {
	return "Checks for os.MkdirTemp in test files; use t.TempDir instead for automatic cleanup"
}

func (r *OsMkdirTempInTestRule) Check(pass *analysis.Pass, file *ast.File) error {
	filename := pass.Fset.Position(file.Pos()).Filename
	if !strings.HasSuffix(filename, "_test.go") {
		return nil // Only check test files.
	}

	// Find benchmark functions to exclude from checks.
	benchmarks := findBenchmarksForMkdirTemp(file)

	// Check os.MkdirTemp calls, skipping benchmarks.
	ast.Inspect(file, func(n ast.Node) bool {
		// Skip if we're inside a benchmark function.
		for benchmark := range benchmarks {
			if isInsideMkdirTemp(n, benchmark.Body) {
				return true
			}
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if isOsMkdirTempCall(call) {
			pass.Reportf(call.Pos(),
				"os.MkdirTemp should not be used in test files; "+
					"use t.TempDir instead for automatic cleanup "+
					"(os.MkdirTemp is allowed in benchmark functions)")
		}

		return true
	})

	return nil
}

// findBenchmarksForMkdirTemp returns a set of function declarations that are benchmarks.
func findBenchmarksForMkdirTemp(file *ast.File) map[*ast.FuncDecl]bool {
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

// isInsideMkdirTemp checks if node is inside parent.
func isInsideMkdirTemp(node, parent ast.Node) bool {
	if node == nil || parent == nil {
		return false
	}
	return node.Pos() >= parent.Pos() && node.End() <= parent.End()
}

// isOsMkdirTempCall checks if a call expression is os.MkdirTemp.
func isOsMkdirTempCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "MkdirTemp" {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "os"
}
