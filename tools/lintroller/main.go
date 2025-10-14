package main

import (
	"flag"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"
)

var (
	enableTSetenvInDefer = flag.Bool("tsetenv-in-defer", true, "check for t.Setenv in defer/cleanup blocks")
	enableOsSetenvInTest = flag.Bool("os-setenv-in-test", true, "check for os.Setenv in test files")
)

var Analyzer = &analysis.Analyzer{
	Name:  "lintroller",
	Doc:   "Atmos project-specific linting rules",
	Run:   run,
	Flags: *flag.NewFlagSet("lintroller", flag.ExitOnError),
}

func init() {
	Analyzer.Flags.BoolVar(enableTSetenvInDefer, "tsetenv-in-defer", true, "check for t.Setenv in defer/cleanup blocks")
	Analyzer.Flags.BoolVar(enableOsSetenvInTest, "os-setenv-in-test", true, "check for os.Setenv in test files")
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		isTestFile := strings.HasSuffix(filename, "_test.go")

		if *enableTSetenvInDefer {
			checkTSetenvInDefer(pass, file)
		}

		if *enableOsSetenvInTest && isTestFile {
			// Find benchmark functions to exclude from os.Setenv checks.
			benchmarks := findBenchmarks(file)
			checkOsSetenvInTest(pass, file, benchmarks)
		}
	}

	return nil, nil
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

// checkTSetenvInDefer checks for t.Setenv called inside defer or t.Cleanup blocks.
func checkTSetenvInDefer(pass *analysis.Pass, file *ast.File) {
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
}

// checkOsSetenvInTest checks for os.Setenv in test files, except inside defer/cleanup blocks and benchmarks.
func checkOsSetenvInTest(pass *analysis.Pass, file *ast.File, benchmarks map[*ast.FuncDecl]bool) {
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
}

// isInside checks if node is inside parent.
func isInside(node, parent ast.Node) bool {
	if node == nil || parent == nil {
		return false
	}
	return node.Pos() >= parent.Pos() && node.End() <= parent.End()
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

// isOsSetenvCall checks if a call expression is os.Setenv.
func isOsSetenvCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Setenv" {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "os"
}

func main() {
	singlechecker.Main(Analyzer)
}
