package rules

import (
	"fmt"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
)

// l09CycleRule detects inheritance cycles in component metadata.inherits chains.
type l09CycleRule struct{}

func newL09CycleRule() lint.LintRule {
	return &l09CycleRule{}
}

func (r *l09CycleRule) ID() string   { return "L-09" }
func (r *l09CycleRule) Name() string { return "Inheritance Cycle Detection" }
func (r *l09CycleRule) Description() string {
	return "Detects circular inheritance chains in component metadata.inherits definitions."
}
func (r *l09CycleRule) Severity() lint.Severity { return lint.SeverityError }
func (r *l09CycleRule) AutoFixable() bool       { return false }

func (r *l09CycleRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	// Build an adjacency map: component -> list of parents it inherits from.
	// We look at all components across all stack manifests.
	edges := make(map[string][]string)

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
				metadataSection, ok := compMap[cfg.MetadataSectionName].(map[string]any)
				if !ok {
					continue
				}
				inherits := extractInherits(metadataSection)
				if len(inherits) > 0 {
					existing := edges[compName]
					for _, parent := range inherits {
						existing = appendIfMissing(existing, parent)
					}
					edges[compName] = existing
				}
			}
		}
	}

	// Run DFS cycle detection.
	var findings []lint.LintFinding
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	cyclePaths := make(map[string]bool)

	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		if inStack[node] {
			// Cycle detected — find where it begins and report.
			cycleStart := -1
			for i, p := range path {
				if p == node {
					cycleStart = i
					break
				}
			}
			cyclePath := append(path[cycleStart:], node)
			cycleKey := fmt.Sprintf("%v", cyclePath)
			if !cyclePaths[cycleKey] {
				cyclePaths[cycleKey] = true
				findings = append(findings, lint.LintFinding{
					RuleID:    r.ID(),
					Severity:  r.Severity(),
					Message:   fmt.Sprintf("Inheritance cycle detected: %s", formatCyclePath(cyclePath)),
					Component: node,
					FixHint:   "Break the cycle by removing one of the inherits entries",
				})
			}
			return
		}
		if visited[node] {
			return
		}
		visited[node] = true
		inStack[node] = true
		for _, parent := range edges[node] {
			dfs(parent, append(path, node))
		}
		inStack[node] = false
	}

	for node := range edges {
		dfs(node, []string{})
	}

	return findings, nil
}

// formatCyclePath renders a cycle as "A → B → C → A".
func formatCyclePath(path []string) string {
	result := path[0]
	for _, p := range path[1:] {
		result += " → " + p
	}
	return result
}
