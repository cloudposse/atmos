package rules

import (
	"fmt"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
)

// l04AbstractLeakRule detects abstract components that have no concrete inheritor
// in any stack across the entire StacksMap.  Checking cross-stack avoids false
// positives on catalog-pattern repos where abstract base components live in
// catalog/ files and their concrete children live in separate deploy/ stacks.
type l04AbstractLeakRule struct{}

func newL04AbstractLeakRule() lint.LintRule {
	return &l04AbstractLeakRule{}
}

func (r *l04AbstractLeakRule) ID() string   { return "L-04" }
func (r *l04AbstractLeakRule) Name() string { return "Abstract Component Leak" }
func (r *l04AbstractLeakRule) Description() string {
	return "Finds abstract components that have no concrete inheritor in any stack. " +
		"These components are silently skipped at deploy time."
}
func (r *l04AbstractLeakRule) Severity() lint.Severity { return lint.SeverityError }
func (r *l04AbstractLeakRule) AutoFixable() bool       { return false }

func (r *l04AbstractLeakRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	// First pass: collect all abstract components and all components that inherit
	// from something, scanning across every stack and component type.
	// abstractLocation records the first stack where each abstract component was seen.
	abstractLocation := make(map[string]string) // compName → stackName
	// inheritedBy maps abstract component name → list of concrete component names.
	inheritedBy := make(map[string][]string)

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
				compType_, _ := metadataSection["type"].(string)

				if compType_ == "abstract" {
					if _, seen := abstractLocation[compName]; !seen {
						abstractLocation[compName] = stackName
					}
				}

				for _, parent := range extractInherits(metadataSection) {
					if compType_ != "abstract" {
						inheritedBy[parent] = append(inheritedBy[parent], compName)
					}
				}
			}
		}
	}

	// Second pass: report abstract components that have no concrete inheritor anywhere.
	var findings []lint.LintFinding
	for abstractComp, stackName := range abstractLocation {
		if len(inheritedBy[abstractComp]) == 0 {
			findings = append(findings, lint.LintFinding{
				RuleID:    r.ID(),
				Severity:  r.Severity(),
				Message:   fmt.Sprintf("Abstract component '%s' (first seen in stack '%s') has no concrete inheritor in any stack and will be silently skipped at deploy time", abstractComp, stackName),
				Component: abstractComp,
				Stack:     stackName,
				FixHint:   "Either remove this abstract component or create a concrete component that inherits from it",
			})
		}
	}

	return findings, nil
}
