package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolutionContextDistinctHyphenatedPairs(t *testing.T) {
	t.Parallel()
	config := &schema.AtmosConfiguration{}
	ctx := NewResolutionContext()
	first := DependencyNode{Stack: "a-b", Component: "c"}
	second := DependencyNode{Stack: "a", Component: "b-c"}
	require.NoError(t, ctx.Push(config, first))
	require.NoError(t, ctx.Push(config, second), "distinct pairs must not be reported as a cycle")
	require.Len(t, ctx.Visited, 2)
	assert.ErrorIs(t, ctx.Push(config, first), errUtils.ErrCircularDependency)
	cloned := ctx.Clone()
	ctx.Pop(config)
	require.Len(t, ctx.Visited, 1)
	require.NoError(t, ctx.Push(config, second), "popped nodes can be visited again")
	assert.ErrorIs(t, cloned.Push(config, second), errUtils.ErrCircularDependency)
	ctx.Pop(config)
	ctx.Pop(config)
	assert.Empty(t, ctx.Visited)
	require.NoError(t, ctx.Push(config, first))
}
