package convert

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestProcessInput(t *testing.T) {
	t.Parallel()
	input, err := ProcessInput(starlark.None)
	require.NoError(t, err)
	assert.Nil(t, input)
	for _, value := range []string{"", "secret\n\x00café"} {
		input, err := ProcessInput(starlark.String(value))
		require.NoError(t, err)
		require.NotNil(t, input)
		assert.Equal(t, value, *input)
	}
	for _, value := range []starlark.Value{starlark.MakeInt(42), starlark.False, starlark.NewList([]starlark.Value{starlark.String("private-payload")})} {
		input, err := ProcessInput(value)
		require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
		assert.EqualError(t, err, "stdin must be a string or None")
		assert.Nil(t, input)
	}
}
