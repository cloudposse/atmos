package oci_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

func TestReleasePreservesAuthoredMetadataAcrossPlatforms(t *testing.T) {
	directory, descriptors := packageFixture(t)
	definition, err := manifest.Load(directory)
	require.NoError(t, err)
	definition.Metadata.DisplayName = "Corporate Identity"
	definition.Metadata.Description = "OIDC login for corporate infrastructure"
	definition.Metadata.Author = "Infrastructure team"
	definition.Metadata.Repository = "https://github.com/example/atmos-oidc"
	raw, err := yaml.Marshal(definition)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), raw, 0o600))
	index, err := pluginoci.BuildIndex(directory, descriptors)
	require.NoError(t, err)
	release, err := pluginoci.Inspect(index, "")
	require.NoError(t, err)
	require.Equal(t, definition.Metadata, release.Definition.Metadata)
	require.Equal(t, "example/fixture", release.Name, "display name must not change canonical package identity")
	for _, platform := range []string{"linux/amd64", "darwin/arm64"} {
		destination := t.TempDir()
		require.NoError(t, release.Extract(platform, destination))
		installed, err := manifest.Load(destination)
		require.NoError(t, err)
		require.Equal(t, definition.Metadata, installed.Metadata)
	}
}
