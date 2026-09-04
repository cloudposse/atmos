package format

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	listtree "github.com/cloudposse/atmos/pkg/list/tree"
	uitree "github.com/cloudposse/atmos/pkg/ui/tree"
)

// treeRows returns the rendered tree rows with ANSI stripped and the header row removed,
// which is the input uitree.Violations expects.
func treeRows(t *testing.T, output string) []string {
	t.Helper()
	rows := strings.Split(strings.TrimRight(stripANSI(output), "\n"), "\n")
	require.Greater(t, len(rows), 1, "expected a header row followed by tree rows")
	return rows[1:]
}

// TestRenderStacksTree_IsConnected asserts the stacks tree, with nested imports and the
// spacers between stacks, satisfies pkg/ui/tree's connectivity invariant.
func TestRenderStacksTree_IsConnected(t *testing.T) {
	output := RenderStacksTree(map[string][]*listtree.ImportNode{
		"dev": {
			{Path: "orgs/acme/plat/dev/_defaults", Children: []*listtree.ImportNode{
				{Path: "orgs/acme/plat/_defaults", Children: []*listtree.ImportNode{
					{Path: "orgs/acme/_defaults"},
				}},
				{Path: "mixins/region"},
			}},
			{Path: "catalog/vpc"},
		},
		"prod":    {{Path: "orgs/acme/plat/prod/_defaults"}},
		"staging": {{Path: "orgs/acme/plat/staging/_defaults", Circular: true}},
	}, true)

	assert.Empty(t, uitree.Violations(treeRows(t, output)), "\n%s", stripANSI(output))
}

// TestRenderInstancesTree_IsConnected asserts the instances tree, with imports shown and
// the nested spacers between a stack's components, satisfies the connectivity invariant.
func TestRenderInstancesTree_IsConnected(t *testing.T) {
	output := RenderInstancesTree(map[string]map[string][]*listtree.ImportNode{
		"stack-a": {
			"vpc": {{Path: "catalog/vpc", Children: []*listtree.ImportNode{{Path: "mixins/region"}}}},
			"eks": {{Path: "catalog/eks"}},
		},
		"stack-b": {
			"vpc": {{Path: "catalog/vpc"}},
		},
	}, true)

	assert.Empty(t, uitree.Violations(treeRows(t, output)), "\n%s", stripANSI(output))
}

// TestRenderInstancesTree_NestedSpacerKeepsAncestorRail is a regression test: a spacer
// between two components of a stack that has further stacks below it sits one level deep,
// so it must carry both the stack-level rail and its own. The previous cleanup counted
// leading spaces and emitted a single bar, dropping the ancestor rail.
func TestRenderInstancesTree_NestedSpacerKeepsAncestorRail(t *testing.T) {
	output := RenderInstancesTree(map[string]map[string][]*listtree.ImportNode{
		"stack-a": {
			"eks": {{Path: "catalog/eks"}},
			"vpc": {{Path: "catalog/vpc"}},
		},
		"stack-b": {
			"vpc": {{Path: "catalog/vpc"}},
		},
	}, true)

	rows := treeRows(t, output)
	found := false
	for _, row := range rows {
		if strings.Trim(row, "│ ") == "" && strings.Count(row, "│") == 2 {
			found = true
			break
		}
	}
	assert.True(t, found, "nested component spacer must keep the stack-level rail and add its own:\n%s", strings.Join(rows, "\n"))
}

// TestRenderDependenciesTree_IsConnected asserts the dependencies tree, with both
// directions and nested children, satisfies the connectivity invariant.
func TestRenderDependenciesTree_IsConnected(t *testing.T) {
	output := RenderDependenciesTree([]*DepTreeEntry{
		{
			Component: "app", Stack: "dev", Type: "terraform",
			DependsOn: []*DepTreeNode{
				{Component: "db", Stack: "dev", Type: "terraform", Children: []*DepTreeNode{
					{Component: "network", Stack: "shared", Type: "helmfile"},
					{Component: "kms", Stack: "shared", Type: "terraform"},
				}},
				{Component: "cache", Stack: "dev", Type: "terraform"},
			},
			RequiredBy: []*DepTreeNode{
				{Component: "web", Stack: "dev", Type: "terraform"},
			},
		},
	})

	assert.Empty(t, uitree.Violations(treeRows(t, output)), "\n%s", stripANSI(output))
}
