package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatStatusContext(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		parts    []string
		expected string
	}{
		{
			name:     "plan with stack and component",
			prefix:   "atmos",
			parts:    []string{"plan", "dev", "vpc"},
			expected: "atmos/plan/dev/vpc",
		},
		{
			name:     "plan with per-operation suffix",
			prefix:   "atmos",
			parts:    []string{"plan", "dev", "vpc", "add"},
			expected: "atmos/plan/dev/vpc/add",
		},
		{
			name:     "apply with destroy suffix",
			prefix:   "atmos",
			parts:    []string{"apply", "prod-us-east-1", "rds", "destroy"},
			expected: "atmos/apply/prod-us-east-1/rds/destroy",
		},
		{
			name:     "custom prefix",
			prefix:   "myorg",
			parts:    []string{"plan", "staging", "eks"},
			expected: "myorg/plan/staging/eks",
		},
		{
			name:     "prefix only",
			prefix:   "atmos",
			parts:    nil,
			expected: "atmos",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatStatusContext(tt.prefix, tt.parts...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatCheckRunName(t *testing.T) {
	// Verify legacy function still works.
	result := FormatCheckRunName("plan", "dev", "vpc")
	assert.Equal(t, "atmos/plan: dev/vpc", result)
}
