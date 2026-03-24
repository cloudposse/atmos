package rules

import (
	"fmt"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/lint"
	"github.com/cloudposse/atmos/pkg/lint/pathnorm"
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

	// Mark all files in RawStackConfigs as referenced (root/entry-point files).
	// Note: StacksMap keys are logical stack names (e.g. "plat-ue2-prod"), NOT file
	// paths. relNorm("plat-ue2-prod", "/stacks") → "plat-ue2-prod", which never
	// matches a physical file path. RawStackConfigs keys are absolute paths and
	// correctly protect root stacks from being flagged as orphans.
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
// cross-platform comparison. It delegates to pathnorm.NormalizeRelNoExt, which is
// the single authoritative implementation shared with the exec package.
//
// Keeping this thin wrapper avoids having to update every call-site in this file
// while still eliminating the duplicate implementation.
func relNorm(path, basePath string) string {
	return pathnorm.NormalizeRelNoExt(path, basePath)
}

// normalizeForComparison is kept for backward compatibility with tests that call
// ExportedNormalizeForComparison. It delegates to pathnorm.NormalizeRelNoExt with
// an empty basePath (i.e. it only strips extensions and cleans the path).
func normalizeForComparison(path string) string {
	return pathnorm.NormalizeRelNoExt(path, "")
}
