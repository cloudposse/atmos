package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecFunction_Execute_Basic(t *testing.T) {
	fn := NewExecFunction()

	// Test simple echo command (echo adds a newline).
	result, err := fn.Execute(context.Background(), "echo hello", nil)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", result)
}

func TestExecFunction_Execute_EmptyArgs(t *testing.T) {
	fn := NewExecFunction()

	// Empty args should return error.
	_, err := fn.Execute(context.Background(), "", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArguments)
}

func TestExecFunction_Execute_WhitespaceOnly(t *testing.T) {
	fn := NewExecFunction()

	// Whitespace-only args should return error.
	_, err := fn.Execute(context.Background(), "   ", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArguments)
}

func TestExecFunction_Execute_JSONOutput(t *testing.T) {
	fn := NewExecFunction()

	// Test command that outputs JSON.
	result, err := fn.Execute(context.Background(), `echo '{"key": "value"}'`, nil)
	require.NoError(t, err)

	// Result should be parsed as map.
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", m["key"])
}

func TestExecFunction_Execute_JSONArray(t *testing.T) {
	fn := NewExecFunction()

	// Test command that outputs JSON array.
	result, err := fn.Execute(context.Background(), `echo '[1, 2, 3]'`, nil)
	require.NoError(t, err)

	// Result should be parsed as slice.
	arr, ok := result.([]any)
	require.True(t, ok)
	assert.Len(t, arr, 3)
}

func TestExecFunction_Execute_NonJSONOutput(t *testing.T) {
	fn := NewExecFunction()

	// Test command that outputs non-JSON (echo adds a newline).
	result, err := fn.Execute(context.Background(), "echo 'not json'", nil)
	require.NoError(t, err)
	assert.Equal(t, "not json\n", result)
}

func TestNewExecFunction(t *testing.T) {
	fn := NewExecFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagExec, fn.Name())
	assert.Equal(t, PreMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}
