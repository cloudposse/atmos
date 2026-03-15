package rules

import (
	"fmt"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/lint"
)

// l07OrphanedFileRule detects YAML files under the stacks base path that are
// not reachable from any import chain.
type l07OrphanedFileRule struct{}

func newL07OrphanedFileRule() lint.LintRule {
	return &l07OrphanedFileRule{}
}

func (r *l07OrphanedFileRule) ID() string          { return "L-07" }
func (r *l07OrphanedFileRule) Name() string        { return "Orphaned Catalog File Detection" }
func (r *l07OrphanedFileRule) Description() string {
	return "Finds YAML files under the stacks base path that are not referenced by any import chain."
}
func (r *l07OrphanedFileRule) Severity() lint.Severity { return lint.SeverityWarning }
func (r *l07OrphanedFileRule) AutoFixable() bool       { return false }

func (r *l07OrphanedFileRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	// Build the set of all files referenced in any import chain.
	referenced := make(map[string]bool)

	// The import graph keys are the files that appear as importers;
	// the values are the files they import.
	for importer, imports := range ctx.ImportGraph {
		referenced[normalizeForComparison(importer)] = true
		for _, imp := range imports {
			referenced[normalizeForComparison(imp)] = true
		}
	}

	// Also mark all files in StacksMap as referenced (they are the root files).
	for key := range ctx.StacksMap {
		referenced[normalizeForComparison(key)] = true
	}
	for key := range ctx.RawStackConfigs {
		referenced[normalizeForComparison(key)] = true
	}

	var findings []lint.LintFinding
	for _, file := range ctx.AllStackFiles {
		norm := normalizeForComparison(file)
		if !referenced[norm] {
			// Trim the base path for a shorter display path.
			displayPath := file
			if ctx.StacksBasePath != "" {
				if rel, err := filepath.Rel(ctx.StacksBasePath, file); err == nil {
					displayPath = rel
				}
			}
			findings = append(findings, lint.LintFinding{
				RuleID:   r.ID(),
				Severity: r.Severity(),
				File:     displayPath,
				Message:  fmt.Sprintf("Stack file '%s' is not referenced by any import chain and may be orphaned", displayPath),
				FixHint:  "Either import this file from a parent stack or remove it if it is no longer needed",
			})
		}
	}

	return findings, nil
}

// normalizeForComparison strips common path variations for robust comparison.
func normalizeForComparison(path string) string {
	// Remove trailing slash.
	if len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	// Remove common YAML extensions for comparison since import keys may omit them.
	base := path
	for _, ext := range []string{".yaml", ".yml"} {
		if len(base) > len(ext) && base[len(base)-len(ext):] == ext {
			base = base[:len(base)-len(ext)]
			break
		}
	}
	return filepath.Clean(base)
}
