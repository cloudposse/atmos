package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConfigEnvValueIsEnvironmentOnly(t *testing.T) {
	const name = "ATMOS_TEST_CONFIG_ENV_BINDING"
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetDefault(name, "default")
	viper.Set(name, "global override")
	t.Setenv(name, "")
	require.NoError(t, os.Unsetenv(name))
	require.Empty(t, configEnvValue(name), "global config must not supply an absent environment override")

	for _, value := range []string{"", "MixedCase", "  keep whitespace  ", "false", "0"} {
		t.Setenv(name, value)
		require.Equal(t, value, configEnvValue(name))
	}
}

func TestProcessEnvVarsStopsAtInvalidOverride(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "environment-base")
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE", "invalid")
	t.Setenv("ATMOS_COMPONENTS_HELMFILE_COMMAND", "later-override")
	config := schema.AtmosConfiguration{}
	config.Components.Helmfile.Command = "original"
	err := processEnvVars(&config)
	require.Error(t, err)
	require.Equal(t, "environment-base", config.BasePath)
	require.Equal(t, basePathSourceRuntime, config.BasePathSource)
	require.Equal(t, "original", config.Components.Helmfile.Command, "later overrides must not run after invalid input")
}
