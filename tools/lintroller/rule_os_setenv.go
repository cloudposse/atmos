package linters

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// OsSetenvInTestRule checks for os.Setenv in test files (except in defer/cleanup blocks and benchmarks).
type OsSetenvInTestRule struct{}

func (r *OsSetenvInTestRule) Name() string {
	return "os-setenv-in-test"
}

func (r *OsSetenvInTestRule) Doc() string {
	return "Checks for os.Setenv in test files; use t.Setenv instead for automatic cleanup"
}

func (r *OsSetenvInTestRule) Check(pass *analysis.Pass, file *ast.File) error {
	filename := pass.Fset.Position(file.Pos()).Filename
	if !strings.HasSuffix(filename, "_test.go") {
		return nil // Only check test files.
	}

	// Find benchmark functions to exclude from checks.
	benchmarks := findBenchmarks(file)

	// Track positions of defer/cleanup blocks to skip os.Setenv inside them.
	cleanupBlocks := make(map[ast.Node]bool)

	// First pass: find all defer and t.Cleanup blocks.
	ast.Inspect(file, func(n ast.Node) bool {
		// Check for defer statements.
		if deferStmt, ok := n.(*ast.DeferStmt); ok {
			if funcLit, ok := deferStmt.Call.Fun.(*ast.FuncLit); ok {
				cleanupBlocks[funcLit.Body] = true
			}
			return true
		}

		// Check for t.Cleanup calls.
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Cleanup" {
				if len(call.Args) > 0 {
					if funcLit, ok := call.Args[0].(*ast.FuncLit); ok {
						cleanupBlocks[funcLit.Body] = true
					}
				}
			}
		}

		return true
	})

	// Second pass: check os.Setenv calls, skipping those in cleanup blocks and benchmarks.
	ast.Inspect(file, func(n ast.Node) bool {
		// Skip if we're inside a cleanup block.
		for cleanupBlock := range cleanupBlocks {
			if isInside(n, cleanupBlock) {
				return true
			}
		}

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

		if isOsSetenvCall(call) {
			pass.Reportf(call.Pos(),
				"os.Setenv should not be used in test files; "+
					"use t.Setenv instead for automatic cleanup "+
					"(os.Setenv is allowed inside defer/t.Cleanup blocks and benchmark functions for manual restoration)")
		}

		return true
	})

	return nil
}

// findBenchmarks returns a set of function declarations that are benchmarks.
func findBenchmarks(file *ast.File) map[*ast.FuncDecl]bool {
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

// isInside checks if node is inside parent.
func isInside(node, parent ast.Node) bool {
	if node == nil || parent == nil {
		return false
	}
	return node.Pos() >= parent.Pos() && node.End() <= parent.End()
}

// isOsSetenvCall checks if a call expression is os.Setenv.
func isOsSetenvCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Setenv" {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "os"
}
