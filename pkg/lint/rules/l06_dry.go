package rules

import (
	"fmt"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
)

const defaultDRYThresholdPct = 80

// l06DRYRule detects variable values that are repeated across many stacks and suggests
// extracting them to a catalog file.
type l06DRYRule struct{}

func newL06DRYRule() lint.LintRule {
	return &l06DRYRule{}
}

func (r *l06DRYRule) ID() string          { return "L-06" }
func (r *l06DRYRule) Name() string        { return "DRY Extraction Opportunity" }
func (r *l06DRYRule) Description() string {
	return "Identifies variable values repeated across many stacks that could be extracted to a catalog file."
}
func (r *l06DRYRule) Severity() lint.Severity { return lint.SeverityInfo }
func (r *l06DRYRule) AutoFixable() bool       { return false }

func (r *l06DRYRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	thresholdPct := ctx.LintConfig.DRYThresholdPct
	if thresholdPct <= 0 {
		thresholdPct = defaultDRYThresholdPct
	}

	// Maps component name -> varKey -> value string -> count of stacks with that value.
	stats := make(map[string]map[string]map[string]int)
	// Maps component name -> varKey -> total stacks count.
	totals := make(map[string]map[string]int)

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
			for compName, compData := range compSection {
				compMap, ok := compData.(map[string]any)
				if !ok {
					continue
				}
				vars, ok := compMap[cfg.VarsSectionName].(map[string]any)
				if !ok {
					continue
				}

				if stats[compName] == nil {
					stats[compName] = make(map[string]map[string]int)
				}
				if totals[compName] == nil {
					totals[compName] = make(map[string]int)
				}

				for varKey, varVal := range vars {
					valStr := fmt.Sprintf("%v", varVal)
					if stats[compName][varKey] == nil {
						stats[compName][varKey] = make(map[string]int)
					}
					stats[compName][varKey][valStr]++
					totals[compName][varKey]++
				}
			}
		}
	}

	var findings []lint.LintFinding
	seen := make(map[string]bool)

	for compName, varKeys := range stats {
		for varKey, valueCounts := range varKeys {
			total := totals[compName][varKey]
			if total < 2 {
				continue
			}
			for value, count := range valueCounts {
				pct := count * 100 / total
				if pct >= thresholdPct {
					key := fmt.Sprintf("%s.%s=%s", compName, varKey, value)
					if seen[key] {
						continue
					}
					seen[key] = true
					findings = append(findings, lint.LintFinding{
						RuleID:    r.ID(),
						Severity:  r.Severity(),
						Component: compName,
						Message:   fmt.Sprintf("Component '%s' var '%s' has value '%s' in %d%% of stacks (%d/%d) — consider extracting to a catalog base component", compName, varKey, value, pct, count, total),
						FixHint:   fmt.Sprintf("Extract var '%s: %s' to a shared catalog component that '%s' inherits from", varKey, value, compName),
					})
				}
			}
		}
	}

	return findings, nil
}
