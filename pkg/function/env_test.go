package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvFunction_Execute_EdgeCases(t *testing.T) {
	fn := NewEnvFunction()

	tests := []struct {
		name        string
		args        string
		setupEnv    map[string]string
		expected    any
		expectError bool
	}{
		{
			name:        "empty args returns error",
			args:        "",
			expectError: true,
		},
		{
			name:        "whitespace only returns error",
			args:        "   ",
			expectError: true,
		},
		{
			name:     "existing env var",
			args:     "TEST_ENV_VAR",
			setupEnv: map[string]string{"TEST_ENV_VAR": "test_value"},
			expected: "test_value",
		},
		{
			name:     "missing env var returns empty",
			args:     "NONEXISTENT_VAR_12345",
			expected: "",
		},
		{
			name:     "missing env var with default",
			args:     "NONEXISTENT_VAR_12345 default_value",
			expected: "default_value",
		},
		{
			name:     "existing env var ignores default",
			args:     "TEST_ENV_VAR fallback",
			setupEnv: map[string]string{"TEST_ENV_VAR": "actual_value"},
			expected: "actual_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables.
			for k, v := range tt.setupEnv {
				t.Setenv(k, v)
			}

			result, err := fn.Execute(context.Background(), tt.args, nil)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestEnvFunction_Execute_TooManyArgs(t *testing.T) {
	fn := NewEnvFunction()

	// Test with too many arguments.
	_, err := fn.Execute(context.Background(), "VAR default extra_arg", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 or 2 arguments")
}

func TestParseEnvArgs(t *testing.T) {
	tests := []struct {
		name            string
		args            string
		expectedName    string
		expectedDefault string
		expectError     bool
	}{
		{
			name:            "single argument",
			args:            "VAR_NAME",
			expectedName:    "VAR_NAME",
			expectedDefault: "",
		},
		{
			name:            "two arguments",
			args:            "VAR_NAME default_value",
			expectedName:    "VAR_NAME",
			expectedDefault: "default_value",
		},
		{
			name:            "with extra whitespace",
			args:            "  VAR_NAME   default_value  ",
			expectedName:    "VAR_NAME",
			expectedDefault: "default_value",
		},
		{
			name:        "empty args",
			args:        "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, def, err := parseEnvArgs(tt.args)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedName, name)
				assert.Equal(t, tt.expectedDefault, def)
			}
		})
	}
}

func TestLookupEnvFromContext(t *testing.T) {
	// Nil context.
	val, found := lookupEnvFromContext(nil, "TEST")
	assert.False(t, found)
	assert.Empty(t, val)

	// Nil stack info.
	execCtx := &ExecutionContext{}
	val, found = lookupEnvFromContext(execCtx, "TEST")
	assert.False(t, found)
	assert.Empty(t, val)
}

func TestEnvFunction_Metadata(t *testing.T) {
	fn := NewEnvFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagEnv, fn.Name())
	assert.Equal(t, PreMerge, fn.Phase())
}
