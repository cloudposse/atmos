package rules

import (
	"fmt"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
)

// l01DeadVarRule detects top-level vars declared in stack files that are not
// consumed by any component in the import chain.
type l01DeadVarRule struct{}

func newL01DeadVarRule() lint.LintRule {
	return &l01DeadVarRule{}
}

func (r *l01DeadVarRule) ID() string   { return "L-01" }
func (r *l01DeadVarRule) Name() string { return "Dead Var Detection" }
func (r *l01DeadVarRule) Description() string {
	return "Detects top-level vars declared in stack files that are not consumed by any component in the import chain."
}
func (r *l01DeadVarRule) Severity() lint.Severity { return lint.SeverityWarning }
func (r *l01DeadVarRule) AutoFixable() bool       { return false }

func (r *l01DeadVarRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	var findings []lint.LintFinding

	for stackName, stackSection := range ctx.StacksMap {
		stackMap, ok := stackSection.(map[string]any)
		if !ok {
			continue
		}

		// Get top-level (global) vars for this stack.
		globalVars, ok := stackMap[cfg.VarsSectionName].(map[string]any)
		if !ok || len(globalVars) == 0 {
			continue
		}

		// Collect all var keys actually used by components in this stack.
		usedVarKeys := make(map[string]bool)
		componentsSection, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
		if ok {
			for _, compType := range []string{cfg.TerraformSectionName, cfg.HelmfileSectionName} {
				compSection, ok := componentsSection[compType].(map[string]any)
				if !ok {
					continue
				}
				for _, compData := range compSection {
					compMap, ok := compData.(map[string]any)
					if !ok {
						continue
					}
					if compVars, ok := compMap[cfg.VarsSectionName].(map[string]any); ok {
						for k := range compVars {
							usedVarKeys[k] = true
						}
					}
				}
			}
		}

		// Any global var key not present in any component vars is "dead".
		for varKey := range globalVars {
			if !usedVarKeys[varKey] {
				findings = append(findings, lint.LintFinding{
					RuleID:   r.ID(),
					Severity: r.Severity(),
					Message:  fmt.Sprintf("Stack '%s' declares global var '%s' that is not consumed by any component", stackName, varKey),
					Stack:    stackName,
					FixHint:  fmt.Sprintf("Remove or relocate the global var '%s' if it is not needed", varKey),
				})
			}
		}
	}

	return findings, nil
}
