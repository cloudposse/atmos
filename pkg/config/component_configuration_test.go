package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

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
	require.NoError(t, checkConfig(config, true))
	provider.Discovery.Enabled = false
	config.Components.Terraform.ConfigurationProviders["terragrunt"] = provider
	require.False(t, config.HasComponentDiscovery())
	require.Error(t, checkConfig(config, true))
}
