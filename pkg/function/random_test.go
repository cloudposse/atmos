package function

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomFunction_Execute_ErrorCases(t *testing.T) {
	fn := NewRandomFunction()

	tests := []struct {
		name        string
		args        string
		expectError error
		errorMsg    string
	}{
		{
			name:        "invalid max value",
			args:        "not-a-number",
			expectError: ErrInvalidArguments,
			errorMsg:    "invalid max value",
		},
		{
			name:        "invalid min value",
			args:        "not-a-number 100",
			expectError: ErrInvalidArguments,
			errorMsg:    "invalid min value",
		},
		{
			name:        "invalid max value with valid min",
			args:        "10 not-a-number",
			expectError: ErrInvalidArguments,
			errorMsg:    "invalid max value",
		},
		{
			name:        "too many arguments",
			args:        "1 2 3",
			expectError: ErrInvalidArguments,
			errorMsg:    "accepts 0, 1, or 2 arguments",
		},
		{
			name:        "min equals max",
			args:        "10 10",
			expectError: ErrInvalidArguments,
			errorMsg:    "min value",
		},
		{
			name:        "min greater than max",
			args:        "100 10",
			expectError: ErrInvalidArguments,
			errorMsg:    "min value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fn.Execute(context.Background(), tt.args, nil)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.expectError))
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

func TestRandomFunction_Execute_BoundaryValues(t *testing.T) {
	fn := NewRandomFunction()

	// Test with min=0, max=1 (only two possible values).
	for i := 0; i < 10; i++ {
		result, err := fn.Execute(context.Background(), "0 1", nil)
		require.NoError(t, err)
		val, ok := result.(int)
		require.True(t, ok)
		assert.GreaterOrEqual(t, val, 0)
		assert.LessOrEqual(t, val, 1)
	}
}

func TestRandomFunction_Execute_NegativeValues(t *testing.T) {
	fn := NewRandomFunction()

	// Test with negative min.
	result, err := fn.Execute(context.Background(), "-10 10", nil)
	require.NoError(t, err)
	val, ok := result.(int)
	require.True(t, ok)
	assert.GreaterOrEqual(t, val, -10)
	assert.LessOrEqual(t, val, 10)
}

func TestNewRandomFunction(t *testing.T) {
	fn := NewRandomFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagRandom, fn.Name())
	assert.Equal(t, PreMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}

func TestGenerateRandom(t *testing.T) {
	// Test basic range.
	for i := 0; i < 100; i++ {
		result, err := generateRandom(0, 100)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, result, 0)
		assert.LessOrEqual(t, result, 100)
	}

	// Test error case.
	_, err := generateRandom(100, 50)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidArguments))
}
