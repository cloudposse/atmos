package dependency

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGraph_TopologicalSort(t *testing.T) {
	t.Run("simple linear dependency", func(t *testing.T) {
		graph := NewGraph()

		// Create a simple chain: node1 -> node2 -> node3
		node1 := &Node{ID: "node1", Component: "comp1", Stack: "dev"}
		node2 := &Node{ID: "node2", Component: "comp2", Stack: "dev"}
		node3 := &Node{ID: "node3", Component: "comp3", Stack: "dev"}

		assert.NoError(t, graph.AddNode(node1))
		assert.NoError(t, graph.AddNode(node2))
		assert.NoError(t, graph.AddNode(node3))

		assert.NoError(t, graph.AddDependency("node2", "node1"))
		assert.NoError(t, graph.AddDependency("node3", "node2"))

		order, err := graph.TopologicalSort()

		assert.NoError(t, err)
		assert.Equal(t, 3, len(order))
		assert.Equal(t, "node1", order[0].ID)
		assert.Equal(t, "node2", order[1].ID)
		assert.Equal(t, "node3", order[2].ID)
	})

	t.Run("diamond dependency", func(t *testing.T) {
		graph := NewGraph()

		// Create a diamond: A -> B,C -> D
		nodeA := &Node{ID: "A"}
		nodeB := &Node{ID: "B"}
		nodeC := &Node{ID: "C"}
		nodeD := &Node{ID: "D"}

		assert.NoError(t, graph.AddNode(nodeA))
		assert.NoError(t, graph.AddNode(nodeB))
		assert.NoError(t, graph.AddNode(nodeC))
		assert.NoError(t, graph.AddNode(nodeD))

		assert.NoError(t, graph.AddDependency("B", "A"))
		assert.NoError(t, graph.AddDependency("C", "A"))
		assert.NoError(t, graph.AddDependency("D", "B"))
		assert.NoError(t, graph.AddDependency("D", "C"))

		order, err := graph.TopologicalSort()

		assert.NoError(t, err)
		assert.Equal(t, 4, len(order))
		assert.Equal(t, "A", order[0].ID)
		// B and C can be in any order, but both must come before D
		assert.Contains(t, []string{"B", "C"}, order[1].ID)
		assert.Contains(t, []string{"B", "C"}, order[2].ID)
		assert.Equal(t, "D", order[3].ID)
	})

	t.Run("multiple roots", func(t *testing.T) {
		graph := NewGraph()

		// Multiple independent chains
		node1 := &Node{ID: "node1"}
		node2 := &Node{ID: "node2"}
		node3 := &Node{ID: "node3"}
		node4 := &Node{ID: "node4"}

		assert.NoError(t, graph.AddNode(node1))
		assert.NoError(t, graph.AddNode(node2))
		assert.NoError(t, graph.AddNode(node3))
		assert.NoError(t, graph.AddNode(node4))

		assert.NoError(t, graph.AddDependency("node2", "node1"))
		assert.NoError(t, graph.AddDependency("node4", "node3"))

		order, err := graph.TopologicalSort()

		assert.NoError(t, err)
		assert.Equal(t, 4, len(order))

		// Create a map to check positions
		positions := make(map[string]int)
		for i, node := range order {
			positions[node.ID] = i
		}

		// node1 must come before node2
		assert.Less(t, positions["node1"], positions["node2"])
		// node3 must come before node4
		assert.Less(t, positions["node3"], positions["node4"])
	})

	t.Run("circular dependency", func(t *testing.T) {
		graph := NewGraph()

		node1 := &Node{ID: "node1"}
		node2 := &Node{ID: "node2"}
		node3 := &Node{ID: "node3"}

		assert.NoError(t, graph.AddNode(node1))
		assert.NoError(t, graph.AddNode(node2))
		assert.NoError(t, graph.AddNode(node3))

		assert.NoError(t, graph.AddDependency("node1", "node2"))
		assert.NoError(t, graph.AddDependency("node2", "node3"))
		assert.NoError(t, graph.AddDependency("node3", "node1")) // Creates cycle

		order, err := graph.TopologicalSort()

		assert.Error(t, err)
		assert.Nil(t, order)
		assert.Contains(t, err.Error(), "circular dependency")
	})

	t.Run("empty graph", func(t *testing.T) {
		graph := NewGraph()

		order, err := graph.TopologicalSort()

		assert.NoError(t, err)
		assert.Equal(t, 0, len(order))
	})
}

