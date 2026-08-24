package planfile

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestMetadataValidate(t *testing.T) {
	tests := []struct {
		name        string
		metadata    *Metadata
		expectError bool
	}{
		{
			name: "valid metadata",
			metadata: func() *Metadata {
				m := &Metadata{}
				m.Stack = "dev"
				m.Component = "vpc"
				m.SHA = "abc123"
				return m
			}(),
			expectError: false,
		},
		{
			name: "empty stack",
			metadata: func() *Metadata {
				m := &Metadata{}
				m.Stack = ""
				m.Component = "vpc"
				m.SHA = "abc123"
				return m
			}(),
			expectError: true,
		},
		{
			name: "empty component",
			metadata: func() *Metadata {
				m := &Metadata{}
				m.Stack = "dev"
				m.Component = ""
				m.SHA = "abc123"
				return m
			}(),
			expectError: true,
		},
		{
			name: "empty SHA",
			metadata: func() *Metadata {
				m := &Metadata{}
				m.Stack = "dev"
				m.Component = "vpc"
				m.SHA = ""
				return m
			}(),
			expectError: true,
		},
		{
			name: "all empty",
			metadata: func() *Metadata {
				m := &Metadata{}
				m.Stack = ""
				m.Component = ""
				m.SHA = ""
				return m
			}(),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.metadata.Validate()
			if tt.expectError {
				assert.ErrorIs(t, err, errUtils.ErrPlanfileMetadataInvalid)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultKeyPattern(t *testing.T) {
	pattern := DefaultKeyPattern()
	assert.Equal(t, "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan.tar", pattern.Pattern)
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
