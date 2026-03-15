package rules

import (
	"fmt"
	"path/filepath"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
)

// defaultSensitiveVarPatterns lists built-in patterns to check.
var defaultSensitiveVarPatterns = []string{
	"*password*",
	"*secret*",
	"*token*",
	"*key*",
	"*arn*",
	"*account_id*",
	"*role*",
}

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
	patterns := ctx.LintConfig.SensitiveVarPatterns
	if len(patterns) == 0 {
		patterns = defaultSensitiveVarPatterns
	} else {
		// Merge with defaults (deduplicate).
		seen := make(map[string]bool)
		merged := make([]string, 0, len(defaultSensitiveVarPatterns)+len(patterns))
		for _, p := range append(defaultSensitiveVarPatterns, patterns...) {
			if !seen[p] {
				seen[p] = true
				merged = append(merged, p)
			}
		}
		patterns = merged
	}

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
				// Try to find the file for this stack.
				file := stackNameToFile(stackName, ctx.StacksBasePath)
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

// matchesSensitivePattern returns true if the var name matches any sensitive pattern.
func matchesSensitivePattern(varName string, patterns []string) bool {
	lower := strings.ToLower(varName)
	for _, pattern := range patterns {
		matched, err := filepath.Match(strings.ToLower(pattern), lower)
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
	// Stack name may already be a relative path.
	if strings.Contains(stackName, "/") || strings.HasSuffix(stackName, ".yaml") || strings.HasSuffix(stackName, ".yml") {
		return stackName
	}
	return ""
}
