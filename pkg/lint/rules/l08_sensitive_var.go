package rules

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
)

// l08SensitiveVarRule warns when sensitive-looking variable names appear at global stack scope.
type l08SensitiveVarRule struct{}

func newL08SensitiveVarRule() lint.LintRule {
	return &l08SensitiveVarRule{}
}

func (r *l08SensitiveVarRule) ID() string   { return "L-08" }
func (r *l08SensitiveVarRule) Name() string { return "Sensitive Var at Global Scope" }
func (r *l08SensitiveVarRule) Description() string {
	return "Warns when variable names matching sensitive patterns (passwords, secrets, tokens, etc.) appear at global stack scope instead of component scope."
}
func (r *l08SensitiveVarRule) Severity() lint.Severity { return lint.SeverityWarning }
func (r *l08SensitiveVarRule) AutoFixable() bool       { return false }

func (r *l08SensitiveVarRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	// Sensitive var patterns come from the merged lint config (defaults are applied
	// in mergedLintConfig in internal/exec/lint_stacks.go so this slice is never empty).
	patterns := ctx.LintConfig.SensitiveVarPatterns

	var findings []lint.LintFinding

	for stackName, stackSection := range ctx.StacksMap {
		stackMap, ok := stackSection.(map[string]any)
		if !ok {
			continue
		}

		globalVars, ok := stackMap[cfg.VarsSectionName].(map[string]any)
		if !ok || len(globalVars) == 0 {
			continue
		}

		for varKey := range globalVars {
			if matchesSensitivePattern(varKey, patterns) {
				// Resolve file attribution using the indexed maps in priority order:
				// 1. StackStemToFile: unambiguous full-stem lookup (e.g. "deploy/prod")
				// 2. StackNameToFileIndex: basename lookup with disambiguation heuristic
				// 3. stackNameToFile: heuristic fallback (handles path-separator names)
				file := resolveFileForStack(stackName, ctx)
				findings = append(findings, lint.LintFinding{
					RuleID:   r.ID(),
					Severity: r.Severity(),
					File:     file,
					Message:  fmt.Sprintf("Stack '%s' declares potentially sensitive variable '%s' at global scope", stackName, varKey),
					Stack:    stackName,
					FixHint:  fmt.Sprintf("Move '%s' to the component-level vars section to limit its scope", varKey),
				})
			}
		}
	}

	return findings, nil
}

// resolveFileForStack determines the best physical file path for a logical stack name
// using a priority cascade:
//  1. StackStemToFile: unambiguous lookup by full relative stem (e.g. "deploy/prod")
//  2. StackNameToFileIndex: basename lookup — if exactly one file matches, use it;
//     if multiple files match, prefer the one under a "deploy" directory, else omit File
//     to avoid attributing the finding to the wrong manifest.
//  3. stackNameToFile: heuristic fallback for names that contain path separators.
func resolveFileForStack(stackName string, ctx lint.LintContext) string {
	// 1. Try the full-stem index (unambiguous).
	if f, ok := ctx.StackStemToFile[stackName]; ok && f != "" {
		return f
	}

	// 2. Try the basename index with disambiguation.
	if files, ok := ctx.StackNameToFileIndex[stackName]; ok {
		switch len(files) {
		case 0:
			// nothing
		case 1:
			return files[0]
		default:
			// Multiple manifests share the basename — prefer one under a "deploy" or
			// "stacks" sub-directory to maximise relevance; if there is no clear winner,
			// return "" so the finding is not attributed to a potentially wrong file.
			var preferred string
			for _, f := range files {
				lower := strings.ToLower(filepath.ToSlash(f))
				if strings.Contains(lower, "/deploy/") || strings.Contains(lower, "/stacks/") {
					if preferred == "" {
						preferred = f
					}
				}
			}
			return preferred // "" when no heuristic winner; caller should handle empty File
		}
	}

	// 3. Heuristic fallback.
	return stackNameToFile(stackName, ctx.StacksBasePath)
}

// matchesSensitivePattern returns true if the var name matches any sensitive glob pattern.
// Returns false immediately when patterns is empty, making the no-patterns contract explicit.
// Uses path.Match (not filepath.Match) so that pattern matching is OS-agnostic — variable
// names are not file-system paths and must not be interpreted with the OS path separator.
func matchesSensitivePattern(varName string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	lower := strings.ToLower(varName)
	for _, pattern := range patterns {
		matched, err := path.Match(strings.ToLower(pattern), lower)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// stackNameToFile attempts to derive a file path from a stack name.
func stackNameToFile(stackName, basePath string) string {
	if basePath == "" {
		return stackName
	}
	// Stack name may already be a relative or absolute path (contains a path separator
	// on either Unix '/' or Windows '\', or has a YAML file extension).
	if strings.ContainsAny(stackName, `/\`) ||
		strings.HasSuffix(stackName, ".yaml") ||
		strings.HasSuffix(stackName, ".yml") {
		return stackName
	}
	return ""
}
