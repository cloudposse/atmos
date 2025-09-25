package dependency

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewGraph(t *testing.T) {
	graph := NewGraph()

	assert.NotNil(t, graph)
	assert.NotNil(t, graph.Nodes)
	assert.NotNil(t, graph.Roots)
	assert.Equal(t, 0, len(graph.Nodes))
	assert.Equal(t, 0, len(graph.Roots))
}

func TestGraph_AddNode(t *testing.T) {
	graph := NewGraph()

	// Test adding a valid node
	node := &Node{
		ID:        "test-node",
		Component: "test",
		Stack:     "dev",
		Type:      "terraform",
	}

	err := graph.AddNode(node)
	assert.NoError(t, err)
	assert.Equal(t, 1, graph.Size())
	assert.NotNil(t, graph.Nodes["test-node"])

	// Test adding nil node
	err = graph.AddNode(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot add nil node")

	// Test adding node with empty ID
	emptyNode := &Node{Component: "test"}
	err = graph.AddNode(emptyNode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node ID cannot be empty")

	// Test adding duplicate node
	err = graph.AddNode(node)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestGraph_AddDependency(t *testing.T) {
	graph := NewGraph()

	// Add two nodes
	node1 := &Node{ID: "node1", Component: "comp1", Stack: "dev"}
	node2 := &Node{ID: "node2", Component: "comp2", Stack: "dev"}

	err := graph.AddNode(node1)
	assert.NoError(t, err)
	err = graph.AddNode(node2)
	assert.NoError(t, err)

	// Test adding valid dependency
	err = graph.AddDependency("node1", "node2")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(node1.Dependencies))
	assert.Equal(t, "node2", node1.Dependencies[0])
	assert.Equal(t, 1, len(node2.Dependents))
	assert.Equal(t, "node1", node2.Dependents[0])

	// Test adding duplicate dependency (should be idempotent)
	err = graph.AddDependency("node1", "node2")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(node1.Dependencies))

	// Test adding self-dependency
	err = graph.AddDependency("node1", "node1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot depend on itself")

	// Test adding dependency with non-existent nodes
	err = graph.AddDependency("node1", "non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")

	err = graph.AddDependency("non-existent", "node1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")

	// Test empty IDs
	err = graph.AddDependency("", "node1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "IDs cannot be empty")
}

func TestGraph_IdentifyRoots(t *testing.T) {
	graph := NewGraph()

	// Add nodes with dependencies
	node1 := &Node{ID: "node1", Component: "comp1", Stack: "dev"}
	node2 := &Node{ID: "node2", Component: "comp2", Stack: "dev"}
	node3 := &Node{ID: "node3", Component: "comp3", Stack: "dev"}

	_ = graph.AddNode(node1)
	_ = graph.AddNode(node2)
	_ = graph.AddNode(node3)

	// node2 depends on node1, node3 depends on node2
	_ = graph.AddDependency("node2", "node1")
	_ = graph.AddDependency("node3", "node2")

	graph.IdentifyRoots()

	assert.Equal(t, 1, len(graph.Roots))
	assert.Equal(t, "node1", graph.Roots[0])
}

func TestGraph_HasCycles(t *testing.T) {
	// Test graph with no cycles
	graph := NewGraph()
	node1 := &Node{ID: "node1"}
	node2 := &Node{ID: "node2"}
	node3 := &Node{ID: "node3"}

	_ = graph.AddNode(node1)
	_ = graph.AddNode(node2)
	_ = graph.AddNode(node3)

	_ = graph.AddDependency("node2", "node1")
	_ = graph.AddDependency("node3", "node2")

	hasCycle, cyclePath := graph.HasCycles()
	assert.False(t, hasCycle)
	assert.Nil(t, cyclePath)

	// Test graph with direct cycle
	_ = graph.AddDependency("node1", "node3")

	hasCycle, cyclePath = graph.HasCycles()
	assert.True(t, hasCycle)
	assert.NotNil(t, cyclePath)
}

func TestGraph_Clone(t *testing.T) {
	graph := NewGraph()

	// Add nodes and dependencies
	node1 := &Node{ID: "node1", Component: "comp1", Stack: "dev"}
	node2 := &Node{ID: "node2", Component: "comp2", Stack: "dev"}

	_ = graph.AddNode(node1)
	_ = graph.AddNode(node2)
	_ = graph.AddDependency("node2", "node1")
	graph.IdentifyRoots()

	// Clone the graph
	cloned := graph.Clone()

	// Verify the clone
	assert.Equal(t, graph.Size(), cloned.Size())
	assert.Equal(t, len(graph.Roots), len(cloned.Roots))

	// Verify nodes are cloned
	clonedNode1 := cloned.Nodes["node1"]
	assert.NotNil(t, clonedNode1)
	assert.Equal(t, node1.Component, clonedNode1.Component)

	// Modify the clone and verify original is unchanged
	clonedNode1.Component = "modified"
	assert.NotEqual(t, node1.Component, clonedNode1.Component)

	// Verify dependencies are cloned
	clonedNode2 := cloned.Nodes["node2"]
	assert.Equal(t, 1, len(clonedNode2.Dependencies))
	assert.Equal(t, "node1", clonedNode2.Dependencies[0])
}
