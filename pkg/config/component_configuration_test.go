package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	stjsonschema "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	configschema "github.com/cloudposse/atmos/pkg/config/schema"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.Terraform{ConfigurationProviders: nil}

func TestComponentConfigurationOptIn(t *testing.T) {
	t.Parallel()
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(`components:
  terraform:
    command: tofu
    terragrunt:
      enabled: true
      mode: native
      discovery:
        enabled: true
        included_paths: [live/**]
        excluded_paths: [live/archive/**]
      download_dir: .cache
`)))
	var config schema.AtmosConfiguration
	require.NoError(t, v.Unmarshal(&config, atmosDecodeHook()))
	require.True(t, config.HasComponentDiscovery())
	require.Equal(t, "tofu", config.Components.Terraform.Command)
	provider := config.Components.Terraform.ConfigurationProviders["terragrunt"]
	require.Equal(t, []string{"live/**"}, provider.Discovery.IncludedPaths)
	require.Equal(t, []string{"live/archive/**"}, provider.Discovery.ExcludedPaths)
	require.Equal(t, map[string]any{"download_dir": ".cache"}, provider.Options)
	require.NoError(t, checkConfig(&config, true))
	provider.Discovery.Enabled = false
	config.Components.Terraform.ConfigurationProviders["terragrunt"] = provider
	require.False(t, config.HasComponentDiscovery())
	require.Error(t, checkConfig(&config, true))
}

func TestComponentDiscoveryRequiresConfiguredYAMLInventory(t *testing.T) {
	for _, base := range []string{"", "missing-stacks"} {
		t.Run("base="+base, func(t *testing.T) {
			root := t.TempDir()
			config := schema.AtmosConfiguration{BasePathAbsolute: root, StacksBaseAbsolutePath: filepath.Join(root, base), Components: schema.Components{Terraform: schema.Terraform{ConfigurationProviders: map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true, Discovery: schema.ComponentDiscoveryConfig{Enabled: true}}}}}}
			config.Stacks.BasePath = base
			pattern := filepath.Join(root, base, "**", "*.yaml")
			err := processStackConfigs(&config, &schema.ConfigAndStacksInfo{}, []string{pattern}, nil)
			if base == "" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errUtils.ErrNoStackManifestsFound)
			}
			config.Stacks.IncludedPaths = []string{"**/*.yaml"}
			require.ErrorIs(t, processStackConfigs(&config, &schema.ConfigAndStacksInfo{}, []string{pattern}, nil), errUtils.ErrNoStackManifestsFound)
			require.NoError(t, os.MkdirAll(config.StacksBaseAbsolutePath, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(config.StacksBaseAbsolutePath, "dev.yaml"), []byte("components: {}"), 0o644))
			require.NoError(t, processStackConfigs(&config, &schema.ConfigAndStacksInfo{}, []string{pattern}, nil))
			require.Equal(t, []string{filepath.Join(config.StacksBaseAbsolutePath, "dev.yaml")}, config.StackConfigFilesAbsolutePaths)
		})
	}
}

func TestComponentConfigurationIgnoresUnclaimedMappings(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(`components:
  terraform:
    inventory_metadata:
      discovery: external
    another_setting: value
    terragrunt:
      enabled: true
      discovery:
        enabled: true
    custom:
      provider: custom-plugin
      enabled: false
`)))
	var config schema.AtmosConfiguration
	require.NoError(t, v.Unmarshal(&config, atmosDecodeHook()))
	require.Len(t, config.Components.Terraform.ConfigurationProviders, 2)
	require.True(t, config.Components.Terraform.ConfigurationProviders["terragrunt"].Discovery.Enabled)
	require.Equal(t, "custom-plugin", config.Components.Terraform.ConfigurationProviders["custom"].Provider)
	require.Equal(t, "external", v.GetString("components.terraform.inventory_metadata.discovery"), "decode must not mutate source config")
	generated, err := configschema.Generate()
	require.NoError(t, err)
	compiler := stjsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("config.json", bytes.NewReader(generated)))
	compiled, err := compiler.Compile("config.json")
	require.NoError(t, err)
	require.NoError(t, compiled.Validate(v.AllSettings()), "unclaimed mappings retain the base schema compatibility")
	v.Set("components.terraform.broken", map[string]any{"enabled": true, "discovery": "external"})
	require.Error(t, v.Unmarshal(&config, atmosDecodeHook()), "claimed invalid provider still fails")
	require.Error(t, compiled.Validate(v.AllSettings()), "schema rejects the same claimed invalid provider")
}

// Compile-time guard for the recorded unknown keys.
var _ = schema.Terraform{UnknownSections: nil}

func TestRecordUnknownTerraformSections(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(`components:
  terraform:
    base_path: components/terraform
    plan_diff: {}
    flags: {}
    base_pth: typo
    team_notes:
      owner: platform
    team_defaults:
      enabled: true
    terragrunt:
      mode: native
    custom:
      provider: custom-plugin
`)))
	config := schema.AtmosConfiguration{}
	config.Components.Terraform.UnknownSections = []string{"stale"}
	recordUnknownTerraformSections(v, &config)
	// Marked mappings are provider candidates (validated later); the rest are recorded, sorted.
	require.Equal(t, []string{"base_pth", "team_notes", "terragrunt"}, config.Components.Terraform.UnknownSections)

	recordUnknownTerraformSections(viper.New(), &config)
	require.Nil(t, config.Components.Terraform.UnknownSections, "a project without components.terraform records nothing")
}

func TestTerraformKnownKeysComeFromTheSchema(t *testing.T) {
	known := terraformKnownKeys()
	for _, key := range []string{"base_path", "apply_auto_approve", "init", "plan_diff", "flags", "cache"} {
		require.True(t, known[key], key)
	}
	for _, key := range []string{"", "-", "base_pth"} {
		require.False(t, known[key], key)
	}
}
