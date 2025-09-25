package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestBuildTerraformDependencyGraph(t *testing.T) {
	// Test building dependency graph from stacks
	stacks := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{
							"cidr": "10.0.0.0/16",
						},
					},
					"database": map[string]any{
						"vars": map[string]any{
							"engine": "postgres",
						},
						"settings": map[string]any{
							"depends_on": []any{
								map[string]any{
									"component": "vpc",
								},
							},
						},
					},
					"app": map[string]any{
						"vars": map[string]any{
							"replicas": 3,
						},
						"settings": map[string]any{
							"depends_on": []any{
								map[string]any{
									"component": "database",
								},
							},
						},
					},
				},
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}

	graph, err := buildTerraformDependencyGraph(atmosConfig, stacks, info)

	assert.NoError(t, err)
	assert.NotNil(t, graph)
	assert.Equal(t, 3, graph.Size())

	// Verify nodes exist
	vpcNode, exists := graph.GetNode("vpc-dev")
	assert.True(t, exists)
	assert.Equal(t, "vpc", vpcNode.Component)
	assert.Equal(t, "dev", vpcNode.Stack)

	dbNode, exists := graph.GetNode("database-dev")
	assert.True(t, exists)
	assert.Equal(t, "database", dbNode.Component)
	assert.Equal(t, 1, len(dbNode.Dependencies))

	appNode, exists := graph.GetNode("app-dev")
	assert.True(t, exists)
	assert.Equal(t, "app", appNode.Component)
	assert.Equal(t, 1, len(appNode.Dependencies))

	// Verify execution order
	order, err := graph.TopologicalSort()
	assert.NoError(t, err)
	assert.Equal(t, 3, len(order))
	assert.Equal(t, "vpc", order[0].Component)
	assert.Equal(t, "database", order[1].Component)
	assert.Equal(t, "app", order[2].Component)
}

func TestBuildTerraformDependencyGraph_WithAbstractComponents(t *testing.T) {
	// Test that abstract components are filtered out
	stacks := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"base": map[string]any{
						"metadata": map[string]any{
							"type": "abstract",
						},
						"vars": map[string]any{
							"common": "value",
						},
					},
					"real": map[string]any{
						"metadata": map[string]any{
							"component": "mock",
						},
						"vars": map[string]any{
							"foo": "bar",
						},
					},
				},
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}

	graph, err := buildTerraformDependencyGraph(atmosConfig, stacks, info)

	assert.NoError(t, err)
	assert.NotNil(t, graph)
	assert.Equal(t, 1, graph.Size()) // Only "real" component

	_, exists := graph.GetNode("base-dev")
	assert.False(t, exists) // Abstract component not in graph

	realNode, exists := graph.GetNode("real-dev")
	assert.True(t, exists)
	assert.Equal(t, "real", realNode.Component)
}

func TestBuildTerraformDependencyGraph_WithDisabledComponents(t *testing.T) {
	// Test that disabled components are filtered out
	stacks := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"disabled": map[string]any{
						"metadata": map[string]any{
							"enabled": false,
						},
						"vars": map[string]any{
							"foo": "bar",
						},
					},
					"enabled": map[string]any{
						"metadata": map[string]any{
							"enabled": true,
						},
						"vars": map[string]any{
							"foo": "bar",
						},
					},
				},
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}

	graph, err := buildTerraformDependencyGraph(atmosConfig, stacks, info)

	assert.NoError(t, err)
	assert.NotNil(t, graph)
	assert.Equal(t, 1, graph.Size()) // Only "enabled" component

	_, exists := graph.GetNode("disabled-dev")
	assert.False(t, exists) // Disabled component not in graph

	enabledNode, exists := graph.GetNode("enabled-dev")
	assert.True(t, exists)
	assert.Equal(t, "enabled", enabledNode.Component)
}

func TestApplyFiltersToGraph(t *testing.T) {
	// Create a test graph
	graph := dependency.NewGraph()

	node1 := &dependency.Node{
		ID:        "vpc-dev",
		Component: "vpc",
		Stack:     "dev",
		Type:      config.TerraformComponentType,
	}
	node2 := &dependency.Node{
		ID:        "database-dev",
		Component: "database",
		Stack:     "dev",
		Type:      config.TerraformComponentType,
	}
	node3 := &dependency.Node{
		ID:        "app-prod",
		Component: "app",
		Stack:     "prod",
		Type:      config.TerraformComponentType,
	}

	_ = graph.AddNode(node1)
	_ = graph.AddNode(node2)
	_ = graph.AddNode(node3)
	_ = graph.AddDependency("database-dev", "vpc-dev")

	t.Run("filter by stack", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			Stack: "dev",
		}

		filtered := applyFiltersToGraph(graph, nil, info)
		assert.Equal(t, 2, filtered.Size()) // Only dev stack components

		_, exists := filtered.GetNode("vpc-dev")
		assert.True(t, exists)
		_, exists = filtered.GetNode("database-dev")
		assert.True(t, exists)
		_, exists = filtered.GetNode("app-prod")
		assert.False(t, exists) // prod stack component filtered out
	})

	t.Run("filter by components", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			Components: []string{"vpc", "database"},
		}

		filtered := applyFiltersToGraph(graph, nil, info)
		assert.Equal(t, 2, filtered.Size())

		_, exists := filtered.GetNode("vpc-dev")
		assert.True(t, exists)
		_, exists = filtered.GetNode("database-dev")
		assert.True(t, exists)
		_, exists = filtered.GetNode("app-prod")
		assert.False(t, exists) // app component filtered out
	})

	t.Run("no filters", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{}

		filtered := applyFiltersToGraph(graph, nil, info)
		assert.Equal(t, 3, filtered.Size()) // All components included
	})
}
