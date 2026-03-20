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

func (r *l07OrphanedFileRule) ID() string   { return "L-07" }
func (r *l07OrphanedFileRule) Name() string { return "Orphaned Catalog File Detection" }
func (r *l07OrphanedFileRule) Description() string {
	return "Finds YAML files under the stacks base path that are not referenced by any import chain."
}
func (r *l07OrphanedFileRule) Severity() lint.Severity { return lint.SeverityWarning }
func (r *l07OrphanedFileRule) AutoFixable() bool       { return false }

func (r *l07OrphanedFileRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	// Build the set of all referenced files, normalized to paths relative to
	// StacksBasePath so that absolute import-graph keys and relative import values
	// can be compared uniformly.
	referenced := make(map[string]bool)

	// The import graph keys are the files that appear as importers (absolute paths);
	// the values are the files they import (relative logical names, possibly without extension).
	for importer, imports := range ctx.ImportGraph {
		referenced[relNorm(importer, ctx.StacksBasePath)] = true
		for _, imp := range imports {
			referenced[relNorm(imp, ctx.StacksBasePath)] = true
		}
	}

	// Also mark all files in StacksMap and RawStackConfigs as referenced (root files).
	for key := range ctx.StacksMap {
		referenced[relNorm(key, ctx.StacksBasePath)] = true
	}
	for key := range ctx.RawStackConfigs {
		referenced[relNorm(key, ctx.StacksBasePath)] = true
	}

	var findings []lint.LintFinding
	for _, file := range ctx.AllStackFiles {
		norm := relNorm(file, ctx.StacksBasePath)
		if !referenced[norm] {
			// Trim the base path for a shorter display path.
			displayPath := file
			if ctx.StacksBasePath != "" {
				if rel, err := filepath.Rel(ctx.StacksBasePath, file); err == nil {
					displayPath = filepath.ToSlash(rel)
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

// relNorm converts path to a normalized form relative to basePath for consistent
// cross-platform comparison. Absolute paths are first made relative to basePath;
// relative paths are used as-is. YAML extensions are stripped so that import
// values (which often omit extensions) match physical file names.
// When basePath is empty, absolute paths remain absolute after normalization.
func relNorm(path, basePath string) string {
	if filepath.IsAbs(path) && basePath != "" {
		if rel, err := filepath.Rel(basePath, path); err == nil {
			path = rel
		}
	}
	return normalizeForComparison(filepath.ToSlash(path))
}

// normalizeForComparison strips common path variations for robust comparison.
// It removes YAML extensions so that import keys (which often omit extensions)
// match physical file names. Uses forward slashes for OS-agnostic comparison.
// Trailing slashes are removed by filepath.Clean.
func normalizeForComparison(path string) string {
	// Remove common YAML extensions for comparison since import keys may omit them.
	base := path
	for _, ext := range []string{".yaml", ".yml"} {
		if len(base) > len(ext) && base[len(base)-len(ext):] == ext {
			base = base[:len(base)-len(ext)]
			break
		}
	}
	// Clean with forward slashes for OS-agnostic comparison.
	return filepath.ToSlash(filepath.Clean(base))
}
