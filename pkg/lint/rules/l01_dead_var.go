package rules

import (
	"fmt"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
)

// l01DeadVarRule detects top-level vars declared in stack files that are not
// consumed by any component across the entire StacksMap.  A cross-stack check is
// used because Atmos delivers global vars to components via deep-merge: a
// component that relies on a globally-set var will have that var in its merged
// vars section, so checking only within the stack that declares the var would
// generate false positives in standard catalog/deploy split repos.
type l01DeadVarRule struct{}

func newL01DeadVarRule() lint.LintRule {
	return &l01DeadVarRule{}
}

func (r *l01DeadVarRule) ID() string   { return "L-01" }
func (r *l01DeadVarRule) Name() string { return "Dead Var Detection" }
func (r *l01DeadVarRule) Description() string {
	return "Detects top-level vars declared in stack files that are not consumed by any component across all stacks."
}
func (r *l01DeadVarRule) Severity() lint.Severity { return lint.SeverityWarning }
func (r *l01DeadVarRule) AutoFixable() bool       { return false }

func (r *l01DeadVarRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	// First pass: collect every var key that appears in any component's vars
	// across all stacks.  Because Atmos deep-merges global vars into component
	// vars, a component that consumes a global var will have that key in its
	// merged vars section.
	usedVarKeys := make(map[string]bool)
	for _, stackSection := range ctx.StacksMap {
		stackMap, ok := stackSection.(map[string]any)
		if !ok {
			continue
		}
		componentsSection, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
		if !ok {
			continue
		}
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

	// Second pass: flag global vars in each stack that are never seen in any
	// component's vars anywhere across all stacks.
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
			if !usedVarKeys[varKey] {
				findings = append(findings, lint.LintFinding{
					RuleID:   r.ID(),
					Severity: r.Severity(),
					Message:  fmt.Sprintf("Stack '%s' declares global var '%s' that is not consumed by any component in any stack", stackName, varKey),
					Stack:    stackName,
					FixHint:  fmt.Sprintf("Remove or relocate the global var '%s' if it is not needed", varKey),
				})
			}
		}
	}

	return findings, nil
}
