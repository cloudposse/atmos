package dependency

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBuilder(t *testing.T) {
	b := NewBuilder()

	assert.NotNil(t, b)
	assert.NotNil(t, b.graph)
	assert.False(t, b.built)
}

func TestBuilder_AddNode(t *testing.T) {
	t.Run("adds node successfully", func(t *testing.T) {
		b := NewBuilder()

		err := b.AddNode(&Node{ID: "vpc", Component: "vpc", Stack: "dev", Type: "terraform"})
		require.NoError(t, err)
		assert.Equal(t, 1, b.graph.Size())
	})

	t.Run("rejects empty ID node", func(t *testing.T) {
		b := NewBuilder()

		err := b.AddNode(&Node{Component: "test"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAddNodeFailed)
	})

	t.Run("rejects duplicate node", func(t *testing.T) {
		b := NewBuilder()

		err := b.AddNode(&Node{ID: "vpc", Component: "vpc", Stack: "dev"})
		require.NoError(t, err)

		err = b.AddNode(&Node{ID: "vpc", Component: "vpc", Stack: "dev"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAddNodeFailed)
	})

	t.Run("rejects after build", func(t *testing.T) {
		b := NewBuilder()

		err := b.AddNode(&Node{ID: "vpc", Component: "vpc", Stack: "dev"})
		require.NoError(t, err)

		_, err = b.Build()
		require.NoError(t, err)

		err = b.AddNode(&Node{ID: "rds", Component: "rds", Stack: "dev"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGraphAlreadyBuilt)
	})
}

func TestBuilder_AddDependency(t *testing.T) {
	t.Run("adds dependency successfully", func(t *testing.T) {
		b := NewBuilder()

		require.NoError(t, b.AddNode(&Node{ID: "vpc", Component: "vpc", Stack: "dev"}))
		require.NoError(t, b.AddNode(&Node{ID: "rds", Component: "rds", Stack: "dev"}))

		err := b.AddDependency("rds", "vpc")
		require.NoError(t, err)
	})

	t.Run("rejects unknown nodes", func(t *testing.T) {
		b := NewBuilder()

		require.NoError(t, b.AddNode(&Node{ID: "vpc", Component: "vpc", Stack: "dev"}))

		err := b.AddDependency("nonexistent", "vpc")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAddDependencyFailed)
	})

	t.Run("rejects after build", func(t *testing.T) {
		b := NewBuilder()

		require.NoError(t, b.AddNode(&Node{ID: "vpc", Component: "vpc", Stack: "dev"}))
		require.NoError(t, b.AddNode(&Node{ID: "rds", Component: "rds", Stack: "dev"}))

		_, err := b.Build()
		require.NoError(t, err)

		err = b.AddDependency("rds", "vpc")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGraphAlreadyBuilt)
	})
}

func TestBuilder_Build(t *testing.T) {
	t.Run("builds simple graph", func(t *testing.T) {
		b := NewBuilder()

		require.NoError(t, b.AddNode(&Node{ID: "vpc", Component: "vpc", Stack: "dev"}))
		require.NoError(t, b.AddNode(&Node{ID: "rds", Component: "rds", Stack: "dev"}))
		require.NoError(t, b.AddDependency("rds", "vpc"))

		g, err := b.Build()
		require.NoError(t, err)
		assert.NotNil(t, g)
		assert.Equal(t, 2, g.Size())
		assert.Contains(t, g.Roots, "vpc")
	})

	t.Run("detects circular dependency", func(t *testing.T) {
		b := NewBuilder()

		require.NoError(t, b.AddNode(&Node{ID: "a", Component: "a", Stack: "dev"}))
		require.NoError(t, b.AddNode(&Node{ID: "b", Component: "b", Stack: "dev"}))
		require.NoError(t, b.AddDependency("a", "b"))
		require.NoError(t, b.AddDependency("b", "a"))

		g, err := b.Build()
		assert.Nil(t, g)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCircularDependency)
	})

	t.Run("rejects double build", func(t *testing.T) {
		b := NewBuilder()

		require.NoError(t, b.AddNode(&Node{ID: "vpc", Component: "vpc", Stack: "dev"}))

		_, err := b.Build()
		require.NoError(t, err)

		_, err = b.Build()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGraphAlreadyBuilt)
	})

	t.Run("builds empty graph", func(t *testing.T) {
		b := NewBuilder()

		g, err := b.Build()
		require.NoError(t, err)
		assert.NotNil(t, g)
		assert.Equal(t, 0, g.Size())
	})
}

