package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecFunction_Execute_InvalidArgs(t *testing.T) {
	fn := NewExecFunction()

	tests := []struct {
		name string
		args string
	}{
		{name: "empty args", args: ""},
		{name: "whitespace only", args: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fn.Execute(context.Background(), tt.args, nil)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidArguments)
		})
	}
}

func TestExecFunction_Execute_OutputParsing(t *testing.T) {
	fn := NewExecFunction()

	tests := []struct {
		name      string
		command   string
		checkFunc func(t *testing.T, result any)
	}{
		{
			name:    "simple string output",
			command: "echo hello",
			checkFunc: func(t *testing.T, result any) {
				assert.Equal(t, "hello\n", result)
			},
		},
		{
			name:    "JSON object output",
			command: `echo '{"key": "value"}'`,
			checkFunc: func(t *testing.T, result any) {
				m, ok := result.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "value", m["key"])
			},
		},
		{
			name:    "JSON array output",
			command: `echo '[1, 2, 3]'`,
			checkFunc: func(t *testing.T, result any) {
				arr, ok := result.([]any)
				require.True(t, ok)
				assert.Len(t, arr, 3)
			},
		},
		{
			name:    "non-JSON output",
			command: "echo 'not json'",
			checkFunc: func(t *testing.T, result any) {
				assert.Equal(t, "not json\n", result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := fn.Execute(context.Background(), tt.command, nil)
			require.NoError(t, err)
			tt.checkFunc(t, result)
		})
	}
}

func TestExecFunction_Metadata(t *testing.T) {
	fn := NewExecFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagExec, fn.Name())
	assert.Equal(t, PreMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}
