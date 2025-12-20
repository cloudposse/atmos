package extract

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAffected_EmptySlice(t *testing.T) {
	result := Affected(nil, false)
	assert.Empty(t, result)

	result = Affected([]schema.Affected{}, false)
	assert.Empty(t, result)
}

func TestAffected_SingleAffected(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			ComponentPath: "components/terraform/vpc",
			Stack:         "plat-ue2-dev",
			StackSlug:     "plat-ue2-dev",
			Affected:      "file",
			AffectedAll:   []string{"file", "config"},
			File:          "main.tf",
		},
	}

	result := Affected(affected, false)

	assert.Len(t, result, 1)
	assert.Equal(t, "vpc", result[0]["component"])
	assert.Equal(t, "terraform", result[0]["component_type"])
	assert.Equal(t, "plat-ue2-dev", result[0]["stack"])
	assert.Equal(t, "file", result[0]["affected"])
	assert.Equal(t, "file,config", result[0]["affected_all"])
	assert.Equal(t, "main.tf", result[0]["file"])
	assert.Equal(t, false, result[0]["is_dependent"])
	assert.Equal(t, 0, result[0]["depth"])
}

func TestAffected_WithDependents_NotFlattened(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			Stack:         "plat-ue2-dev",
			Affected:      "file",
			Dependents: []schema.Dependent{
				{
					Component:     "eks",
					ComponentType: "terraform",
					Stack:         "plat-ue2-dev",
				},
			},
		},
	}

	// Without flattening, dependents should not appear as separate rows.
	result := Affected(affected, false)

	assert.Len(t, result, 1)
	assert.Equal(t, "vpc", result[0]["component"])
	assert.Equal(t, 1, result[0]["dependents_count"])
}

func TestAffected_WithDependents_Flattened(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			Stack:         "plat-ue2-dev",
			Affected:      "file",
			Dependents: []schema.Dependent{
				{
					Component:     "eks",
					ComponentType: "terraform",
					Stack:         "plat-ue2-dev",
				},
				{
					Component:     "rds",
					ComponentType: "terraform",
					Stack:         "plat-ue2-dev",
				},
			},
		},
	}

	// With flattening, dependents should appear as separate rows.
	result := Affected(affected, true)

	assert.Len(t, result, 3)

	// First row is the affected component.
	assert.Equal(t, "vpc", result[0]["component"])
	assert.Equal(t, false, result[0]["is_dependent"])
	assert.Equal(t, 0, result[0]["depth"])

	// Second row is first dependent.
	assert.Equal(t, "eks", result[1]["component"])
	assert.Equal(t, true, result[1]["is_dependent"])
	assert.Equal(t, 1, result[1]["depth"])
	assert.Equal(t, "dependent", result[1]["affected"])

	// Third row is second dependent.
	assert.Equal(t, "rds", result[2]["component"])
	assert.Equal(t, true, result[2]["is_dependent"])
	assert.Equal(t, 1, result[2]["depth"])
}

func TestAffected_NestedDependents_Flattened(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			Stack:         "plat-ue2-dev",
			Affected:      "file",
			Dependents: []schema.Dependent{
				{
					Component:     "eks",
					ComponentType: "terraform",
					Stack:         "plat-ue2-dev",
					Dependents: []schema.Dependent{
						{
							Component:     "app",
							ComponentType: "terraform",
							Stack:         "plat-ue2-dev",
						},
					},
				},
			},
		},
	}

	result := Affected(affected, true)

	assert.Len(t, result, 3)

	// First row is the affected component.
	assert.Equal(t, "vpc", result[0]["component"])
	assert.Equal(t, 0, result[0]["depth"])

	// Second row is first level dependent.
	assert.Equal(t, "eks", result[1]["component"])
	assert.Equal(t, 1, result[1]["depth"])

	// Third row is nested dependent.
	assert.Equal(t, "app", result[2]["component"])
	assert.Equal(t, 2, result[2]["depth"])
}

func TestAffected_StatusIndicator_Enabled(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			Stack:         "plat-ue2-dev",
			Affected:      "file",
			Settings: map[string]any{
				"metadata": map[string]any{
					"enabled": true,
					"locked":  false,
				},
			},
		},
	}

	result := Affected(affected, false)

	assert.Len(t, result, 1)
	assert.Equal(t, true, result[0]["enabled"])
	assert.Equal(t, false, result[0]["locked"])
	// Status should be the green dot (enabled and not locked).
	assert.NotEmpty(t, result[0]["status"])
}

