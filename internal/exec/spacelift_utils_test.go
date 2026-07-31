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
