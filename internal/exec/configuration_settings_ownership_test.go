package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestUserSettingsConfigurationIsNotProviderOwnership reproduces free-form user data under
// settings.configuration, which v1.234.1 passed through unchanged.
func TestUserSettingsConfigurationIsNotProviderOwnership(t *testing.T) {
	t.Run("notes only", func(t *testing.T) {
		_, info := configurationProject(t, "", `      settings:
        configuration:
          notes: consumed by internal tooling
      vars:
        a: 2
`)
		require.False(t, schema.ConfigurationOwned(info.ComponentSettingsSection))
		require.Equal(t, map[string]any{"notes": "consumed by internal tooling"}, info.ComponentSettingsSection["configuration"])
		require.Equal(t, "dev-app.terraform.tfvars.json", constructTerraformComponentVarfileName(&info))
		require.Equal(t, "dev-app.planfile", constructTerraformComponentPlanfileName(&info))
	})

	t.Run("provider key with a component backend", func(t *testing.T) {
		_, info := configurationProject(t, "", `      settings:
        configuration:
          provider: aws
          notes: team-owned settings consumed by internal tooling
      backend_type: s3
      backend:
        s3:
          key: app.tfstate
      vars:
        a: 1
`)
		require.False(t, schema.ConfigurationOwned(info.ComponentSettingsSection))
		require.Equal(t, map[string]any{"provider": "aws", "notes": "team-owned settings consumed by internal tooling"}, info.ComponentSettingsSection["configuration"])
		require.Equal(t, "app.tfstate", info.ComponentBackendSection["key"])
		require.Equal(t, "dev-app.terraform.tfvars.json", constructTerraformComponentVarfileName(&info))
	})
}

// TestUserSettingsConfigurationCannotClaimOwnership verifies that a manifest cannot mark its own
// component provider-owned by writing the reserved key: stack processing removes it.
func TestUserSettingsConfigurationCannotClaimOwnership(t *testing.T) {
	_, info := configurationProject(t, "", `      settings:
        configuration:
          provider: terragrunt
          `+schema.ConfigurationOwnedKey+`: true
      vars:
        a: 1
`)
	require.False(t, schema.ConfigurationOwned(info.ComponentSettingsSection))
	require.Equal(t, map[string]any{"provider": "terragrunt"}, info.ComponentSettingsSection["configuration"])
}

// TestDiscoveredUnitsRemainProviderOwned is the negative path: a unit that an enabled provider
// discovered is still provider-owned after stack processing, while an ordinary component beside it
// keeps its own settings.configuration data unowned.
func TestDiscoveredUnitsRemainProviderOwned(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, StacksBaseAbsolutePath: filepath.Join(root, "stacks"), TerraformDirAbsolutePath: filepath.Join(root, "components")}
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true, Discovery: schema.ComponentDiscoveryConfig{Enabled: true}}}
	provider := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config.ComponentConfigurations = provider
	unit := map[string]any{"provider": "terragrunt", "engine": "opentofu", "source": "live/app/terragrunt.hcl"}
	definitions := map[string]map[string]any{"live": {"name": "live", "components": map[string]any{"terraform": map[string]any{
		"app":   map[string]any{"settings": map[string]any{"configuration": unit}},
		"other": map[string]any{"settings": map[string]any{"configuration": map[string]any{"provider": "aws"}}},
	}}}}
	provider.EXPECT().Discover(gomock.Any()).Return(definitions, nil)
	stacks := map[string]any{}
	require.NoError(t, mergeConfigurationStacks(config, stacks, map[string]map[string]any{}, AllStacksDeferredContexts{}))
	components := stacks["live"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)
	app := components["app"].(map[string]any)["settings"].(map[string]any)
	require.True(t, schema.ConfigurationOwned(app))
	require.Equal(t, "live/app/terragrunt.hcl", app["configuration"].(map[string]any)["source"])
	require.NotContains(t, unit, schema.ConfigurationOwnedKey, "the provider's definition must not change")
	require.False(t, schema.ConfigurationOwned(components["other"].(map[string]any)["settings"].(map[string]any)))
}
