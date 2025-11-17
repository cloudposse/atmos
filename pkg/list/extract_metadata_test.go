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
					"stack":            "plat-ue2-dev",
					"component":        "vpc",
					"component_type":   "terraform",
					"component_folder": "vpc-base",
					"type":             "real",
					"enabled":          true,
					"locked":           false,
					"component_base":   "vpc-base",
					"inherits":         "vpc/defaults",
					"description":      "VPC infrastructure",
					"metadata": map[string]any{
						"type":        "real",
						"enabled":     true,
						"locked":      false,
						"component":   "vpc-base",
						"inherits":    []interface{}{"vpc/defaults"},
						"description": "VPC infrastructure",
					},
					"vars":     map[string]any(nil),
					"settings": map[string]any(nil),
					"env":      map[string]any(nil),
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
					"stack":            "plat-ue2-prod",
					"component":        "eks",
					"component_type":   "terraform",
					"component_folder": "eks-base",
					"type":             "real",
					"enabled":          true,
					"locked":           true,
					"component_base":   "eks-base",
					"inherits":         "eks/defaults, eks/prod-overrides",
					"description":      "EKS cluster",
					"metadata": map[string]any{
						"type":        "real",
						"enabled":     true,
						"locked":      true,
						"component":   "eks-base",
						"inherits":    []interface{}{"eks/defaults", "eks/prod-overrides"},
						"description": "EKS cluster",
					},
					"vars":     map[string]any(nil),
					"settings": map[string]any(nil),
					"env":      map[string]any(nil),
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
					"stack":            "test",
					"component":        "minimal",
					"component_type":   "terraform",
					"component_folder": "minimal", // Uses component name when metadata.component is not set.
					"type":             "real",    // Defaults to "real" since abstract components are filtered.
					"enabled":          true,      // Defaults to true.
					"locked":           false,
					"component_base":   "",
					"inherits":         "",
					"description":      "",
					"metadata":         map[string]any{},
					"vars":             map[string]any(nil),
					"settings":         map[string]any(nil),
					"env":              map[string]any(nil),
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
					"stack":            "plat-ue2-dev",
					"component":        "vpc",
					"component_type":   "terraform",
					"component_folder": "vpc", // Uses component name when metadata.component is not set.
					"type":             "real",
					"enabled":          true,
					"locked":           false,
					"component_base":   "",
					"inherits":         "",
					"description":      "Development VPC",
					"metadata": map[string]any{
						"type":        "real",
						"enabled":     true,
						"description": "Development VPC",
					},
					"vars":     map[string]any(nil),
					"settings": map[string]any(nil),
					"env":      map[string]any(nil),
				},
				{
					"stack":            "plat-ue2-prod",
					"component":        "eks",
					"component_type":   "terraform",
					"component_folder": "eks-base",
					"type":             "real",
					"enabled":          true,
					"locked":           true,
					"component_base":   "eks-base",
					"inherits":         "",
					"description":      "",
					"metadata": map[string]any{
						"type":      "real",
						"enabled":   true,
						"locked":    true,
						"component": "eks-base",
					},
					"vars":     map[string]any(nil),
					"settings": map[string]any(nil),
					"env":      map[string]any(nil),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractMetadata(tc.instances)

			// Check length matches.
			assert.Len(t, result, len(tc.expected))

			// Check each item's fields (excluding status which contains ANSI codes).
			for i := range result {
				if i < len(tc.expected) {
					// Verify status field exists and is non-empty.
					assert.Contains(t, result[i], "status")
					assert.NotEmpty(t, result[i]["status"])

					// Check all other fields match expected.
					for key, expectedVal := range tc.expected[i] {
						if key != "status" {
							assert.Equal(t, expectedVal, result[i][key], "mismatch for key %s", key)
						}
					}
				}
			}
		})
	}
}

func TestExtractMetadata_IncludesVarsSettingsEnv(t *testing.T) {
	instances := []schema.Instance{
		{
			Component:     "vpc",
			Stack:         "plat-ue2-dev",
			ComponentType: "terraform",
			Metadata: map[string]any{
				"type":        "real",
				"enabled":     true,
				"description": "VPC infrastructure",
			},
			Vars: map[string]any{
				"region":      "us-east-2",
				"environment": "dev",
				"tags": map[string]string{
					"Team": "platform",
					"Env":  "dev",
				},
			},
			Settings: map[string]any{
				"spacelift": map[string]any{
					"workspace_enabled": true,
				},
			},
			Env: map[string]any{
				"AWS_REGION": "us-east-2",
			},
		},
	}

	result := ExtractMetadata(instances)

	assert.Len(t, result, 1)

	// Verify status is included.
	assert.Contains(t, result[0], "status")
	assert.NotEmpty(t, result[0]["status"])

	// Verify vars are included
	assert.Contains(t, result[0], "vars")
	vars := result[0]["vars"].(map[string]any)
	assert.Equal(t, "us-east-2", vars["region"])
	assert.Equal(t, "dev", vars["environment"])

	// Verify settings are included
	assert.Contains(t, result[0], "settings")
	settings := result[0]["settings"].(map[string]any)
	spacelift := settings["spacelift"].(map[string]any)
	assert.Equal(t, true, spacelift["workspace_enabled"])

	// Verify env is included
	assert.Contains(t, result[0], "env")
	env := result[0]["env"].(map[string]any)
	assert.Equal(t, "us-east-2", env["AWS_REGION"])
}

func TestGetStatusIndicator(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		locked   bool
		contains string // Check if output contains the dot character
	}{
		{
			name:     "enabled and not locked shows green",
			enabled:  true,
			locked:   false,
			contains: "●",
		},
		{
			name:     "locked shows red",
			enabled:  true,
			locked:   true,
			contains: "●",
		},
		{
			name:     "disabled shows gray",
			enabled:  false,
			locked:   false,
			contains: "●",
		},
		{
			name:     "disabled and locked shows red (locked takes precedence)",
			enabled:  false,
			locked:   true,
			contains: "●",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStatusIndicator(tt.enabled, tt.locked)
			// Always contains the status dot.
			assert.Contains(t, result, tt.contains)
			// Result is non-empty (may or may not have ANSI codes depending on TTY).
			assert.NotEmpty(t, result)
		})
	}
}