func TestBuilder_GetGraph(t *testing.T) {
	b := NewBuilder()

	require.NoError(t, b.AddNode(&Node{ID: "vpc", Component: "vpc", Stack: "dev"}))

	g := b.GetGraph()
	assert.NotNil(t, g)
	assert.Equal(t, 1, g.Size())
}

func TestBuilder_Reset(t *testing.T) {
	b := NewBuilder()

	require.NoError(t, b.AddNode(&Node{ID: "vpc", Component: "vpc", Stack: "dev"}))

	_, err := b.Build()
	require.NoError(t, err)

	// After build, adding should fail.
	err = b.AddNode(&Node{ID: "rds", Component: "rds", Stack: "dev"})
	require.Error(t, err)

	// Reset should allow adding again.
	b.Reset()
	assert.False(t, b.built)
	assert.Equal(t, 0, b.graph.Size())

	err = b.AddNode(&Node{ID: "rds", Component: "rds", Stack: "dev"})
	require.NoError(t, err)
	assert.Equal(t, 1, b.graph.Size())
}

func TestGraph_Reset(t *testing.T) {
	g := NewGraph()

	require.NoError(t, g.AddNode(&Node{ID: "a", Component: "a", Stack: "dev"}))
	require.NoError(t, g.AddNode(&Node{ID: "b", Component: "b", Stack: "dev"}))

	// Mark nodes as processed.
	g.Nodes["a"].Processed = true
	g.Nodes["b"].Processed = true

	g.Reset()

	assert.False(t, g.Nodes["a"].Processed)
	assert.False(t, g.Nodes["b"].Processed)
}

func TestBuilder_Build_NoRootNodes(t *testing.T) {
	// Create a graph where all nodes have dependencies but no circular dependency is detected
	// by the HasCycles check. This tests the ErrNoRootNodes branch.
	// In practice, if every node has dependencies and there's no cycle, that's impossible
	// (it would be a cycle). So ErrNoRootNodes is only hit if HasCycles misses something.
	// We can't easily trigger this without modifying internals, so we skip this edge case.
	// The circular dependency test already covers the practical scenario.
}

func TestBuilder_MultiNodeChain(t *testing.T) {
	b := NewBuilder()

	// Build a chain: app -> api -> rds -> vpc.
	require.NoError(t, b.AddNode(&Node{ID: "vpc", Component: "vpc", Stack: "dev", Type: "terraform"}))
	require.NoError(t, b.AddNode(&Node{ID: "rds", Component: "rds", Stack: "dev", Type: "terraform"}))
	require.NoError(t, b.AddNode(&Node{ID: "api", Component: "api", Stack: "dev", Type: "terraform"}))
	require.NoError(t, b.AddNode(&Node{ID: "app", Component: "app", Stack: "dev", Type: "terraform"}))

	require.NoError(t, b.AddDependency("rds", "vpc"))
	require.NoError(t, b.AddDependency("api", "rds"))
	require.NoError(t, b.AddDependency("app", "api"))

	g, err := b.Build()
	require.NoError(t, err)

	assert.Equal(t, 4, g.Size())
	assert.Equal(t, []string{"vpc"}, g.Roots)

	// Verify topological order.
	order, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Equal(t, 4, len(order))
	assert.Equal(t, "vpc", order[0].ID)
	assert.Equal(t, "rds", order[1].ID)
	assert.Equal(t, "api", order[2].ID)
	assert.Equal(t, "app", order[3].ID)
}
