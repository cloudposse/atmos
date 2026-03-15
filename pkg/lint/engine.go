package lint

import (
	"sort"
)

// Engine manages and runs lint rules against a LintContext.
type Engine struct {
	rules []LintRule
}

// NewEngine creates a new Engine with the given rules.
func NewEngine(rules []LintRule) *Engine {
	return &Engine{rules: rules}
}

// Run executes all registered rules against the provided context.
// If ruleIDs is non-empty, only the specified rules are run.
// The minSeverity filter excludes findings below the given severity level.
func (e *Engine) Run(ctx LintContext, ruleIDs []string, minSeverity Severity) (*LintResult, error) {
	result := &LintResult{}

	// Build a set of requested rule IDs for O(1) lookup.
	ruleFilter := make(map[string]bool)
	for _, id := range ruleIDs {
		ruleFilter[id] = true
	}

	for _, rule := range e.rules {
		// Skip rules not in the filter if a filter was provided.
		if len(ruleFilter) > 0 && !ruleFilter[rule.ID()] {
			continue
		}

		findings, err := rule.Run(ctx)
		if err != nil {
			return nil, err
		}

		for _, f := range findings {
			// Apply configured severity override if present.
			if override, ok := ctx.LintConfig.Rules[f.RuleID]; ok {
				f.Severity = Severity(override)
			}

			// Apply minimum severity filter.
			if f.Severity.Level() < minSeverity.Level() {
				continue
			}

			result.Findings = append(result.Findings, f)

			switch f.Severity {
			case SeverityError:
				result.Summary.Errors++
			case SeverityWarning:
				result.Summary.Warnings++
			case SeverityInfo:
				result.Summary.Info++
			}
		}
	}

	// Sort findings: errors first, then warnings, then info; within each severity by file then rule ID.
	sort.Slice(result.Findings, func(i, j int) bool {
		fi, fj := result.Findings[i], result.Findings[j]
		if fi.Severity.Level() != fj.Severity.Level() {
			return fi.Severity.Level() > fj.Severity.Level()
		}
		if fi.File != fj.File {
			return fi.File < fj.File
		}
		return fi.RuleID < fj.RuleID
	})

	return result, nil
}

// DefaultRules returns the full set of default lint rules.
// NOTE: This is intentionally empty in the base package to avoid circular imports.
// Callers should use pkg/lint/rules.All() to get the default rules.
func DefaultRules() []LintRule {
	return nil
}
