package linters

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// Rule represents a linting rule that can be applied to Go source files.
type Rule interface {
	// Name returns the rule's identifier (e.g., "tsetenv-in-defer").
	Name() string

	// Doc returns the rule's documentation.
	Doc() string

	// Check analyzes a file and reports violations.
	Check(pass *analysis.Pass, file *ast.File) error
}
