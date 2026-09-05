package provider

import (
	"errors"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatStatusContext(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		parts       []string
		expected    string
		expectError bool
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
		{
			name:        "empty prefix rejected",
			prefix:      "",
			parts:       []string{"plan", "dev", "vpc"},
			expectError: true,
		},
		{
			name:        "empty stack rejected",
			prefix:      "atmos",
			parts:       []string{"plan", "", "vpc"},
			expectError: true,
		},
		{
			name:        "empty component rejected",
			prefix:      "atmos",
			parts:       []string{"plan", "dev", ""},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatStatusContext(tt.prefix, tt.parts...)
			if tt.expectError {
				require.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrCIStatusContextIncomplete))
				assert.Empty(t, result)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatCheckRunName(t *testing.T) {
	// Verify legacy function still works.
	result := FormatCheckRunName("plan", "dev", "vpc")
	assert.Equal(t, "atmos/plan: dev/vpc", result)
}
