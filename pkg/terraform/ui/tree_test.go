package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColorizedActionSymbol(t *testing.T) {
	tests := []struct {
		action   string
		expected string
	}{
		{"create", "●"},  // Colored dot for create.
		{"update", "●"},  // Colored dot for update.
		{"delete", "●"},  // Colored dot for delete.
		{"read", "●"},    // Colored dot for read.
		{"no-op", " "},   // Space for no-op.
		{"unknown", " "}, // Space for unknown.
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			result := colorizedActionSymbol(tt.action)
			// The result includes ANSI codes, but should contain the expected symbol.
			assert.Contains(t, result, tt.expected)
		})
	}
}

func TestDependencyTree_RenderTree_Simple(t *testing.T) {
	tree := &DependencyTree{
		Root: &TreeNode{
			Address: "root",
			Children: []*TreeNode{
				{Address: "local_file.cache", Action: "create"},
			},
		},
		Stack:     "dev",
		Component: "myapp",
	}

	result := tree.RenderTree()

	// Should contain the stack/component header.
	assert.Contains(t, result, "dev/myapp")
	// Should contain the resource.
	assert.Contains(t, result, "local_file.cache")
	// Should contain the tree connector.
	assert.Contains(t, result, "└─")
}

func TestDependencyTree_RenderTree_MultipleResources(t *testing.T) {
	tree := &DependencyTree{
		Root: &TreeNode{
			Address: "root",
			Children: []*TreeNode{
				{
					Address: "aws_vpc.main",
					Action:  "create",
					Children: []*TreeNode{
						{Address: "aws_subnet.public[0]", Action: "create"},
						{Address: "aws_subnet.public[1]", Action: "create"},
					},
				},
				{Address: "aws_security_group.default", Action: "update"},
			},
		},
		Stack:     "plat-ue2-dev",
		Component: "vpc",
	}

	result := tree.RenderTree()

	// Should contain all resources.
	assert.Contains(t, result, "aws_vpc.main")
	assert.Contains(t, result, "aws_subnet.public[0]")
	assert.Contains(t, result, "aws_subnet.public[1]")
	assert.Contains(t, result, "aws_security_group.default")

	// Should contain tree connectors.
	assert.Contains(t, result, "├─")
	assert.Contains(t, result, "└─")
	assert.Contains(t, result, "│")
}

func TestDependencyTree_GetChangeSummary(t *testing.T) {
	tree := &DependencyTree{
		Root: &TreeNode{
			Address: "root",
			Children: []*TreeNode{
				{Address: "aws_vpc.main", Action: "create"},
				{
					Address: "aws_subnet.public",
					Action:  "create",
					Children: []*TreeNode{
						{Address: "aws_instance.web", Action: "update"},
					},
				},
				{Address: "aws_instance.old", Action: "delete"},
			},
		},
	}

	add, change, remove := tree.GetChangeSummary()

	assert.Equal(t, 2, add)    // aws_vpc.main, aws_subnet.public
	assert.Equal(t, 1, change) // aws_instance.web
	assert.Equal(t, 1, remove) // aws_instance.old
}

func TestSortChildren(t *testing.T) {
	root := &TreeNode{
		Address: "root",
		Children: []*TreeNode{
			{Address: "z_resource"},
			{Address: "a_resource"},
			{Address: "m_resource"},
		},
	}

	sortChildren(root)

	assert.Equal(t, "a_resource", root.Children[0].Address)
	assert.Equal(t, "m_resource", root.Children[1].Address)
	assert.Equal(t, "z_resource", root.Children[2].Address)
}

func TestRenderChildren_Empty(t *testing.T) {
	var b strings.Builder
	// No styling in test for simplicity.
	renderChildren(&b, nil, "", defaultTreeStyle())

	assert.Empty(t, b.String())
}

func TestRenderChildren_SingleNode(t *testing.T) {
	var b strings.Builder
	nodes := []*TreeNode{
		{Address: "aws_vpc.main", Action: "create"},
	}

	renderChildren(&b, nodes, "", defaultTreeStyle())

	result := b.String()
	assert.Contains(t, result, "aws_vpc.main")
	assert.Contains(t, result, "└─") // Last (and only) child uses └─
}

func TestRenderChildren_MultipleNodes(t *testing.T) {
	var b strings.Builder
	nodes := []*TreeNode{
		{Address: "aws_vpc.main", Action: "create"},
		{Address: "aws_security_group.default", Action: "update"},
	}

	renderChildren(&b, nodes, "", defaultTreeStyle())

	result := b.String()
	assert.Contains(t, result, "aws_vpc.main")
	assert.Contains(t, result, "aws_security_group.default")
	assert.Contains(t, result, "├─") // First child uses ├─
	assert.Contains(t, result, "└─") // Last child uses └─
}
