package provisioner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsBackendProvisionEnabled tests the isBackendProvisionEnabled function.
func TestIsBackendProvisionEnabled(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expected        bool
	}{
		{
			name: "enabled true",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"backend": map[string]any{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
		{
			name: "enabled false",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"backend": map[string]any{
						"enabled": false,
					},
				},
			},
			expected: false,
		},
		{
			name:            "no provision section",
			componentConfig: map[string]any{},
			expected:        false,
		},
		{
			name: "no backend section",
			componentConfig: map[string]any{
				"provision": map[string]any{},
			},
			expected: false,
		},
		{
			name: "no enabled field",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"backend": map[string]any{},
				},
			},
			expected: false,
		},
		{
			name: "provision is not a map",
			componentConfig: map[string]any{
				"provision": "invalid",
			},
			expected: false,
		},
		{
			name: "backend is not a map",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"backend": "invalid",
				},
			},
			expected: false,
		},
		{
			name: "enabled is not a bool",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"backend": map[string]any{
						"enabled": "true",
					},
				},
			},
			expected: false,
		},
		{
			name:            "nil config",
			componentConfig: nil,
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBackendProvisionEnabled(tt.componentConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractBackendComponent tests the extractBackendComponent function.
func TestExtractBackendComponent(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expected        string
	}{
		{
			name: "atmos_component present",
			componentConfig: map[string]any{
				"atmos_component": "vpc",
			},
			expected: "vpc",
		},
		{
			name: "component present",
			componentConfig: map[string]any{
				"component": "rds",
			},
			expected: "rds",
		},
		{
			name: "metadata.component present",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"component": "eks",
				},
			},
			expected: "eks",
		},
		{
			name: "atmos_component takes priority over component",
			componentConfig: map[string]any{
				"atmos_component": "vpc",
				"component":       "rds",
			},
			expected: "vpc",
		},
		{
			name: "component takes priority over metadata.component",
			componentConfig: map[string]any{
				"component": "rds",
				"metadata": map[string]any{
					"component": "eks",
				},
			},
			expected: "rds",
		},
		{
			name: "empty atmos_component falls through",
			componentConfig: map[string]any{
				"atmos_component": "",
				"component":       "rds",
			},
			expected: "rds",
		},
		{
			name: "empty component falls through to metadata",
			componentConfig: map[string]any{
				"component": "",
				"metadata": map[string]any{
					"component": "eks",
				},
			},
			expected: "eks",
		},
		{
			name:            "no component returns unknown",
			componentConfig: map[string]any{},
			expected:        "unknown",
		},
		{
			name: "metadata is not a map returns unknown",
			componentConfig: map[string]any{
				"metadata": "invalid",
			},
			expected: "unknown",
		},
		{
			name: "metadata.component is not a string",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"component": 123,
				},
			},
			expected: "unknown",
		},
		{
			name: "metadata.component is empty",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"component": "",
				},
			},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBackendComponent(tt.componentConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractBackendStack tests the extractBackendStack function.
func TestExtractBackendStack(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expected        string
	}{
		{
			name: "atmos_stack present",
			componentConfig: map[string]any{
				"atmos_stack": "dev-us-west-2",
			},
			expected: "dev-us-west-2",
		},
		{
			name:            "no atmos_stack returns unknown",
			componentConfig: map[string]any{},
			expected:        "unknown",
		},
		{
			name: "atmos_stack is empty returns unknown",
			componentConfig: map[string]any{
				"atmos_stack": "",
			},
			expected: "unknown",
		},
		{
			name: "atmos_stack is not a string",
			componentConfig: map[string]any{
				"atmos_stack": 123,
			},
			expected: "unknown",
		},
		{
			name:            "nil config returns unknown",
			componentConfig: nil,
			expected:        "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBackendStack(tt.componentConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}
