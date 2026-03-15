package rules

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/lint"
)

const defaultMaxImportDepth = 3

// l03ImportDepthRule warns when a logical stack has an import depth that exceeds a configurable threshold.
type l03ImportDepthRule struct{}

func newL03ImportDepthRule() lint.LintRule {
	return &l03ImportDepthRule{}
}

func (r *l03ImportDepthRule) ID() string   { return "L-03" }
func (r *l03ImportDepthRule) Name() string { return "Import Depth Warning" }
func (r *l03ImportDepthRule) Description() string {
	return "Warns when the import graph depth for a stack file exceeds the configured threshold."
}
func (r *l03ImportDepthRule) Severity() lint.Severity { return lint.SeverityWarning }
func (r *l03ImportDepthRule) AutoFixable() bool       { return false }

func (r *l03ImportDepthRule) Run(ctx lint.LintContext) ([]lint.LintFinding, error) {
	threshold := ctx.LintConfig.MaxImportDepth
	if threshold <= 0 {
		threshold = defaultMaxImportDepth
	}

	var findings []lint.LintFinding

	// For each file that imports others, measure max depth using BFS.
	for rootFile, imports := range ctx.ImportGraph {
		if len(imports) == 0 {
			continue
		}
		depth := maxImportDepth(rootFile, ctx.ImportGraph)
		if depth > threshold {
			findings = append(findings, lint.LintFinding{
				RuleID:   r.ID(),
				Severity: r.Severity(),
				File:     rootFile,
				Message:  fmt.Sprintf("Stack file '%s' has import depth %d which exceeds the configured threshold of %d", rootFile, depth, threshold),
				FixHint:  fmt.Sprintf("Reduce nesting by flattening imports or increasing max_import_depth in the lint config"),
			})
		}
	}

	return findings, nil
}

// maxImportDepth computes the maximum import chain depth starting from root.
func maxImportDepth(root string, graph map[string][]string) int {
	visited := make(map[string]bool)
	return dfsDepth(root, graph, visited)
}

func dfsDepth(node string, graph map[string][]string, visited map[string]bool) int {
	if visited[node] {
		return 0
	}
	visited[node] = true
	maxChild := 0
	for _, child := range graph[node] {
		d := dfsDepth(child, graph, visited)
		if d > maxChild {
			maxChild = d
		}
	}
	visited[node] = false
	return maxChild + 1
}
