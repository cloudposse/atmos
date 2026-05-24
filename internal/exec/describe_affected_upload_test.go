package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStripAffectedForUpload_PreservesRequiredFields(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:            "vpc",
			Stack:                "plat-use2-dev",
			IncludedInDependents: false,
			Settings: schema.AtmosSectionMapType{
				"pro": map[string]interface{}{
					"enabled": true,
				},
			},
		},
	}

	result := StripAffectedForUpload(affected)

	assert.Len(t, result, 1)
	assert.Equal(t, "vpc", result[0].Component)
	assert.Equal(t, "plat-use2-dev", result[0].Stack)
	assert.Equal(t, false, result[0].IncludedInDependents)
	assert.NotNil(t, result[0].Settings)
	assert.NotNil(t, result[0].Settings["pro"])
}

func TestStripAffectedForUpload_RemovesUnusedFields(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:     "vpc",
			ComponentType: "terraform",
			ComponentPath: "components/terraform/vpc",
			Namespace:     "ex1",
			Tenant:        "plat",
			Environment:   "use2",
			Stage:         "dev",
			Stack:         "plat-use2-dev",
			StackSlug:     "plat-use2-dev-vpc",
			Affected:      "stack.vars",
			Settings: schema.AtmosSectionMapType{
				"depends_on": map[string]interface{}{
					"1": map[string]interface{}{"component": "network"},
				},
				"github": map[string]interface{}{
					"actions_enabled": true,
				},
				"pro": map[string]interface{}{
					"enabled": true,
				},
			},
		},
	}

	result := StripAffectedForUpload(affected)

	assert.Len(t, result, 1)
	// Verify removed fields are empty/zero
	assert.Empty(t, result[0].ComponentType)
	assert.Empty(t, result[0].ComponentPath)
	assert.Empty(t, result[0].Namespace)
	assert.Empty(t, result[0].Tenant)
	assert.Empty(t, result[0].Environment)
	assert.Empty(t, result[0].Stage)
	assert.Empty(t, result[0].StackSlug)
	assert.Empty(t, result[0].Affected)

	// Verify settings only contains pro
	assert.Nil(t, result[0].Settings["depends_on"])
	assert.Nil(t, result[0].Settings["github"])
	assert.NotNil(t, result[0].Settings["pro"])
}

func TestStripAffectedForUpload_RecursiveDependents(t *testing.T) {
	affected := []schema.Affected{
		{
			Component: "vpc",
			Stack:     "plat-use2-dev",
			Dependents: []schema.Dependent{
				{
					Component:     "database",
					ComponentType: "terraform",
					Stack:         "plat-use2-dev",
					Dependents: []schema.Dependent{
						{
							Component:     "api",
							ComponentType: "terraform",
							Stack:         "plat-use2-dev",
							Settings: schema.AtmosSectionMapType{
								"depends_on": map[string]interface{}{
									"1": map[string]interface{}{"component": "database"},
								},
								"pro": map[string]interface{}{
									"enabled": true,
								},
							},
						},
					},
					Settings: schema.AtmosSectionMapType{
						"depends_on": map[string]interface{}{
							"1": map[string]interface{}{"component": "vpc"},
						},
						"pro": map[string]interface{}{
							"enabled": true,
						},
					},
				},
			},
		},
	}

	result := StripAffectedForUpload(affected)

	// Check first level dependent
	assert.Len(t, result[0].Dependents, 1)
	assert.Equal(t, "database", result[0].Dependents[0].Component)
	assert.Empty(t, result[0].Dependents[0].ComponentType)
	assert.Nil(t, result[0].Dependents[0].Settings["depends_on"])
	assert.NotNil(t, result[0].Dependents[0].Settings["pro"])

	// Check nested dependent
	assert.Len(t, result[0].Dependents[0].Dependents, 1)
	assert.Equal(t, "api", result[0].Dependents[0].Dependents[0].Component)
	assert.Empty(t, result[0].Dependents[0].Dependents[0].ComponentType)
	assert.Nil(t, result[0].Dependents[0].Dependents[0].Settings["depends_on"])
	assert.NotNil(t, result[0].Dependents[0].Dependents[0].Settings["pro"])
}

