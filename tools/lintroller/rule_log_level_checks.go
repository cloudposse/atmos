package linters

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// LogLevelChecksRule checks for log level comparisons outside the logger package.
type LogLevelChecksRule struct{}

func (r *LogLevelChecksRule) Name() string {
	return "log-level-checks"
}

func (r *LogLevelChecksRule) Doc() string {
	return "Checks for log level comparisons outside the logger package; log levels are internal implementation details and should not control UI behavior"
}

func (r *LogLevelChecksRule) Check(pass *analysis.Pass, file *ast.File) error {
	filename := pass.Fset.Position(file.Pos()).Filename

	// Skip files in the logger package (check both path and package name).
	if strings.Contains(filename, "/pkg/logger/") {
		return nil
	}
	if file.Name != nil && file.Name.Name == "logger" {
		return nil
	}

	// Skip test files - it's reasonable for tests to check log levels.
	if strings.HasSuffix(filename, "_test.go") {
		return nil
	}

	// Check for log level accesses and comparisons.
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			// Check for atmosConfig.Logs.Level access.
			if r.isLogsLevelAccess(node) {
				pass.Reportf(node.Pos(),
					"accessing atmosConfig.Logs.Level outside of logger package is not allowed; "+
						"log levels are internal implementation details and should not control UI behavior or program logic")
				return true
			}

		case *ast.BinaryExpr:
			// Check for comparisons with log level constants.
			if r.isLogLevelComparison(node) {
				pass.Reportf(node.Pos(),
					"comparing log levels outside of logger package is not allowed; "+
						"log levels are internal implementation details and should not control UI behavior or program logic")
				return true
			}
		}

		return true
	})

	return nil
}

// isLogsLevelAccess checks if a selector expression accesses atmosConfig.Logs.Level.
func (r *LogLevelChecksRule) isLogsLevelAccess(sel *ast.SelectorExpr) bool {
	if sel.Sel.Name != "Level" {
		return false
	}

	// Check if this is accessing .Logs.Level.
	if innerSel, ok := sel.X.(*ast.SelectorExpr); ok {
		if innerSel.Sel.Name == "Logs" {
			// Check if the base is atmosConfig or any variable.
			return true
		}
	}

	return false
}

// isLogLevelComparison checks if a binary expression compares log levels.
func (r *LogLevelChecksRule) isLogLevelComparison(bin *ast.BinaryExpr) bool {
	// Check for equality or inequality operators.
	if bin.Op.String() != "==" && bin.Op.String() != "!=" {
		return false
	}

	// Check if either side references LogLevel constants.
	return r.referencesLogLevel(bin.X) || r.referencesLogLevel(bin.Y)
}

// referencesLogLevel checks if an expression references a log level constant.
func (r *LogLevelChecksRule) referencesLogLevel(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		// Check for u.LogLevelTrace, u.LogLevelDebug, etc.
		if ident, ok := e.X.(*ast.Ident); ok {
			if ident.Name == "u" && strings.HasPrefix(e.Sel.Name, "LogLevel") {
				return true
			}
		}

		// Check for atmosConfig.Logs.Level access.
		if r.isLogsLevelAccess(e) {
			return true
		}
	}

	return false
}
