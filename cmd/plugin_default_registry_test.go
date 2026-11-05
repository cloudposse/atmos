package cmd

import (
	"encoding/json"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginconfig "github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPluginDefaultRegistryListRemoveAndReadd(t *testing.T) {
	config := pluginCoverageProject(t)
	config.Plugins.Discovery.DefaultRegistry = nil
	writePluginCoverageConfig(t, config)
	list := func() map[string]schema.PluginRegistry {
		t.Helper()
		output, err := invokePluginCoverage(t, config, "registry", "list", "--format=json")
		require.NoError(t, err)
		var entries map[string]schema.PluginRegistry
		require.NoError(t, json.Unmarshal([]byte(output), &entries))
		return entries
	}
	require.Equal(t, map[string]schema.PluginRegistry{"cloudposse": {Catalog: pluginconfig.DefaultCatalog}}, list())
	_, err := invokePluginCoverage(t, config, "registry", "remove", "cloudposse")
	require.NoError(t, err)
	reloadPluginCoverageConfig(t, config)
	require.Empty(t, list())
	_, err = invokePluginCoverage(t, config, "registry", "remove", "cloudposse")
	require.ErrorIs(t, err, errUtils.ErrPluginRegistryNotFound)
	_, err = invokePluginCoverage(t, config, "registry", "add", "cloudposse", pluginconfig.DefaultCatalog)
	require.NoError(t, err)
	reloadPluginCoverageConfig(t, config)
	require.Equal(t, map[string]schema.PluginRegistry{"cloudposse": {Catalog: pluginconfig.DefaultCatalog, Auth: "docker"}}, list())
}

func TestPluginDefaultRegistryOverrideWarnsAndReplacesWholeEntry(t *testing.T) {
	config := pluginCoverageProject(t)
	config.Plugins.Discovery.DefaultRegistry = nil
	writePluginCoverageConfig(t, config)
	output := capturePluginUI(t)
	_, err := invokePluginCoverage(t, config, "registry", "add", "cloudposse", "--host=example.com")
	require.NoError(t, err)
	require.Contains(t, ansi.Strip(output.String()), "Replaced existing plugin registry")
	require.Contains(t, ansi.Strip(output.String()), pluginconfig.DefaultCatalog)
	reloadPluginCoverageConfig(t, config)
	require.Equal(t, schema.PluginRegistry{Host: "example.com", Auth: "docker"}, config.Plugins.Registries["cloudposse"])
	encoded, err := invokePluginCoverage(t, config, "registry", "list", "--format=json")
	require.NoError(t, err)
	var entries map[string]schema.PluginRegistry
	require.NoError(t, json.Unmarshal([]byte(encoded), &entries))
	require.Equal(t, config.Plugins.Registries, entries)
}
