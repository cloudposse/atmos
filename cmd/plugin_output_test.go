package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	plugincmd "github.com/cloudposse/atmos/cmd/plugin"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPluginOutputDefaultAndFormatPrecedence(t *testing.T) {
	config := pluginCoverageProject(t)
	config.Plugins.Registries = map[string]schema.PluginRegistry{
		"corporate": {Catalog: "oci://registry.example.com/catalog:stable", Auth: "docker"},
	}
	t.Setenv("ATMOS_PLUGIN_FORMAT", "")
	output, err := invokePluginCoverage(t, config, "registry", "list")
	require.NoError(t, err)
	require.Contains(t, output, "corporate")
	require.Contains(t, output, "\t")
	require.False(t, json.Valid([]byte(output)), "default table output must not remain JSON")

	t.Setenv("ATMOS_PLUGIN_FORMAT", "json")
	output, err = invokePluginCoverage(t, config, "registry", "list")
	require.NoError(t, err)
	var registries map[string]schema.PluginRegistry
	require.NoError(t, json.Unmarshal([]byte(output), &registries))
	require.Equal(t, config.Plugins.Registries, registries)

	output, err = invokePluginCoverage(t, config, "registry", "list", "--format=yaml")
	require.NoError(t, err)
	require.Contains(t, output, "corporate:")
	require.False(t, json.Valid([]byte(output)))
}

func TestPluginOutputListPreservesStructuredStatus(t *testing.T) {
	config := pluginCoverageProject(t)
	config.Plugins.Packages = map[string]schema.PluginPackage{
		"corporate": {Source: "oci://registry.example.com/plugins/login", Version: "^1"},
	}
	t.Setenv("ATMOS_PLUGIN_FORMAT", "yaml")
	output, err := invokePluginCoverage(t, config, "list", "--format=json")
	require.NoError(t, err)
	var statuses []manage.Status
	require.NoError(t, json.Unmarshal([]byte(output), &statuses))
	require.Equal(t, []manage.Status{{Alias: "corporate", State: "missing"}}, statuses)

	output, err = invokePluginCoverage(t, config, "info", "corporate", "--format=json")
	require.NoError(t, err)
	var status manage.Status
	require.NoError(t, json.Unmarshal([]byte(output), &status))
	require.Equal(t, statuses[0], status)
}

func TestPluginOutputRejectsFormatBeforeConfigurationAccess(t *testing.T) {
	for _, args := range [][]string{{"search"}, {"list"}, {"info", "corporate"}, {"registry", "list"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := invokePluginCoverage(t, nil, append(args, "--format=invalid")...)
			require.Error(t, err)
			require.NotErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			require.Contains(t, err.Error(), "format")
		})
	}
}

func TestPluginOutputEmptyStructuredListsAndCompletion(t *testing.T) {
	config := pluginCoverageProject(t)
	t.Setenv("ATMOS_PLUGIN_FORMAT", "json")
	for _, args := range [][]string{{"search", "no-match"}, {"list"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			output, err := invokePluginCoverage(t, config, args...)
			require.NoError(t, err)
			require.JSONEq(t, "[]", output)
		})
	}
	for _, args := range [][]string{{"search"}, {"list"}, {"info"}, {"registry", "list"}} {
		command, _, err := (&plugincmd.CommandProvider{}).GetCommand().Find(args)
		require.NoError(t, err)
		completion, found := command.GetFlagCompletionFunc("format")
		require.True(t, found)
		values, _ := completion(command, nil, "")
		require.Equal(t, []string{"table", "json", "yaml", "csv", "tsv"}, values)
	}
}
