package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestProcessEnvVarsWrapsInvalidBooleanOverrides(t *testing.T) {
	names := []string{
		"ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE",
		"ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT",
		"ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE",
		"ATMOS_COMPONENTS_TERRAFORM_INIT_PASS_VARS",
		"ATMOS_COMPONENTS_TERRAFORM_PLAN_SKIP_PLANFILE",
		"ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE",
		"ATMOS_COMPONENTS_HELMFILE_USE_EKS",
	}

	for _, name := range names {
		t.Run(name+"/invalid", func(t *testing.T) {
			t.Setenv(name, "not-a-bool")
			err := processEnvVars(&schema.AtmosConfiguration{})
			require.Error(t, err)
			require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
			require.ErrorIs(t, err, strconv.ErrSyntax, "the underlying parser error must be preserved")
			require.ErrorContains(t, err, name)
		})

		t.Run(name+"/valid", func(t *testing.T) {
			t.Setenv(name, "true")
			require.NoError(t, processEnvVars(&schema.AtmosConfiguration{}))
		})
	}
}

func TestStackConfigSentinelsAreSharedErrors(t *testing.T) {
	err := checkConfig(&schema.AtmosConfiguration{}, true)
	require.ErrorIs(t, err, errUtils.ErrStackBasePathMissing)

	err = checkConfig(&schema.AtmosConfiguration{Stacks: schema.Stacks{BasePath: "stacks"}}, true)
	require.ErrorIs(t, err, errUtils.ErrStackIncludedPathsMissing)
	require.NotErrorIs(t, err, errUtils.ErrStackBasePathMissing)

	missing := filepath.Join(t.TempDir(), "absent", "atmos.yaml")
	_, err = SearchConfigFile(missing, &schema.AtmosConfiguration{})
	require.Error(t, err)
	require.True(t, errors.Is(err, errUtils.ErrSpecifiedConfigNotFound) || errors.Is(err, errUtils.ErrImportMatchNotFound),
		"a missing explicit config path must surface a shared sentinel, got %v", err)
}
