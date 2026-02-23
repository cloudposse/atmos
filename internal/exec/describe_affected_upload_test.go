package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

func TestStripAffectedForUpload_EmptyInput(t *testing.T) {
	affected := []schema.Affected{}

	result := StripAffectedForUpload(affected)

	assert.NotNil(t, result)
	assert.Len(t, result, 0)
}
