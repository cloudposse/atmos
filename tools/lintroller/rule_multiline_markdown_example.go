package linters

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// MultilineMarkdownExampleRule checks for multi-line markdown examples in cobra commands.
type MultilineMarkdownExampleRule struct{}

func (r *MultilineMarkdownExampleRule) Name() string {
	return "multiline-markdown-example"
}

func (r *MultilineMarkdownExampleRule) Doc() string {
	return "Checks for multi-line markdown examples in cobra commands; use markdown package instead"
}

func (r *MultilineMarkdownExampleRule) Check(pass *analysis.Pass, file *ast.File) error {
	filename := pass.Fset.Position(file.Pos()).Filename
	if !strings.HasSuffix(filename, ".go") || strings.HasSuffix(filename, "_test.go") {
		return nil // Only check non-test Go files.
	}

	ast.Inspect(file, func(n ast.Node) bool {
		// Look for composite literals (struct initializations).
		comp, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		// Check if this is a cobra.Command initialization.
		if !isCobraCommand(comp) {
			return true
		}

		// Check the Example field for multi-line content.
		for _, elt := range comp.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}

			// Check if this is the Example field.
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "Example" {
				continue
			}

			// Check if the value is a basic literal (string).
			lit, ok := kv.Value.(*ast.BasicLit)
			if !ok {
				continue
			}

			// Check if the string contains newlines (multi-line example).
			if strings.Contains(lit.Value, "\\n") || strings.Count(lit.Value, "\n") > 0 {
				pass.Reportf(lit.Pos(),
					"multi-line markdown examples should use embedded markdown files from cmd/markdown/ instead of inline strings; "+
						"see CLAUDE.md for the pattern")
			}
		}

		return true
	})

	return nil
}

// isCobraCommand checks if a composite literal is a cobra.Command initialization.
func isCobraCommand(comp *ast.CompositeLit) bool {
	// Check for *cobra.Command or cobra.Command types.
	switch typ := comp.Type.(type) {
	case *ast.StarExpr:
		sel, ok := typ.X.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		ident, ok := sel.X.(*ast.Ident)
		return ok && ident.Name == "cobra" && sel.Sel.Name == "Command"
	case *ast.SelectorExpr:
		ident, ok := typ.X.(*ast.Ident)
		return ok && ident.Name == "cobra" && typ.Sel.Name == "Command"
	}
	return false
}