func TestGraph_ReverseTopologicalSort(t *testing.T) {
	graph := NewGraph()

	// Create a simple chain: node1 -> node2 -> node3
	node1 := &Node{ID: "node1"}
	node2 := &Node{ID: "node2"}
	node3 := &Node{ID: "node3"}

	assert.NoError(t, graph.AddNode(node1))
	assert.NoError(t, graph.AddNode(node2))
	assert.NoError(t, graph.AddNode(node3))

	assert.NoError(t, graph.AddDependency("node2", "node1"))
	assert.NoError(t, graph.AddDependency("node3", "node2"))

	order, err := graph.ReverseTopologicalSort()

	assert.NoError(t, err)
	assert.Equal(t, 3, len(order))
	assert.Equal(t, "node3", order[0].ID)
	assert.Equal(t, "node2", order[1].ID)
	assert.Equal(t, "node1", order[2].ID)
}

func TestGraph_GetExecutionLevels(t *testing.T) {
	graph := NewGraph()

	// Create a complex graph with multiple levels
	node1 := &Node{ID: "node1"}
	node2 := &Node{ID: "node2"}
	node3 := &Node{ID: "node3"}
	node4 := &Node{ID: "node4"}
	node5 := &Node{ID: "node5"}

	assert.NoError(t, graph.AddNode(node1))
	assert.NoError(t, graph.AddNode(node2))
	assert.NoError(t, graph.AddNode(node3))
	assert.NoError(t, graph.AddNode(node4))
	assert.NoError(t, graph.AddNode(node5))

	assert.NoError(t, graph.AddDependency("node2", "node1"))
	assert.NoError(t, graph.AddDependency("node3", "node1"))
	assert.NoError(t, graph.AddDependency("node4", "node2"))
	assert.NoError(t, graph.AddDependency("node4", "node3"))
	assert.NoError(t, graph.AddDependency("node5", "node4"))

	levels, err := graph.GetExecutionLevels()

	assert.NoError(t, err)
	assert.Equal(t, 4, len(levels))

	// Level 0: node1
	assert.Equal(t, 1, len(levels[0]))
	assert.Equal(t, "node1", levels[0][0].ID)

	// Level 1: node2, node3
	assert.Equal(t, 2, len(levels[1]))
	nodeIDs := []string{levels[1][0].ID, levels[1][1].ID}
	assert.Contains(t, nodeIDs, "node2")
	assert.Contains(t, nodeIDs, "node3")

	// Level 2: node4
	assert.Equal(t, 1, len(levels[2]))
	assert.Equal(t, "node4", levels[2][0].ID)

	// Level 3: node5
	assert.Equal(t, 1, len(levels[3]))
	assert.Equal(t, "node5", levels[3][0].ID)
}

func TestGraph_FindPath(t *testing.T) {
	graph := NewGraph()

	// Create a graph: node1 -> node2 -> node3
	node1 := &Node{ID: "node1"}
	node2 := &Node{ID: "node2"}
	node3 := &Node{ID: "node3"}

	assert.NoError(t, graph.AddNode(node1))
	assert.NoError(t, graph.AddNode(node2))
	assert.NoError(t, graph.AddNode(node3))

	assert.NoError(t, graph.AddDependency("node2", "node1"))
	assert.NoError(t, graph.AddDependency("node3", "node2"))

	// Test finding existing path
	path, found := graph.FindPath("node3", "node1")
	assert.True(t, found)
	assert.Equal(t, 3, len(path))
	assert.Equal(t, "node3", path[0])
	assert.Equal(t, "node2", path[1])
	assert.Equal(t, "node1", path[2])

	// Test no path exists
	path, found = graph.FindPath("node1", "node3")
	assert.False(t, found)
	assert.Nil(t, path)

	// Test same node
	path, found = graph.FindPath("node1", "node1")
	assert.True(t, found)
	assert.Equal(t, 1, len(path))
	assert.Equal(t, "node1", path[0])
}

func TestGraph_IsReachable(t *testing.T) {
	graph := NewGraph()

	node1 := &Node{ID: "node1"}
	node2 := &Node{ID: "node2"}
	node3 := &Node{ID: "node3"}

	assert.NoError(t, graph.AddNode(node1))
	assert.NoError(t, graph.AddNode(node2))
	assert.NoError(t, graph.AddNode(node3))

	assert.NoError(t, graph.AddDependency("node2", "node1"))
	assert.NoError(t, graph.AddDependency("node3", "node2"))

	// Test reachability
	assert.True(t, graph.IsReachable("node3", "node1"))
	assert.True(t, graph.IsReachable("node2", "node1"))
	assert.False(t, graph.IsReachable("node1", "node3"))
	assert.True(t, graph.IsReachable("node1", "node1"))
}