func TestAffected_StatusIndicator_Locked(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			Stack:         "plat-ue2-dev",
			Affected:      "file",
			Settings: map[string]any{
				"metadata": map[string]any{
					"enabled": true,
					"locked":  true,
				},
			},
		},
	}

	result := Affected(affected, false)

	assert.Len(t, result, 1)
	assert.Equal(t, true, result[0]["enabled"])
	assert.Equal(t, true, result[0]["locked"])
}

func TestAffected_StatusIndicator_Disabled(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			Stack:         "plat-ue2-dev",
			Affected:      "file",
			Settings: map[string]any{
				"metadata": map[string]any{
					"enabled": false,
					"locked":  false,
				},
			},
		},
	}

	result := Affected(affected, false)

	assert.Len(t, result, 1)
	assert.Equal(t, false, result[0]["enabled"])
	assert.Equal(t, false, result[0]["locked"])
}

func TestAffected_StatusIndicator_DefaultValues(t *testing.T) {
	// When no settings are provided, defaults to enabled=true, locked=false.
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			Stack:         "plat-ue2-dev",
			Affected:      "file",
		},
	}

	result := Affected(affected, false)

	assert.Len(t, result, 1)
	assert.Equal(t, true, result[0]["enabled"])
	assert.Equal(t, false, result[0]["locked"])
}

func TestAffected_StatusText_AllStates(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		locked         bool
		expectedStatus string
	}{
		{
			name:           "enabled and not locked returns enabled",
			enabled:        true,
			locked:         false,
			expectedStatus: "enabled",
		},
		{
			name:           "locked returns locked regardless of enabled",
			enabled:        true,
			locked:         true,
			expectedStatus: "locked",
		},
		{
			name:           "disabled returns disabled",
			enabled:        false,
			locked:         false,
			expectedStatus: "disabled",
		},
		{
			name:           "disabled and locked returns locked (locked takes precedence)",
			enabled:        false,
			locked:         true,
			expectedStatus: "locked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affected := []schema.Affected{
				{
					Component:     "vpc",
					ComponentType: "terraform",
					Stack:         "plat-ue2-dev",
					Affected:      "file",
					Settings: map[string]any{
						"metadata": map[string]any{
							"enabled": tt.enabled,
							"locked":  tt.locked,
						},
					},
				},
			}

			result := Affected(affected, false)

			assert.Len(t, result, 1)
			assert.Equal(t, tt.expectedStatus, result[0]["status_text"])
		})
	}
}

func TestAffected_StatusText_DefaultsToEnabled(t *testing.T) {
	// When no settings are provided, status_text should default to "enabled".
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			Stack:         "plat-ue2-dev",
			Affected:      "file",
		},
	}

	result := Affected(affected, false)

	assert.Len(t, result, 1)
	assert.Equal(t, "enabled", result[0]["status_text"])
}

func TestAffected_StatusText_WithDependents_Flattened(t *testing.T) {
	// Verify that dependents also get proper status_text.
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			Stack:         "plat-ue2-dev",
			Affected:      "file",
			Settings: map[string]any{
				"metadata": map[string]any{
					"enabled": true,
					"locked":  false,
				},
			},
			Dependents: []schema.Dependent{
				{
					Component:     "eks",
					ComponentType: "terraform",
					Stack:         "plat-ue2-dev",
				},
			},
		},
	}

	result := Affected(affected, true)

	assert.Len(t, result, 2)
	// Parent should have status_text "enabled".
	assert.Equal(t, "enabled", result[0]["status_text"])
	// Dependent should also have status_text "enabled" (default for dependents).
	assert.Equal(t, "enabled", result[1]["status_text"])
}

func TestAffected_AllFieldsPresent(t *testing.T) {
	// Verify that all expected fields are present in the result.
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			ComponentPath: "components/terraform/vpc",
			Stack:         "plat-ue2-dev",
			StackSlug:     "plat-ue2-dev-vpc",
			Affected:      "file",
			AffectedAll:   []string{"file", "config"},
			File:          "main.tf",
			Settings: map[string]any{
				"metadata": map[string]any{
					"enabled": true,
					"locked":  false,
				},
			},
		},
	}

	result := Affected(affected, false)

	assert.Len(t, result, 1)

	// Verify all expected fields are present.
	expectedFields := []string{
		"component", "component_type", "component_path", "stack", "stack_slug",
		"affected", "affected_all", "file", "enabled", "locked", "status", "status_text",
		"is_dependent", "depth", "dependents_count",
	}

	for _, field := range expectedFields {
		assert.Contains(t, result[0], field, "Missing field: %s", field)
	}
}
