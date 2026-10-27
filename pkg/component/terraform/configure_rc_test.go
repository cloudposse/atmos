package terraform

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
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

// TestAssembleComponentEnvVarsRejectsInvalidRC verifies malformed CLI configuration stops environment preparation.
func TestAssembleComponentEnvVarsRejectsInvalidRC(t *testing.T) {
	clearUserCLIConfigEnv(t)
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	config.Components.Terraform.RC = &schema.TerraformRCConfig{
		Enabled: true,
		Config:  map[string]any{"provider_installation": "invalid-method-list"},
	}
	info := &schema.ConfigAndStacksInfo{}
	err := assembleComponentEnvVars(config, info, nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	assert.Nil(t, info.RCCleanup)
	for _, env := range info.ComponentEnvList {
		assert.NotContains(t, env, rc.EnvKeyTFCLIConfigFile+"=", "invalid configuration must not be exported to Terraform")
	}
}

// TestConfigureTerraformRCIncludesRegistryCache verifies the generated CLI configuration routes providers and modules through the registry proxy.
func TestConfigureTerraformRCIncludesRegistryCache(t *testing.T) {
	clearUserCLIConfigEnv(t)
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.Cache = &schema.TerraformCacheConfig{Enabled: true, Location: t.TempDir()}
	setup, err := tfcache.Start(t.Context(), config)
	require.NoError(t, err)
	require.NotNil(t, setup)
	t.Cleanup(func() { require.NoError(t, setup.Close(context.Background())) })
	info := &schema.ConfigAndStacksInfo{TerraformCache: setup}
	require.NoError(t, configureTerraformRC(config, info))
	require.NotNil(t, info.RCCleanup)
	t.Cleanup(func() { require.NoError(t, info.RCCleanup()) })
	var configPath string
	for _, env := range info.ComponentEnvList {
		if path, ok := strings.CutPrefix(env, rc.EnvKeyTFCLIConfigFile+"="); ok {
			configPath = path
		}
	}
	require.NotEmpty(t, configPath)
	contents, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "network_mirror")
	assert.Contains(t, string(contents), "https://127.0.0.1:")
	assert.Contains(t, string(contents), `host "registry.terraform.io"`)
	assert.Contains(t, string(contents), `host "registry.opentofu.org"`)
	assert.Contains(t, info.ComponentEnvList, rc.EnvKeyTofuCLIConfigFile+"="+configPath)
}

// TestAppendTerraformSecretEnv verifies secret encoding failures propagate and test secrets take precedence over component values.
func TestAppendTerraformSecretEnv(t *testing.T) {
	secret := "environment-fixture-sensitive-value"
	atmosio.RegisterSecret(secret)
	for _, tc := range []struct {
		name    string
		info    schema.ConfigAndStacksInfo
		want    []string
		invalid bool
	}{
		{name: "component encoding error", info: schema.ConfigAndStacksInfo{
			TerraformSecretVarKeys: map[string]bool{"token": true}, ComponentVarsSection: map[string]any{"token": make(chan int)},
		}, invalid: true},
		{name: "test encoding error", info: schema.ConfigAndStacksInfo{
			SubCommand: "test", ComponentSection: map[string]any{"test": map[string]any{"vars": map[string]any{"credentials": map[string]any{"token": secret, "invalid": make(chan int)}}}},
		}, invalid: true},
		{name: "test secrets override component variables", info: schema.ConfigAndStacksInfo{
			SubCommand: "test", ComponentEnvList: []string{"TF_VAR_token=component-value"},
			ComponentSection: map[string]any{"test": map[string]any{"vars": map[string]any{"token": secret, "public": "unclassified"}}},
		}, want: []string{"TF_VAR_token=component-value", "TF_VAR_token=" + secret}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := appendTerraformSecretEnv(&tc.info)
			if tc.invalid {
				var encodingErr *json.UnsupportedTypeError
				require.ErrorAs(t, err, &encodingErr)
				assert.Empty(t, tc.info.ComponentEnvList, "invalid secrets must not yield partial environment output")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, tc.info.ComponentEnvList)
		})
	}
}
