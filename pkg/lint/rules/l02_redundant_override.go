package rules

import (
	"fmt"
	"reflect"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
)

// l02RedundantOverrideRule detects variables that override a value with the same value
// that the parent would have provided (redundant no-op overrides).
type l02RedundantOverrideRule struct{}

func newL02RedundantOverrideRule() lint.LintRule {
	return &l02RedundantOverrideRule{}
}

func (r *l02RedundantOverrideRule) ID() string          { return "L-02" }
func (r *l02RedundantOverrideRule) Name() string        { return "Redundant No-Op Override" }
func (r *l02RedundantOverrideRule) Description() string {
	return "Detects vars that are set to the same value they would have inherited from the parent component, making them redundant."
}
func (r *l02RedundantOverrideRule) Severity() lint.Severity { return lint.SeverityWarning }
func (r *l02RedundantOverrideRule) AutoFixable() bool       { return true }

func (r *l02RedundantOverrideRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	var findings []lint.LintFinding

	// Build a map of component base vars from abstract/catalog components.
	// Key: component name, Value: vars map.
	baseVars := make(map[string]map[string]any)

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
				metadataSection, _ := compMap[cfg.MetadataSectionName].(map[string]any)
				if metadataType, _ := metadataSection["type"].(string); metadataType != "abstract" {
					continue
				}
				if vars, ok := compMap[cfg.VarsSectionName].(map[string]any); ok {
					baseVars[compName] = vars
				}
			}
		}
	}

	// Check concrete components: for each var that inherits from a parent,
	// if the var value matches the parent's value, it's redundant.
	for stackName, stackSection := range ctx.StacksMap {
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
				metadataSection, _ := compMap[cfg.MetadataSectionName].(map[string]any)
				if metadataType, _ := metadataSection["type"].(string); metadataType == "abstract" {
					continue
				}

				inherits := extractInherits(metadataSection)
				if len(inherits) == 0 {
					continue
				}

				componentVars, ok := compMap[cfg.VarsSectionName].(map[string]any)
				if !ok {
					continue
				}

				// For each parent in the inheritance chain, check for redundant overrides.
				for _, parent := range inherits {
					parentVars, hasParent := baseVars[parent]
					if !hasParent {
						continue
					}
					for varKey, varVal := range componentVars {
						parentVal, parentHas := parentVars[varKey]
						if !parentHas {
							continue
						}
						if reflect.DeepEqual(varVal, parentVal) {
							findings = append(findings, lint.LintFinding{
								RuleID:    r.ID(),
								Severity:  r.Severity(),
								Message:   fmt.Sprintf("Component '%s' in stack '%s' redundantly overrides var '%s' with the same value as parent '%s'", compName, stackName, varKey, parent),
								Component: compName,
								Stack:     stackName,
								FixHint:   fmt.Sprintf("Remove the redundant '%s' key from the vars section of component '%s'", varKey, compName),
							})
						}
					}
				}
			}
		}
	}

	return findings, nil
}
