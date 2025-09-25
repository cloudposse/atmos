package dependency

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/config"
)

func TestGraph_Filter(t *testing.T) {
	// Create a test graph
	graph := NewGraph()

	// Add nodes
	nodes := []*Node{
		{ID: "vpc-dev", Component: "vpc", Stack: "dev", Type: config.TerraformComponentType},
		{ID: "database-dev", Component: "database", Stack: "dev", Type: config.TerraformComponentType},
		{ID: "app-dev", Component: "app", Stack: "dev", Type: config.TerraformComponentType},
		{ID: "vpc-prod", Component: "vpc", Stack: "prod", Type: config.TerraformComponentType},
		{ID: "database-prod", Component: "database", Stack: "prod", Type: config.TerraformComponentType},
	}

	for _, node := range nodes {
		_ = graph.AddNode(node)
	}

	// Add dependencies
	_ = graph.AddDependency("database-dev", "vpc-dev")
	_ = graph.AddDependency("app-dev", "database-dev")
	_ = graph.AddDependency("database-prod", "vpc-prod")

	tests := []struct {
		name          string
		filter        Filter
		expectCount   int
		expectNodeIDs []string
	}{
		{
			name: "filter single node without dependencies",
			filter: Filter{
				NodeIDs:             []string{"vpc-dev"},
				IncludeDependencies: false,
				IncludeDependents:   false,
			},
			expectCount:   1,
			expectNodeIDs: []string{"vpc-dev"},
		},
		{
			name: "filter single node with dependencies",
			filter: Filter{
				NodeIDs:             []string{"app-dev"},
				IncludeDependencies: true,
				IncludeDependents:   false,
			},
			expectCount:   3,
			expectNodeIDs: []string{"app-dev", "database-dev", "vpc-dev"},
		},
		{
			name: "filter single node with dependents",
			filter: Filter{
				NodeIDs:             []string{"vpc-dev"},
				IncludeDependencies: false,
				IncludeDependents:   true,
			},
			expectCount:   3,
			expectNodeIDs: []string{"vpc-dev", "database-dev", "app-dev"},
		},
		{
			name: "filter multiple nodes",
			filter: Filter{
				NodeIDs:             []string{"vpc-dev", "vpc-prod"},
				IncludeDependencies: false,
				IncludeDependents:   false,
			},
			expectCount:   2,
			expectNodeIDs: []string{"vpc-dev", "vpc-prod"},
		},
		{
			name: "filter with both dependencies and dependents",
			filter: Filter{
				NodeIDs:             []string{"database-dev"},
				IncludeDependencies: true,
				IncludeDependents:   true,
			},
			expectCount:   3,
			expectNodeIDs: []string{"vpc-dev", "database-dev", "app-dev"},
		},
		{
			name: "filter non-existent node",
			filter: Filter{
				NodeIDs:             []string{"non-existent"},
				IncludeDependencies: true,
				IncludeDependents:   true,
			},
			expectCount:   0,
			expectNodeIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := graph.Filter(tt.filter)

			assert.Equal(t, tt.expectCount, filtered.Size())

			for _, expectedID := range tt.expectNodeIDs {
				_, exists := filtered.GetNode(expectedID)
				assert.True(t, exists, "Expected node %s to exist in filtered graph", expectedID)
			}

			// Verify relationships are preserved in filtered graph
			if tt.filter.IncludeDependencies || tt.filter.IncludeDependents {
				for _, node := range filtered.Nodes {
					// Check dependencies
					for _, depID := range node.Dependencies {
						_, exists := filtered.GetNode(depID)
						assert.True(t, exists, "Dependency %s should exist in filtered graph", depID)
					}
					// Check dependents
					for _, depID := range node.Dependents {
						_, exists := filtered.GetNode(depID)
						assert.True(t, exists, "Dependent %s should exist in filtered graph", depID)
					}
				}
			}
		})
	}
}

