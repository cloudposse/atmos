package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLiteralFunction(t *testing.T) {
	fn := NewLiteralFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagLiteral, fn.Name())
	assert.Equal(t, PreMerge, fn.Phase())
}

func TestLiteralFunction_Execute(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		expected string
	}{
		{
			name:     "terraform template syntax",
			args:     "{{external.email}}",
			expected: "{{external.email}}",
		},
		{
			name:     "helm template syntax",
			args:     "{{ .Values.ingress.class }}",
			expected: "{{ .Values.ingress.class }}",
		},
		{
			name:     "bash variable syntax",
			args:     "${USER}",
			expected: "${USER}",
		},
		{
			name:     "leading and trailing whitespace trimmed",
			args:     "  hello world  ",
			expected: "hello world",
		},
		{
			name:     "empty string",
			args:     "",
			expected: "",
		},
		{
			name:     "multiline value",
			args:     "#!/bin/bash\necho \"Hello ${USER}\"",
			expected: "#!/bin/bash\necho \"Hello ${USER}\"",
		},
		{
			name:     "plain string preserved",
			args:     "simple-value",
			expected: "simple-value",
		},
	}

	fn := NewLiteralFunction()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := fn.Execute(context.Background(), tt.args, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
