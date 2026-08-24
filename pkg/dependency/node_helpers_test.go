package dependency

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCloneNodeCore(t *testing.T) {
	tests := []struct {
		name string
		node *Node
	}{
		{
			name: "clone node with all fields",
			node: &Node{
				ID:           "test-node",
				Component:    "test-component",
				Stack:        "test-stack",
				Type:         "terraform",
				Dependencies: []string{"dep1", "dep2"},
				Dependents:   []string{"dependent1"},
				Metadata: map[string]any{
					"key1": "value1",
					"key2": 42,
				},
				Processed: true,
			},
		},
		{
			name: "clone node with empty slices",
			node: &Node{
				ID:           "empty-node",
				Component:    "component",
				Stack:        "stack",
				Type:         "helmfile",
				Dependencies: []string{},
				Dependents:   []string{},
				Metadata:     nil,
				Processed:    false,
			},
		},
		{
			name: "clone nil node",
			node: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloned := cloneNodeCore(tt.node)

			if tt.node == nil {
				assert.Nil(t, cloned)
				return
			}

			// Verify the clone is a different instance.
			assert.NotSame(t, tt.node, cloned)

			// Verify all fields are copied.
			assert.Equal(t, tt.node.ID, cloned.ID)
			assert.Equal(t, tt.node.Component, cloned.Component)
			assert.Equal(t, tt.node.Stack, cloned.Stack)
			assert.Equal(t, tt.node.Type, cloned.Type)
			assert.Equal(t, tt.node.Processed, cloned.Processed)

			// Verify slices are deep copied.
			assert.Equal(t, tt.node.Dependencies, cloned.Dependencies)
			assert.Equal(t, tt.node.Dependents, cloned.Dependents)
			// Slices should be different instances even with same content.
			if len(tt.node.Dependencies) > 0 {
				// Modify clone to verify independence.
				originalDep := tt.node.Dependencies[0]
				cloned.Dependencies[0] = "modified"
				assert.Equal(t, originalDep, tt.node.Dependencies[0])
				// Restore for comparison.
				cloned.Dependencies[0] = originalDep
			}

			// Verify metadata is deep copied.
			assert.Equal(t, tt.node.Metadata, cloned.Metadata)

			// Verify modifying the clone doesn't affect the original.
			if cloned.Metadata != nil {
				cloned.Metadata["newKey"] = "newValue"
				_, exists := tt.node.Metadata["newKey"]
				assert.False(t, exists)
			}
		})
	}
}

func TestCloneNodeMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
	}{
		{
			name: "clone non-empty metadata",
			metadata: map[string]any{
				"string":  "value",
				"number":  123,
				"float":   45.67,
				"boolean": true,
				"nested": map[string]any{
					"key": "value",
				},
			},
		},
		{
			name:     "clone empty metadata",
			metadata: map[string]any{},
		},
		{
			name:     "clone nil metadata",
			metadata: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloned := cloneNodeMetadata(tt.metadata)

			if tt.metadata == nil {
				assert.Nil(t, cloned)
				return
			}

			// Verify it's a different instance by checking modifications don't affect original.

			// Verify all entries are copied.
			assert.Equal(t, tt.metadata, cloned)

			// Verify modifications don't affect original.
			cloned["testKey"] = "testValue"
			_, exists := tt.metadata["testKey"]
			assert.False(t, exists)
		})
	}
}

func TestCloneNodeWithFilteredEdges(t *testing.T) {
	node := &Node{
		ID:           "node1",
		Component:    "comp1",
		Stack:        "stack1",
		Type:         "terraform",
		Dependencies: []string{"node2", "node3", "node4"},
		Dependents:   []string{"node5", "node6", "node7"},
		Metadata: map[string]any{
			"key": "value",
		},
		Processed: true,
	}

	tests := []struct {
		name            string
		allowedNodes    map[string]bool
		expectedDeps    []string
		expectedDepents []string
	}{
		{
			name: "filter to subset of nodes",
			allowedNodes: map[string]bool{
				"node2": true,
				"node4": true,
				"node5": true,
			},
			expectedDeps:    []string{"node2", "node4"},
			expectedDepents: []string{"node5"},
		},
		{
			name:            "filter with no allowed nodes",
			allowedNodes:    map[string]bool{},
			expectedDeps:    []string{},
			expectedDepents: []string{},
		},
		{
			name: "filter with all nodes allowed",
			allowedNodes: map[string]bool{
				"node2": true,
				"node3": true,
				"node4": true,
				"node5": true,
				"node6": true,
				"node7": true,
			},
			expectedDeps:    []string{"node2", "node3", "node4"},
			expectedDepents: []string{"node5", "node6", "node7"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloned := cloneNodeWithFilteredEdges(node, tt.allowedNodes)

			// Verify basic fields are copied.
			assert.Equal(t, node.ID, cloned.ID)
			assert.Equal(t, node.Component, cloned.Component)
			assert.Equal(t, node.Stack, cloned.Stack)
			assert.Equal(t, node.Type, cloned.Type)
			assert.Equal(t, node.Processed, cloned.Processed)

			// Verify edges are filtered correctly.
			assert.Equal(t, tt.expectedDeps, cloned.Dependencies)
			assert.Equal(t, tt.expectedDepents, cloned.Dependents)

			// Verify metadata is cloned.
			assert.Equal(t, node.Metadata, cloned.Metadata)
			// Test independence by modifying clone.
			if cloned.Metadata != nil {
				cloned.Metadata["testKey"] = "testValue"
				_, exists := node.Metadata["testKey"]
				assert.False(t, exists)
				delete(cloned.Metadata, "testKey")
			}
		})
	}

	// Test nil node.
	t.Run("clone nil node with filter", func(t *testing.T) {
		cloned := cloneNodeWithFilteredEdges(nil, map[string]bool{"node1": true})
		assert.Nil(t, cloned)
	})
}

func TestFilterStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		allowed  map[string]bool
		expected []string
	}{
		{
			name:     "filter subset",
			slice:    []string{"a", "b", "c", "d"},
			allowed:  map[string]bool{"a": true, "c": true},
			expected: []string{"a", "c"},
		},
		{
			name:     "filter none",
			slice:    []string{"a", "b", "c"},
			allowed:  map[string]bool{},
			expected: []string{},
		},
		{
			name:     "filter all",
			slice:    []string{"a", "b", "c"},
			allowed:  map[string]bool{"a": true, "b": true, "c": true},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "filter empty slice",
			slice:    []string{},
			allowed:  map[string]bool{"a": true},
			expected: []string{},
		},
		{
			name:     "filter nil slice",
			slice:    nil,
			allowed:  map[string]bool{"a": true},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterStringSlice(tt.slice, tt.allowed)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGraphTraversalHelpers(t *testing.T) {
	// Create a test graph with multiple connected components.
	graph := NewGraph()

	// Component 1: nodes 1, 2, 3.
	_ = graph.AddNode(&Node{ID: "node1"})
	_ = graph.AddNode(&Node{ID: "node2"})
	_ = graph.AddNode(&Node{ID: "node3"})
	_ = graph.AddDependency("node2", "node1")
	_ = graph.AddDependency("node3", "node2")

	// Component 2: nodes 4, 5.
	_ = graph.AddNode(&Node{ID: "node4"})
	_ = graph.AddNode(&Node{ID: "node5"})
	_ = graph.AddDependency("node5", "node4")

	// Isolated node.
	_ = graph.AddNode(&Node{ID: "node6"})

	t.Run("findConnectedComponent", func(t *testing.T) {
		visited := make(map[string]bool)

		// Find first component starting from node1.
		component1 := graph.findConnectedComponent("node1", visited)
		assert.Len(t, component1, 3)
		assert.True(t, component1["node1"])
		assert.True(t, component1["node2"])
		assert.True(t, component1["node3"])

		// Verify visited map is updated.
		assert.True(t, visited["node1"])
		assert.True(t, visited["node2"])
		assert.True(t, visited["node3"])
	})

	t.Run("traverseConnectedNodes", func(t *testing.T) {
		visited := make(map[string]bool)
		componentNodeIDs := make(map[string]bool)

		graph.traverseConnectedNodes("node4", visited, componentNodeIDs)

		// Should find node4 and node5.
		assert.Len(t, componentNodeIDs, 2)
		assert.True(t, componentNodeIDs["node4"])
		assert.True(t, componentNodeIDs["node5"])

		// Should not find nodes from other components.
		assert.False(t, componentNodeIDs["node1"])
		assert.False(t, componentNodeIDs["node6"])
	})

	t.Run("buildComponentGraph", func(t *testing.T) {
		componentNodeIDs := map[string]bool{
			"node1": true,
			"node2": true,
			"node3": true,
		}

		component := graph.buildComponentGraph(componentNodeIDs)

		// Verify the component has the right nodes.
		assert.Len(t, component.Nodes, 3)
		assert.NotNil(t, component.Nodes["node1"])
		assert.NotNil(t, component.Nodes["node2"])
		assert.NotNil(t, component.Nodes["node3"])

		// Verify dependencies are preserved within the component.
		node2 := component.Nodes["node2"]
		assert.Contains(t, node2.Dependencies, "node1")
		assert.Contains(t, node2.Dependents, "node3")

		// Verify roots are identified.
		assert.Contains(t, component.Roots, "node1")
	})
}