func TestGraph_FilterByType(t *testing.T) {
	graph := NewGraph()

	// Add nodes with different types
	nodes := []*Node{
		{ID: "vpc-dev", Component: "vpc", Stack: "dev", Type: config.TerraformComponentType},
		{ID: "app-helm", Component: "app", Stack: "dev", Type: config.HelmfileComponentType},
		{ID: "database-dev", Component: "database", Stack: "dev", Type: config.TerraformComponentType},
		{ID: "monitoring-helm", Component: "monitoring", Stack: "dev", Type: config.HelmfileComponentType},
	}

	for _, node := range nodes {
		_ = graph.AddNode(node)
	}

	// Add dependencies
	_ = graph.AddDependency("database-dev", "vpc-dev")
	_ = graph.AddDependency("app-helm", "database-dev")

	t.Run("filter terraform components", func(t *testing.T) {
		filtered := graph.FilterByType(config.TerraformComponentType)

		assert.Equal(t, 2, filtered.Size())

		_, exists := filtered.GetNode("vpc-dev")
		assert.True(t, exists)
		_, exists = filtered.GetNode("database-dev")
		assert.True(t, exists)
		_, exists = filtered.GetNode("app-helm")
		assert.False(t, exists)
	})

	t.Run("filter helmfile components", func(t *testing.T) {
		filtered := graph.FilterByType(config.HelmfileComponentType)

		// Should include helmfile components and their terraform dependencies
		assert.Greater(t, filtered.Size(), 0)

		_, exists := filtered.GetNode("app-helm")
		assert.True(t, exists)
		_, exists = filtered.GetNode("monitoring-helm")
		assert.True(t, exists)
	})
}

func TestGraph_FilterByStack(t *testing.T) {
	graph := NewGraph()

	// Add nodes from different stacks
	nodes := []*Node{
		{ID: "vpc-dev", Component: "vpc", Stack: "dev", Type: config.TerraformComponentType},
		{ID: "database-dev", Component: "database", Stack: "dev", Type: config.TerraformComponentType},
		{ID: "vpc-prod", Component: "vpc", Stack: "prod", Type: config.TerraformComponentType},
		{ID: "database-prod", Component: "database", Stack: "prod", Type: config.TerraformComponentType},
		{ID: "vpc-staging", Component: "vpc", Stack: "staging", Type: config.TerraformComponentType},
	}

	for _, node := range nodes {
		_ = graph.AddNode(node)
	}

	// Add dependencies within and across stacks
	_ = graph.AddDependency("database-dev", "vpc-dev")
	_ = graph.AddDependency("database-prod", "vpc-prod")
	_ = graph.AddDependency("vpc-staging", "vpc-dev") // Cross-stack dependency

	t.Run("filter dev stack", func(t *testing.T) {
		filtered := graph.FilterByStack("dev")

		// Should include dev components and staging vpc (which depends on dev)
		assert.Greater(t, filtered.Size(), 0)

		_, exists := filtered.GetNode("vpc-dev")
		assert.True(t, exists)
		_, exists = filtered.GetNode("database-dev")
		assert.True(t, exists)
		_, exists = filtered.GetNode("vpc-prod")
		assert.False(t, exists)
	})

	t.Run("filter prod stack", func(t *testing.T) {
		filtered := graph.FilterByStack("prod")

		assert.Equal(t, 2, filtered.Size())

		_, exists := filtered.GetNode("vpc-prod")
		assert.True(t, exists)
		_, exists = filtered.GetNode("database-prod")
		assert.True(t, exists)
		_, exists = filtered.GetNode("vpc-dev")
		assert.False(t, exists)
	})
}

