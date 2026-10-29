package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func registryIdentityProject(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	config := pluginCoverageProject(t)
	t.Setenv("ATMOS_TEST_REGISTRY_PASSWORD", "fixture-registry-secret")
	config.Auth.Identities = map[string]schema.Identity{
		"registry-reader": {Kind: "oci/user", Spec: map[string]any{
			"registry": "registry.example.com", "username": "robot", "password_env": "ATMOS_TEST_REGISTRY_PASSWORD",
		}},
	}
	writePluginCoverageConfig(t, config)
	return config
}

func TestPluginRegistryAddHonorsTheGlobalIdentityShorthandAndEnvironment(t *testing.T) {
	t.Run("shorthand", func(t *testing.T) {
		config := registryIdentityProject(t)
		// The root command folds a space-separated "-i value" into "-i=value" before parsing.
		_, err := invokePluginCoverage(t, config, "registry", "add", "corporate", "--host=registry.example.com", "-i=registry-reader")
		require.NoError(t, err)
		reloadPluginCoverageConfig(t, config)
		assert.Equal(t, "registry-reader", config.Plugins.Registries["corporate"].Identity)
	})
	t.Run("environment", func(t *testing.T) {
		config := registryIdentityProject(t)
		t.Setenv("ATMOS_IDENTITY", "registry-reader")
		_, err := invokePluginCoverage(t, config, "registry", "add", "corporate", "--host=registry.example.com")
		require.NoError(t, err)
		reloadPluginCoverageConfig(t, config)
		assert.Equal(t, "registry-reader", config.Plugins.Registries["corporate"].Identity)
	})
	t.Run("long flag still wins over the environment", func(t *testing.T) {
		config := registryIdentityProject(t)
		t.Setenv("ATMOS_IDENTITY", "does-not-exist")
		_, err := invokePluginCoverage(t, config, "registry", "add", "corporate", "--host=registry.example.com", "--identity=registry-reader")
		require.NoError(t, err)
		reloadPluginCoverageConfig(t, config)
		assert.Equal(t, "registry-reader", config.Plugins.Registries["corporate"].Identity)
	})
	t.Run("a bare flag asking for interactive selection is explained", func(t *testing.T) {
		config := registryIdentityProject(t)
		_, err := invokePluginCoverage(t, config, "registry", "add", "corporate", "--host=registry.example.com", "--identity")
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		assert.Contains(t, strings.Join(errUtils.AllHints(err), " "), "--identity=<name>")
	})
}

func TestPluginRegistryAddWarnsWhenReplacingAnExistingAlias(t *testing.T) {
	config := pluginCoverageProject(t)
	output := capturePluginUI(t)
	_, err := invokePluginCoverage(t, config, "registry", "add", "corporate", "oci://registry.example.com/first/catalog:stable")
	require.NoError(t, err)
	assert.NotContains(t, ansi.Strip(output.String()), "Replacing", "a new alias replaces nothing")
	reloadPluginCoverageConfig(t, config)

	output.Reset()
	_, err = invokePluginCoverage(t, config, "registry", "add", "corporate", "oci://registry.example.com/second/catalog:stable")
	require.NoError(t, err)
	text := ansi.Strip(output.String())
	assert.Contains(t, text, "⚠")
	assert.Contains(t, text, "corporate")
	assert.Contains(t, text, "oci://registry.example.com/first/catalog:stable", "the replaced catalog is named")
	assert.Contains(t, text, "Added plugin registry")
}

func TestPluginRegistryRemoveAndLogoutExplainUnknownAliases(t *testing.T) {
	config := pluginCoverageProject(t)
	config.Plugins.Registries = map[string]schema.PluginRegistry{"corporate": {Catalog: "oci://registry.example.com/catalog:stable"}}
	writePluginCoverageConfig(t, config)

	_, err := invokePluginCoverage(t, config, "registry", "remove", "corporat")
	require.ErrorIs(t, err, errUtils.ErrPluginRegistryNotFound)
	assert.Contains(t, formatPluginError(err), "corporate")

	_, err = invokePluginCoverage(t, config, "registry", "logout", "corporat")
	require.ErrorIs(t, err, errUtils.ErrPluginRegistryNotFound)
	assert.Contains(t, formatPluginError(err), "corporate")
}

func TestPluginRegistryLoginRefusesKeyringsThatCannotPersist(t *testing.T) {
	config := pluginCoverageProject(t)
	config.Auth.Keyring = schema.KeyringConfig{Type: "memory"}
	writePluginCoverageConfig(t, config)
	t.Setenv("ATMOS_KEYRING_TYPE", "memory") // The test project pins a file keyring through the environment.
	output := capturePluginUI(t)

	_, err := invokePluginCoverage(t, config, "registry", "login", "registry.example.com", "--username=robot", "--password-stdin")
	require.ErrorIs(t, err, errUtils.ErrPluginCredentialStore)
	rendered := formatPluginError(err)
	assert.Contains(t, rendered, "auth.keyring.type")
	assert.Contains(t, rendered, "password_env")
	assert.NotContains(t, ansi.Strip(output.String()), "Stored registry credentials", "no false success line")
}

func TestPluginRegistryLoginWithoutIdentityOrFlagsExplainsTheOptions(t *testing.T) {
	config := pluginCoverageProject(t)
	_, err := invokePluginCoverage(t, config, "registry", "login", "registry.example.com")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Contains(t, formatPluginError(err), "--password-stdin")
}
