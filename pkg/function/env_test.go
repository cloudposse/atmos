package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEnvFunction(t *testing.T) {
	fn := NewEnvFunction()

	assert.Equal(t, "env", fn.Name())
	assert.Empty(t, fn.Aliases())
	assert.Equal(t, PreMerge, fn.Phase())
}

func TestEnvFunctionExecute(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		envVars     map[string]string
		contextEnv  map[string]string
		expected    any
		expectError bool
	}{
		{
			name:     "existing env var",
			args:     "TEST_ENV_VAR",
			envVars:  map[string]string{"TEST_ENV_VAR": "test_value"},
			expected: "test_value",
		},
		{
			name:     "missing env var returns empty",
			args:     "NONEXISTENT_VAR",
			expected: "",
		},
		{
			name:     "missing env var with default",
			args:     "NONEXISTENT_VAR default_value",
			expected: "default_value",
		},
		{
			name:     "existing env var ignores default",
			args:     "TEST_ENV_VAR default",
			envVars:  map[string]string{"TEST_ENV_VAR": "actual"},
			expected: "actual",
		},
		{
			name:       "context env takes precedence",
			args:       "MY_VAR",
			envVars:    map[string]string{"MY_VAR": "os_value"},
			contextEnv: map[string]string{"MY_VAR": "context_value"},
			expected:   "context_value",
		},
		{
			name:        "empty args returns error",
			args:        "",
			expectError: true,
		},
		{
			name:        "too many args returns error",
			args:        "VAR default extra",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables.
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			fn := NewEnvFunction()

			var execCtx *ExecutionContext
			if tt.contextEnv != nil {
				execCtx = &ExecutionContext{Env: tt.contextEnv}
			}

			result, err := fn.Execute(context.Background(), tt.args, execCtx)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnvFunctionWithQuotedArgs(t *testing.T) {
	fn := NewEnvFunction()

	result, err := fn.Execute(context.Background(), `MY_VAR "default with spaces"`, nil)
	require.NoError(t, err)
	assert.Equal(t, "default with spaces", result)
}

func TestSplitStringByDelimiter(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    []string
		expectError bool
	}{
		{
			name:     "simple split",
			input:    "a b c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "quoted string",
			input:    `a "b c" d`,
			expected: []string{"a", "b c", "d"},
		},
		{
			name:     "single quoted",
			input:    `a 'b c' d`,
			expected: []string{"a", "b c", "d"},
		},
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:        "unclosed quote",
			input:       `a "b c`,
			expectError: true,
		},
		{
			name:     "multiple spaces",
			input:    "a   b   c",
			expected: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := splitStringByDelimiter(tt.input, ' ')

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
