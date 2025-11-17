package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractMetadata(t *testing.T) {
	testCases := []struct {
		name      string
		instances []schema.Instance
		expected  []map[string]any
	}{
		{
			name:      "empty instances",
			instances: []schema.Instance{},
			expected:  []map[string]any{},
		},
		{
			name: "single instance with metadata",
			instances: []schema.Instance{
				{
					Component:     "vpc",
					Stack:         "plat-ue2-dev",
					ComponentType: "terraform",
					Metadata: map[string]any{
						"type":        "real",
						"enabled":     true,
						"locked":      false,
						"component":   "vpc-base",
						"inherits":    []interface{}{"vpc/defaults"},
						"description": "VPC infrastructure",
					},
				},
			},
			expected: []map[string]any{
				{
					"stack":          "plat-ue2-dev",
					"component":      "vpc",
					"component_type": "terraform",
					"type":           "real",
					"enabled":        true,
					"locked":         false,
					"component_base": "vpc-base",
					"inherits":       "vpc/defaults",
					"description":    "VPC infrastructure",
					"metadata": map[string]any{
						"type":        "real",
						"enabled":     true,
						"locked":      false,
						"component":   "vpc-base",
						"inherits":    []interface{}{"vpc/defaults"},
						"description": "VPC infrastructure",
					},
				},
			},
		},
		{
			name: "instance with multiple inherits",
			instances: []schema.Instance{
				{
					Component:     "eks",
					Stack:         "plat-ue2-prod",
					ComponentType: "terraform",
					Metadata: map[string]any{
						"type":        "real",
						"enabled":     true,
						"locked":      true,
						"component":   "eks-base",
						"inherits":    []interface{}{"eks/defaults", "eks/prod-overrides"},
						"description": "EKS cluster",
					},
				},
			},
			expected: []map[string]any{
				{
					"stack":          "plat-ue2-prod",
					"component":      "eks",
					"component_type": "terraform",
					"type":           "real",
					"enabled":        true,
					"locked":         true,
					"component_base": "eks-base",
					"inherits":       "eks/defaults, eks/prod-overrides",
					"description":    "EKS cluster",
					"metadata": map[string]any{
						"type":        "real",
						"enabled":     true,
						"locked":      true,
						"component":   "eks-base",
						"inherits":    []interface{}{"eks/defaults", "eks/prod-overrides"},
						"description": "EKS cluster",
					},
				},
			},
		},
		{
			name: "instance with minimal metadata",
			instances: []schema.Instance{
				{
					Component:     "minimal",
					Stack:         "test",
					ComponentType: "terraform",
					Metadata:      map[string]any{},
				},
			},
			expected: []map[string]any{
				{
					"stack":          "test",
					"component":      "minimal",
					"component_type": "terraform",
					"type":           "",
					"enabled":        false,
					"locked":         false,
					"component_base": "",
					"inherits":       "",
					"description":    "",
					"metadata":       map[string]any{},
				},
			},
		},
		{
			name: "multiple instances with mixed metadata",
			instances: []schema.Instance{
				{
					Component:     "vpc",
					Stack:         "plat-ue2-dev",
					ComponentType: "terraform",
					Metadata: map[string]any{
						"type":        "real",
						"enabled":     true,
						"description": "Development VPC",
					},
				},
				{
					Component:     "eks",
					Stack:         "plat-ue2-prod",
					ComponentType: "terraform",
					Metadata: map[string]any{
						"type":      "real",
						"enabled":   true,
						"locked":    true,
						"component": "eks-base",
					},
				},
			},
			expected: []map[string]any{
				{
					"stack":          "plat-ue2-dev",
					"component":      "vpc",
					"component_type": "terraform",
					"type":           "real",
					"enabled":        true,
					"locked":         false,
					"component_base": "",
					"inherits":       "",
					"description":    "Development VPC",
					"metadata": map[string]any{
						"type":        "real",
						"enabled":     true,
						"description": "Development VPC",
					},
				},
				{
					"stack":          "plat-ue2-prod",
					"component":      "eks",
					"component_type": "terraform",
					"type":           "real",
					"enabled":        true,
					"locked":         true,
					"component_base": "eks-base",
					"inherits":       "",
					"description":    "",
					"metadata": map[string]any{
						"type":      "real",
						"enabled":   true,
						"locked":    true,
						"component": "eks-base",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractMetadata(tc.instances)
			assert.Equal(t, tc.expected, result)
		})
	}
}
