package cmd

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"
)

func TestPluginAuthorOutputFormats(t *testing.T) {
	config := pluginCoverageProject(t)
	layout, release := pluginSigningLayout(t, config)
	directory := filepath.Join(config.BasePath, "plugin")
	t.Setenv("ATMOS_PLUGIN_FORMAT", "")
	output, err := invokePluginCoverage(t, config, "validate", directory)
	require.NoError(t, err)
	require.Contains(t, output, "auth.provider")
	require.NotContains(t, output, "Metadata")
	require.False(t, json.Valid([]byte(output)))
	t.Setenv("ATMOS_PLUGIN_FORMAT", "json")
	output, err = invokePluginCoverage(t, config, "validate", directory)
	require.NoError(t, err)
	var bindings []extension.Binding
	require.NoError(t, json.Unmarshal([]byte(output), &bindings))
	require.NotEmpty(t, bindings)
	require.Contains(t, output, "schema")
	output, err = invokePluginCoverage(t, config, "validate", directory, "--format=table")
	require.NoError(t, err)
	require.False(t, json.Valid([]byte(output)))
	output, err = invokePluginCoverage(t, config, "pack", directory, "--output", layout+"-json")
	require.NoError(t, err)
	var packed pluginoutput.PackedRelease
	require.NoError(t, json.Unmarshal([]byte(output), &packed))
	require.Equal(t, release.Name, packed.Name)
	require.Equal(t, release.Version, packed.Version)
	require.Equal(t, layout+"-json", packed.Output)
	require.NotEmpty(t, packed.Platforms)
}

func TestPluginAuthorRejectsFormatBeforeWork(t *testing.T) {
	config := pluginCoverageProject(t)
	for _, args := range [][]string{
		{"validate", "missing"},
		{"pack", "missing", "--output=missing"},
		{"publish", "missing", "--to=example.com/oidc"},
		{"sign", "example.com/oidc@sha256:invalid"},
	} {
		_, err := invokePluginCoverage(t, config, append(args, "--format=invalid")...)
		require.ErrorContains(t, err, "unsupported format")
	}
}
