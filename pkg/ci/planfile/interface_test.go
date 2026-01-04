package planfile

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestDefaultKeyPattern(t *testing.T) {
	pattern := DefaultKeyPattern()
	assert.Equal(t, "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan", pattern.Pattern)
}

func TestKeyPatternGenerateKey(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		ctx         *KeyContext
		expected    string
		expectError bool
	}{
		{
			name:    "default pattern",
			pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan",
			ctx: &KeyContext{
				Stack:     "plat-ue2-dev",
				Component: "vpc",
				SHA:       "abc123",
			},
			expected: "plat-ue2-dev/vpc/abc123.tfplan",
		},
		{
			name:    "pattern with branch",
			pattern: "{{ .Branch }}/{{ .Stack }}/{{ .Component }}.tfplan",
			ctx: &KeyContext{
				Stack:     "plat-ue2-dev",
				Component: "vpc",
				Branch:    "feature-branch",
			},
			expected: "feature-branch/plat-ue2-dev/vpc.tfplan",
		},
		{
			name:    "missing required SHA returns error",
			pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan",
			ctx: &KeyContext{
				Stack:     "plat-ue2-dev",
				Component: "vpc",
				// SHA is empty
			},
			expectError: true,
		},
		{
			name:    "component path without required fields",
			pattern: "{{ .ComponentPath }}/{{ .SHA }}.tfplan",
			ctx: &KeyContext{
				ComponentPath: "components/terraform/vpc",
				SHA:           "abc123",
			},
			expected: "components/terraform/vpc/abc123.tfplan",
		},
		{
			name:    "optional fields can be empty",
			pattern: "{{ .Stack }}/{{ .Component }}/{{ .BaseSHA }}.tfplan",
			ctx: &KeyContext{
				Stack:     "plat-ue2-dev",
				Component: "vpc",
				// BaseSHA is optional, can be empty
			},
			expected: "plat-ue2-dev/vpc/.tfplan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := KeyPattern{Pattern: tt.pattern}
			key, err := pattern.GenerateKey(tt.ctx)
			if tt.expectError {
				assert.ErrorIs(t, err, errUtils.ErrPlanfileKeyInvalid)
				assert.Empty(t, key)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, key)
			}
		})
	}
}
