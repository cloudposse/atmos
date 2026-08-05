package exec

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/rc"
)

// clearUserCLIConfigEnv removes the env vars that would make rc.Generate defer to a
// user-managed CLI config, so configureTerraformRC renders its own file.
func clearUserCLIConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{rc.EnvKeyTFCLIConfigFile, rc.EnvKeyTofuCLIConfigFile, rc.EnvKeyLegacyTerraformConfig} {
		if _, ok := os.LookupEnv(key); ok {
			t.Setenv(key, "")
		}
	}
}

func TestConfigureTerraformRC_NoConfigIsNoop(t *testing.T) {
	clearUserCLIConfigEnv(t)
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}

	require.NoError(t, configureTerraformRC(atmosConfig, info))
	assert.Empty(t, info.ComponentEnvList, "an empty RC map must not contribute env vars")
	assert.Nil(t, info.RCCleanup)
}

func TestConfigureTerraformRC_RendersAndExposesConfigFile(t *testing.T) {
	clearUserCLIConfigEnv(t)

	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.RC = &schema.TerraformRCConfig{
		Enabled: true,
		Config: map[string]any{
			"plugin_cache_dir": "/tmp/plugin-cache",
		},
	}
	info := &schema.ConfigAndStacksInfo{}

	require.NoError(t, configureTerraformRC(atmosConfig, info))
	require.NotNil(t, info.RCCleanup)
	t.Cleanup(func() { _ = info.RCCleanup() })

	// Both TF_CLI_CONFIG_FILE and TOFU_CLI_CONFIG_FILE point at the rendered file.
	var rcFile string
	for _, env := range info.ComponentEnvList {
		if strings.HasPrefix(env, rc.EnvKeyTFCLIConfigFile+"=") {
			rcFile = strings.TrimPrefix(env, rc.EnvKeyTFCLIConfigFile+"=")
		}
	}
	require.NotEmpty(t, rcFile, "TF_CLI_CONFIG_FILE must be exported")

	contents, err := os.ReadFile(rcFile)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "plugin_cache_dir")

	// The cleanup closer removes the rendered file.
	require.NoError(t, info.RCCleanup())
	_, statErr := os.Stat(rcFile)
	assert.True(t, os.IsNotExist(statErr), "RCCleanup must remove the rendered CLI config file")
}
