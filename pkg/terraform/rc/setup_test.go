package rc

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func enabledConfig() *schema.AtmosConfiguration {
	cfg := &schema.AtmosConfiguration{}
	cfg.Components.Terraform.RC = &schema.TerraformRCConfig{
		Enabled: true,
		Config: map[string]any{
			"provider_installation": []any{
				map[string]any{"network_mirror": map[string]any{"url": "http://127.0.0.1:5000/"}},
				map[string]any{"direct": nil},
			},
		},
	}
	return cfg
}

func TestSetup_DisabledReturnsNoop(t *testing.T) {
	cfg := &schema.AtmosConfiguration{} // RC == nil.
	env, closer, err := Setup(cfg, &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Empty(t, env)
	require.NotNil(t, closer)
	assert.NoError(t, closer())

	cfg.Components.Terraform.RC = &schema.TerraformRCConfig{Enabled: false}
	env, _, err = Setup(cfg, &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Empty(t, env)
}

func TestSetup_EnabledWritesAndCleansUp(t *testing.T) {
	cfg := enabledConfig()
	env, closer, err := Setup(cfg, &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	// Both Terraform and OpenTofu CLI config env vars point at the same file.
	envMap := map[string]string{}
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		envMap[k] = v
	}
	tfPath := envMap[EnvKeyTFCLIConfigFile]
	tofuPath := envMap[EnvKeyTofuCLIConfigFile]
	require.NotEmpty(t, tfPath)
	require.Equal(t, tfPath, tofuPath, "TF and TOFU CLI config vars must point at the same file")

	info, statErr := os.Stat(tfPath)
	require.NoError(t, statErr)
	assert.False(t, info.IsDir())

	content, readErr := os.ReadFile(tfPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "network_mirror {")

	// Closer removes the file and is idempotent.
	require.NoError(t, closer())
	_, statErr = os.Stat(tfPath)
	assert.True(t, os.IsNotExist(statErr))
	assert.NoError(t, closer(), "closer must be idempotent")
}

func TestSetup_DefersToUserTofuCLIConfig(t *testing.T) {
	t.Setenv(EnvKeyTofuCLIConfigFile, "/user/managed.tofurc")
	cfg := enabledConfig()
	env, _, err := Setup(cfg, &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Empty(t, env, "Atmos must defer when the user sets TOFU_CLI_CONFIG_FILE")
}

func TestSetup_DefersToUserOSEnv(t *testing.T) {
	t.Setenv(EnvKeyTFCLIConfigFile, "/user/managed.tfrc")
	cfg := enabledConfig()
	env, closer, err := Setup(cfg, &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Empty(t, env)
	assert.NoError(t, closer())
}

func TestSetup_DefersToUserAtmosEnv(t *testing.T) {
	cfg := enabledConfig()
	cfg.Env = map[string]string{EnvKeyTFCLIConfigFile: "/user/managed.tfrc"}
	env, _, err := Setup(cfg, &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Empty(t, env)
}

func TestSetup_DefersToUserComponentEnv(t *testing.T) {
	cfg := enabledConfig()
	info := &schema.ConfigAndStacksInfo{
		ComponentEnvSection: schema.AtmosSectionMapType{EnvKeyTFCLIConfigFile: "/user/managed.tfrc"},
	}
	env, _, err := Setup(cfg, info)
	require.NoError(t, err)
	assert.Empty(t, env)
}
