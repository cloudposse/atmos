package rules

import (
	"fmt"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
)

// l04AbstractLeakRule detects abstract components present in deployable stacks
// that have no concrete inheritor within the same stack.
type l04AbstractLeakRule struct{}

func newL04AbstractLeakRule() lint.LintRule {
	return &l04AbstractLeakRule{}
}

func (r *l04AbstractLeakRule) ID() string   { return "L-04" }
func (r *l04AbstractLeakRule) Name() string { return "Abstract Component Leak" }
func (r *l04AbstractLeakRule) Description() string {
	return "Finds abstract components present in deployable stacks with no concrete inheritor. " +
		"These components are silently skipped at deploy time."
}
func (r *l04AbstractLeakRule) Severity() lint.Severity { return lint.SeverityError }
func (r *l04AbstractLeakRule) AutoFixable() bool       { return false }

func (r *l04AbstractLeakRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	var findings []lint.LintFinding

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

			// Collect abstract components and components that inherit from them.
			abstractComponents := make(map[string]bool)
			inheritedBy := make(map[string][]string)

			for compName, compData := range compSection {
				compMap, ok := compData.(map[string]any)
				if !ok {
					continue
				}
				metadataSection, _ := compMap[cfg.MetadataSectionName].(map[string]any)

				if metadataType, _ := metadataSection["type"].(string); metadataType == "abstract" {
					abstractComponents[compName] = true
				}

				for _, parent := range extractInherits(metadataSection) {
					inheritedBy[parent] = append(inheritedBy[parent], compName)
				}
			}

			// Report abstract components that have no concrete inheritor in this stack.
			for abstractComp := range abstractComponents {
				children := inheritedBy[abstractComp]
				hasConcreteChild := false
				for _, child := range children {
					// children are only added to inheritedBy when compData was a valid
					// map[string]any, so this type assertion should always succeed.
					// The guard is retained as a defensive safeguard against data corruption.
					childData, ok := compSection[child].(map[string]any)
					if !ok {
						continue
					}
					childMeta, _ := childData[cfg.MetadataSectionName].(map[string]any)
					if childType, _ := childMeta["type"].(string); childType != "abstract" {
						hasConcreteChild = true
						break
					}
				}

				if !hasConcreteChild {
					findings = append(findings, lint.LintFinding{
						RuleID:    r.ID(),
						Severity:  r.Severity(),
						Message:   fmt.Sprintf("Abstract component '%s' in stack '%s' has no concrete inheritor and will be silently skipped at deploy time", abstractComp, stackName),
						Component: abstractComp,
						Stack:     stackName,
						FixHint:   "Either remove this abstract component from the stack or create a concrete component that inherits from it",
					})
				}
			}
		}
	}

	return findings, nil
}