func TestGraph_FilterByComponent(t *testing.T) {
	graph := NewGraph()

	// Add nodes
	nodes := []*Node{
		{ID: "vpc-dev", Component: "vpc", Stack: "dev", Type: config.TerraformComponentType},
		{ID: "vpc-prod", Component: "vpc", Stack: "prod", Type: config.TerraformComponentType},
		{ID: "database-dev", Component: "database", Stack: "dev", Type: config.TerraformComponentType},
		{ID: "database-prod", Component: "database", Stack: "prod", Type: config.TerraformComponentType},
		{ID: "app-dev", Component: "app", Stack: "dev", Type: config.TerraformComponentType},
	}

	for _, node := range nodes {
		_ = graph.AddNode(node)
	}

	// Add dependencies
	_ = graph.AddDependency("database-dev", "vpc-dev")
	_ = graph.AddDependency("database-prod", "vpc-prod")
	_ = graph.AddDependency("app-dev", "database-dev")

	t.Run("filter vpc component", func(t *testing.T) {
		filtered := graph.FilterByComponent("vpc")

		// Should include all vpc instances and their dependents
		assert.Greater(t, filtered.Size(), 2)

		_, exists := filtered.GetNode("vpc-dev")
		assert.True(t, exists)
		_, exists = filtered.GetNode("vpc-prod")
		assert.True(t, exists)

		// Should include dependents as well
		_, exists = filtered.GetNode("database-dev")
		assert.True(t, exists)
		_, exists = filtered.GetNode("database-prod")
		assert.True(t, exists)
	})

	t.Run("filter database component", func(t *testing.T) {
		filtered := graph.FilterByComponent("database")

		// Should include database instances, their dependencies and dependents
		assert.Greater(t, filtered.Size(), 2)

		_, exists := filtered.GetNode("database-dev")
		assert.True(t, exists)
		_, exists = filtered.GetNode("database-prod")
		assert.True(t, exists)
	})
}

func TestGraph_GetConnectedComponents(t *testing.T) {
	graph := NewGraph()

	// Create two separate subgraphs
	// Subgraph 1: vpc-dev <- database-dev <- app-dev
	nodes1 := []*Node{
		{ID: "vpc-dev", Component: "vpc", Stack: "dev"},
		{ID: "database-dev", Component: "database", Stack: "dev"},
		{ID: "app-dev", Component: "app", Stack: "dev"},
	}

	// Subgraph 2: vpc-prod <- database-prod
	nodes2 := []*Node{
		{ID: "vpc-prod", Component: "vpc", Stack: "prod"},
		{ID: "database-prod", Component: "database", Stack: "prod"},
	}

	// Isolated node
	isolatedNode := &Node{ID: "isolated", Component: "isolated", Stack: "test"}

	for _, node := range nodes1 {
		_ = graph.AddNode(node)
	}
	for _, node := range nodes2 {
		_ = graph.AddNode(node)
	}
	_ = graph.AddNode(isolatedNode)

	// Add dependencies for subgraph 1
	_ = graph.AddDependency("database-dev", "vpc-dev")
	_ = graph.AddDependency("app-dev", "database-dev")

	// Add dependencies for subgraph 2
	_ = graph.AddDependency("database-prod", "vpc-prod")

	components := graph.GetConnectedComponents()

	assert.Equal(t, 3, len(components), "Should have 3 connected components")

	// Verify each component
	componentSizes := []int{}
	for _, comp := range components {
		componentSizes = append(componentSizes, comp.Size())
	}

	// Should have components of sizes 3, 2, and 1
	assert.Contains(t, componentSizes, 3)
	assert.Contains(t, componentSizes, 2)
	assert.Contains(t, componentSizes, 1)
}

