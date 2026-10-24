package convert

import (
	"errors"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

func TestSequenceCopiesInput(t *testing.T) {
	t.Parallel()
	for _, value := range []starlark.Value{starlark.Tuple{starlark.String("first"), starlark.String("last")}, starlark.NewList([]starlark.Value{starlark.String("first"), starlark.String("last")})} {
		result, err := Sequence(value)
		require.NoError(t, err)
		assert.Equal(t, []starlark.Value{starlark.String("first"), starlark.String("last")}, result)
		result[0] = starlark.None
		assert.Equal(t, starlark.String("first"), value.(starlark.Indexable).Index(0))
	}
	_, err := Sequence(starlark.None)
	require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
	assert.EqualError(t, err, "expected a list or tuple, got NoneType")
}

func TestArgumentErrorPreservesClassificationAndCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("parse failed")
	err := ArgumentCause(cause, "invalid input %q", "value")
	assert.EqualError(t, err, `invalid input "value"`)
	assert.ErrorIs(t, err, cause)
	assert.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
	assert.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.NotErrorIs(t, err, errors.New("other"))
	plain := InvalidArgument("missing %s", "value")
	assert.EqualError(t, plain, "missing value")
	assert.Nil(t, errors.Unwrap(plain))
}

func TestDictionaryStableOrderAndImmutableSnapshot(t *testing.T) {
	t.Parallel()
	for range 100 {
		zones := []string{"east", "west"}
		input := map[string]any{"zones": zones, "replicas": 3, "service": "api", "enabled": true, "optional": nil}
		result, err := Dictionary(input)
		require.NoError(t, err)
		const expected = `{"enabled": True, "optional": None, "replicas": 3, "service": "api", "zones": ["east", "west"]}`
		require.Equal(t, expected, result.String(), "script iteration and printed output must be deterministic")

		input["service"] = "changed"
		zones[0] = "changed"
		assert.Equal(t, expected, result.String(), "native inputs must be copied")
		assert.Error(t, result.SetKey(starlark.String("service"), starlark.String("changed")))
		list, found, err := result.Get(starlark.String("zones"))
		require.NoError(t, err)
		require.True(t, found)
		assert.Error(t, list.(*starlark.List).Append(starlark.String("changed")), "nested input lists must be frozen")
	}
}

func TestDictionaryEmptyAndUnsupportedInputs(t *testing.T) {
	t.Parallel()
	empty, err := Dictionary(nil)
	require.NoError(t, err)
	assert.Equal(t, "{}", empty.String())
	result, err := Dictionary(map[string]any{"invalid": struct{}{}})
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
	assert.EqualError(t, err, `unsupported input type struct {} for "invalid"`)
}
