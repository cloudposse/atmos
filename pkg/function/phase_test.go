package function

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhase_String(t *testing.T) {
	tests := []struct {
		name     string
		phase    Phase
		expected string
	}{
		{
			name:     "PreMerge",
			phase:    PreMerge,
			expected: "pre-merge",
		},
		{
			name:     "PostMerge",
			phase:    PostMerge,
			expected: "post-merge",
		},
		{
			name:     "Unknown phase",
			phase:    Phase(99),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.phase.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPhase_Ordering(t *testing.T) {
	// Verify PreMerge executes before PostMerge (ordering matters for function execution).
	assert.True(t, PreMerge < PostMerge)
}
