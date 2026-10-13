package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStarlarkFunction(t *testing.T) {
	fn, err := DefaultRegistry().Get("starlark")
	require.NoError(t, err)
	assert.Equal(t, PostMerge, fn.Phase())
	_, err = fn.Execute(context.Background(), "return 1", nil)
	require.ErrorIs(t, err, ErrInvalidArguments)
	value, err := fn.Execute(context.Background(), "return 1", &ExecutionContext{EvaluateValue: func(ctx context.Context, body string) (any, error) {
		assert.Equal(t, "return 1", body)
		return int64(1), ctx.Err()
	}})
	require.NoError(t, err)
	assert.Equal(t, int64(1), value)
}