func TestGraph_RemoveNode(t *testing.T) {
	graph := NewGraph()

	// Create a graph with dependencies
	nodes := []*Node{
		{ID: "vpc-dev", Component: "vpc", Stack: "dev"},
		{ID: "database-dev", Component: "database", Stack: "dev"},
		{ID: "app-dev", Component: "app", Stack: "dev"},
		{ID: "cache-dev", Component: "cache", Stack: "dev"},
	}

	for _, node := range nodes {
		_ = graph.AddNode(node)
	}

	// Create dependencies: vpc <- database <- app, vpc <- cache
	_ = graph.AddDependency("database-dev", "vpc-dev")
	_ = graph.AddDependency("app-dev", "database-dev")
	_ = graph.AddDependency("cache-dev", "vpc-dev")

	t.Run("remove middle node", func(t *testing.T) {
		// Clone the graph for this test
		testGraph := graph.Clone()

		err := testGraph.RemoveNode("database-dev")
		assert.NoError(t, err)

		// Verify node is removed
		_, exists := testGraph.GetNode("database-dev")
		assert.False(t, exists)

		// Verify relationships are updated
		appNode, _ := testGraph.GetNode("app-dev")
		assert.NotContains(t, appNode.Dependencies, "database-dev")

		vpcNode, _ := testGraph.GetNode("vpc-dev")
		assert.NotContains(t, vpcNode.Dependents, "database-dev")

		// Graph should now have 3 nodes
		assert.Equal(t, 3, testGraph.Size())
	})

	t.Run("remove root node", func(t *testing.T) {
		testGraph := graph.Clone()

		err := testGraph.RemoveNode("vpc-dev")
		assert.NoError(t, err)

		// Verify node is removed
		_, exists := testGraph.GetNode("vpc-dev")
		assert.False(t, exists)

		// Verify dependencies are updated
		dbNode, _ := testGraph.GetNode("database-dev")
		assert.NotContains(t, dbNode.Dependencies, "vpc-dev")

		cacheNode, _ := testGraph.GetNode("cache-dev")
		assert.NotContains(t, cacheNode.Dependencies, "vpc-dev")
	})

	t.Run("remove leaf node", func(t *testing.T) {
		testGraph := graph.Clone()

		err := testGraph.RemoveNode("app-dev")
		assert.NoError(t, err)

		// Verify node is removed
		_, exists := testGraph.GetNode("app-dev")
		assert.False(t, exists)

		// Verify parent's dependents are updated
		dbNode, _ := testGraph.GetNode("database-dev")
		assert.NotContains(t, dbNode.Dependents, "app-dev")
	})

	t.Run("remove non-existent node", func(t *testing.T) {
		testGraph := graph.Clone()
		originalSize := testGraph.Size()

		err := testGraph.RemoveNode("non-existent")
		assert.NoError(t, err) // Should not error for non-existent node

		// Graph should remain unchanged
		assert.Equal(t, originalSize, testGraph.Size())
	})
}

func TestFilterHelperFunctions(t *testing.T) {
	t.Run("filterNodeIDs", func(t *testing.T) {
		ids := []string{"vpc-dev", "database-dev", "app-dev", "cache-dev"}
		toInclude := map[string]bool{
			"vpc-dev":      true,
			"app-dev":      true,
			"non-existent": true,
		}

		filtered := filterNodeIDs(ids, toInclude)

		assert.Equal(t, 2, len(filtered))
		assert.Contains(t, filtered, "vpc-dev")
		assert.Contains(t, filtered, "app-dev")
		assert.NotContains(t, filtered, "database-dev")
		assert.NotContains(t, filtered, "cache-dev")
	})

	t.Run("removeStringFromSlice", func(t *testing.T) {
		slice := []string{"a", "b", "c", "b", "d"}

		result := removeStringFromSlice(slice, "b")

		assert.Equal(t, 3, len(result))
		assert.Contains(t, result, "a")
		assert.Contains(t, result, "c")
		assert.Contains(t, result, "d")
		assert.NotContains(t, result, "b")
	})

	t.Run("removeStringFromSlice with non-existent item", func(t *testing.T) {
		slice := []string{"a", "b", "c"}

		result := removeStringFromSlice(slice, "d")

		assert.Equal(t, 3, len(result))
		assert.Equal(t, slice, result)
	})
}