func TestStripAffectedForUpload_EmptyDependents(t *testing.T) {
	affected := []schema.Affected{
		{
			Component:  "vpc",
			Stack:      "plat-use2-dev",
			Dependents: []schema.Dependent{},
		},
	}

	result := StripAffectedForUpload(affected)

	assert.Len(t, result, 1)
	assert.NotNil(t, result[0].Dependents)
	assert.Len(t, result[0].Dependents, 0)
}

func TestStripAffectedForUpload_NilSettings(t *testing.T) {
	affected := []schema.Affected{
		{
			Component: "vpc",
			Stack:     "plat-use2-dev",
			Settings:  nil,
		},
	}

	result := StripAffectedForUpload(affected)

	assert.Len(t, result, 1)
	assert.Nil(t, result[0].Settings)
}

func TestStripAffectedForUpload_SettingsWithoutPro(t *testing.T) {
	affected := []schema.Affected{
		{
			Component: "vpc",
			Stack:     "plat-use2-dev",
			Settings: schema.AtmosSectionMapType{
				"depends_on": map[string]interface{}{
					"1": map[string]interface{}{"component": "network"},
				},
				"github": map[string]interface{}{
					"actions_enabled": true,
				},
			},
		},
	}

	result := StripAffectedForUpload(affected)

	assert.Len(t, result, 1)
	// Settings should be nil when there's no pro section
	assert.Nil(t, result[0].Settings)
}

// TestStripAffectedForUpload_PreservesProEventSchema locks in the contract that
// stripSettings preserves the full settings.pro sub-tree opaquely. This is the
// only thing keeping settings.pro.merge_group.checks_requested.workflows alive
// from user YAML to the upload payload, and a future struct-tightening of
// Affected.Settings could silently drop it. Assert the per-event schema we
// support today: pull_request, release, drift_detection, and merge_group.
func TestStripAffectedForUpload_PreservesProEventSchema(t *testing.T) {
	planWorkflows := map[string]interface{}{
		"atmos-terraform-plan.yaml": map[string]interface{}{
			"inputs": map[string]interface{}{
				"component": "{{ .atmos_component }}",
				"stack":     "{{ .atmos_stack }}",
			},
		},
	}
	applyWorkflows := map[string]interface{}{
		"atmos-terraform-apply.yaml": map[string]interface{}{
			"inputs": map[string]interface{}{
				"component": "{{ .atmos_component }}",
				"stack":     "{{ .atmos_stack }}",
			},
		},
	}

	affected := []schema.Affected{
		{
			Component: "vpc",
			Stack:     "plat-use2-dev",
			Settings: schema.AtmosSectionMapType{
				"pro": map[string]interface{}{
					"enabled": true,
					"pull_request": map[string]interface{}{
						"opened":      map[string]interface{}{"workflows": planWorkflows},
						"synchronize": map[string]interface{}{"workflows": planWorkflows},
						"reopened":    map[string]interface{}{"workflows": planWorkflows},
						"merged":      map[string]interface{}{"workflows": applyWorkflows},
					},
					"release": map[string]interface{}{
						"published": map[string]interface{}{"workflows": applyWorkflows},
					},
					"drift_detection": map[string]interface{}{
						"enabled": true,
					},
					"merge_group": map[string]interface{}{
						"checks_requested": map[string]interface{}{"workflows": planWorkflows},
					},
				},
			},
		},
	}

	result := StripAffectedForUpload(affected)

	require.Len(t, result, 1)
	pro, ok := result[0].Settings["pro"].(map[string]interface{})
	require.True(t, ok, "settings.pro must survive stripping as map[string]interface{}")

	// Each per-event block must round-trip verbatim.
	assert.Equal(t, true, pro["enabled"])
	assert.NotNil(t, pro["pull_request"], "settings.pro.pull_request must round-trip")
	assert.NotNil(t, pro["release"], "settings.pro.release must round-trip")
	assert.NotNil(t, pro["drift_detection"], "settings.pro.drift_detection must round-trip")
	assert.NotNil(t, pro["merge_group"], "settings.pro.merge_group must round-trip — required for GitHub merge-queue support")

	// Drill into merge_group to make sure the nested workflows survive too.
	mg, ok := pro["merge_group"].(map[string]interface{})
	require.True(t, ok)
	cr, ok := mg["checks_requested"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, planWorkflows, cr["workflows"])
}

func TestStripAffectedForUpload_EmptyInput(t *testing.T) {
	affected := []schema.Affected{}

	result := StripAffectedForUpload(affected)

	assert.NotNil(t, result)
	assert.Len(t, result, 0)
}
