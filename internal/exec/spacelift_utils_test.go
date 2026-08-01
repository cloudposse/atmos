package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveSpaceliftContextPrefix(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	context := schema.Context{Tenant: "tenant1", Environment: "ue2", Stage: "dev"}
	componentVars := map[string]any{"tenant": "tenant1", "environment": "ue2", "stage": "dev"}

	tests := []struct {
		name          string
		nameTemplate  string
		namePattern   string
		expectedStack string
	}{
		{
			name:          "name_template takes precedence",
			nameTemplate:  "{{.vars.tenant}}-{{.vars.environment}}-{{.vars.stage}}",
			namePattern:   "{tenant}-{environment}-{stage}",
			expectedStack: "tenant1-ue2-dev",
		},
		{
			name:          "deprecated name_pattern still works when name_template is not set",
			namePattern:   "{tenant}-{environment}-{stage}",
			expectedStack: "tenant1-ue2-dev",
		},
		{
			name:          "neither configured falls back to the raw stack name",
			expectedStack: "orgs-cp-tenant1-dev-us-east-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			naming := SpaceliftStackNaming{NameTemplate: tt.nameTemplate, NamePattern: tt.namePattern}
			result, err := ResolveSpaceliftContextPrefix(atmosConfig, "orgs/cp/tenant1/dev/us-east-2", &context, componentVars, naming)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStack, result)
		})
	}
}

func TestBuildSpaceliftStackNames(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stacks := map[string]any{
		"orgs/cp/tenant1/dev/us-east-2": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"infra-vpc": map[string]any{
						"vars": map[string]any{
							"tenant":      "tenant1",
							"environment": "ue2",
							"stage":       "dev",
						},
						"settings": map[string]any{},
					},
				},
			},
		},
	}

	t.Run("name_template resolves logical stack names", func(t *testing.T) {
		naming := SpaceliftStackNaming{NameTemplate: "{{.vars.tenant}}-{{.vars.environment}}-{{.vars.stage}}"}
		names, err := BuildSpaceliftStackNames(atmosConfig, stacks, naming)
		require.NoError(t, err)
		assert.Equal(t, []string{"tenant1-ue2-dev-infra-vpc"}, names)
	})

	t.Run("deprecated name_pattern still resolves logical stack names", func(t *testing.T) {
		naming := SpaceliftStackNaming{NamePattern: "{tenant}-{environment}-{stage}"}
		names, err := BuildSpaceliftStackNames(atmosConfig, stacks, naming)
		require.NoError(t, err)
		assert.Equal(t, []string{"tenant1-ue2-dev-infra-vpc"}, names)
	})

	t.Run("neither configured falls back to raw stack name", func(t *testing.T) {
		names, err := BuildSpaceliftStackNames(atmosConfig, stacks, SpaceliftStackNaming{})
		require.NoError(t, err)
		assert.Equal(t, []string{"orgs-cp-tenant1-dev-us-east-2-infra-vpc"}, names)
	})
}

func TestBuildSpaceliftStackNames_ComponentLevelSpaceliftSettingsOverride(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stacks := map[string]any{
		"orgs/cp/tenant1/dev/us-east-2": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"infra-vpc": map[string]any{
						"vars": map[string]any{
							"tenant":      "tenant1",
							"environment": "ue2",
							"stage":       "dev",
						},
						"settings": map[string]any{
							"spacelift": map[string]any{
								"stack_name": "custom-explicit-stack-name",
							},
						},
					},
				},
			},
		},
	}

	names, err := BuildSpaceliftStackNames(atmosConfig, stacks, SpaceliftStackNaming{NamePattern: "{tenant}-{environment}-{stage}"})
	require.NoError(t, err)
	// A component-level `settings.spacelift.stack_name` overrides the computed context
	// prefix entirely. This confirms buildSpaceliftStackNameForComponent actually reads
	// componentMap["settings"]["spacelift"] instead of always falling back to the
	// naming-derived default.
	assert.Equal(t, []string{"custom-explicit-stack-name"}, names)
}

func TestBuildSpaceliftStackNames_NameTemplateErrorPropagates(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stacks := map[string]any{
		"orgs/cp/tenant1/dev/us-east-2": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"infra-vpc": map[string]any{
						"vars": map[string]any{
							"tenant": "tenant1",
						},
						"settings": map[string]any{},
					},
				},
			},
		},
	}

	naming := SpaceliftStackNaming{NameTemplate: "{{.vars.environment}}"}
	names, err := BuildSpaceliftStackNames(atmosConfig, stacks, naming)
	// The component's `vars` doesn't define `environment`, so the template execution
	// must fail loudly (missingkey=error) instead of silently producing a wrong or
	// empty stack name, and BuildSpaceliftStackNames must forward that error rather
	// than swallowing it.
	require.Error(t, err)
	assert.Nil(t, names)
	assert.Contains(t, err.Error(), "environment")
}

func TestBuildSpaceliftStackNames_SkipsStacksWithoutTerraformComponents(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stacks := map[string]any{
		"stack-without-components": map[string]any{},
		"stack-without-terraform": map[string]any{
			"components": map[string]any{
				"helmfile": map[string]any{},
			},
		},
		"stack-with-terraform": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"infra-vpc": map[string]any{
						"vars": map[string]any{
							"tenant":      "tenant1",
							"environment": "ue2",
							"stage":       "dev",
						},
						"settings": map[string]any{},
					},
				},
			},
		},
	}

	names, err := BuildSpaceliftStackNames(atmosConfig, stacks, SpaceliftStackNaming{})
	require.NoError(t, err)
	// Stacks missing a `components` section, or missing a `components.terraform`
	// section (e.g. helmfile-only stacks), are silently skipped rather than causing a
	// type-assertion panic or contributing a spurious name.
	assert.Equal(t, []string{"stack-with-terraform-infra-vpc"}, names)
}
