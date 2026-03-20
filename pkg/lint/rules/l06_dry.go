package rules

import (
	"fmt"
	"strings"

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

func (r *l06DRYRule) ID() string   { return "L-06" }
func (r *l06DRYRule) Name() string { return "DRY Extraction Opportunity" }
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
					// Include the type in the map key to avoid false matches between
					// different-typed values that have the same Sprintf representation
					// (e.g., bool true vs string "true").
					valStr := fmt.Sprintf("%T:%v", varVal, varVal)
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

	for compName, varKeys := range stats {
		for varKey, valueCounts := range varKeys {
			total := totals[compName][varKey]
			if total < 2 {
				continue
			}
			for value, count := range valueCounts {
				pct := count * 100 / total
				if pct >= thresholdPct {
				// Strip the "type:" prefix added for deduplication before displaying.
					// Use SplitN to handle values that contain colons (e.g., URLs, timestamps).
					displayVal := value
					if parts := strings.SplitN(value, ":", 2); len(parts) == 2 {
						displayVal = parts[1]
					}
					findings = append(findings, lint.LintFinding{
						RuleID:    r.ID(),
						Severity:  r.Severity(),
						Component: compName,
						Message:   fmt.Sprintf("Component '%s' var '%s' has value '%s' in %d%% of stacks (%d/%d) — consider extracting to a catalog base component", compName, varKey, displayVal, pct, count, total),
						FixHint:   fmt.Sprintf("Extract var '%s: %s' to a shared catalog component that '%s' inherits from", varKey, displayVal, compName),
					})
				}
			}
		}
	}

	return findings, nil
}
