package engine

import (
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/afero"
)

// sensitiveLocals returns the paths, from the top of the unit's own locals, of the values its locals block
// declares sensitive. Terragrunt logs a value as it evaluates it, for example the output of
// sensitive(run_cmd(...)), even when the value never reaches the inputs, so these values are redacted too.
func sensitiveLocals(fsys afero.Fs, unit string, locals map[string]any) [][]string {
	body, ok := parseBody(fsys, unit)
	if !ok {
		return nil
	}
	scanner := &sensitivityScanner{locals: localExpressions(body)}
	for name, expression := range scanner.locals {
		scanner.walk(expression, []string{name}, 0)
	}
	return existingPaths(uniquePaths(scanner.paths), locals)
}

// unredactedSensitivity reports whether any of the files calls sensitive(...) where the sink cannot learn
// the value. When the unit was resolved, the values of its inputs and of its own locals are known and
// registered, so only calls elsewhere count: a locals block of an included file, or another attribute or
// block. When it was not resolved, every call counts. A file that cannot be read is skipped, since
// Terragrunt could not evaluate it either, and one that cannot be parsed counts, since nothing is known
// about it.
func unredactedSensitivity(fsys afero.Fs, files []string, unit string, resolved bool) bool {
	for _, file := range files {
		if filepath.Ext(file) != ".hcl" {
			continue
		}
		if _, err := fsys.Stat(file); err != nil {
			continue
		}
		body, ok := parseBody(fsys, file)
		if !ok {
			return true
		}
		for name, attribute := range body.Attributes {
			if resolved && name == inputsName {
				continue
			}
			if callsSensitive(attribute.Expr) {
				return true
			}
		}
		for _, block := range body.Blocks {
			if resolved && block.Type == "locals" && file == unit {
				continue
			}
			if callsSensitive(block.Body) {
				return true
			}
		}
	}
	return false
}

// parseBody reads and parses one HCL file.
func parseBody(fsys afero.Fs, file string) (*hclsyntax.Body, bool) {
	content, err := afero.ReadFile(fsys, file)
	if err != nil {
		return nil, false
	}
	parsed, diags := hclsyntax.ParseConfig(content, file, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, false
	}
	body, ok := parsed.Body.(*hclsyntax.Body)
	return body, ok
}

// callsSensitive reports whether node calls sensitive(...) anywhere below it.
func callsSensitive(node hclsyntax.Node) bool {
	found := false
	_ = hclsyntax.VisitAll(node, func(node hclsyntax.Node) hcl.Diagnostics {
		if call, ok := node.(*hclsyntax.FunctionCallExpr); ok && call.Name == "sensitive" {
			found = true
		}
		return nil
	})
	return found
}
