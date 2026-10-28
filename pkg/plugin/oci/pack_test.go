package oci_test

import (
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

func TestPackRestoreBothPlatformsWithoutExecuting(t *testing.T) {
	directory, descriptors := packageFixture(t)
	output := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, output, descriptors))
	index, err := atmosoci.LoadIndex(output, "")
	require.NoError(t, err)
	release, err := pluginoci.Inspect(index, "")
	require.NoError(t, err)
	require.Equal(t, "example/fixture", release.Name)
	require.Equal(t, "1.2.3", release.Version)
	require.Len(t, release.Platforms, 2)
	for platform, binary := range map[string]string{"linux/amd64": "linux-precompiled-fixture", "darwin/arm64": "mac-precompiled-fixture"} {
		t.Run(platform, func(t *testing.T) {
			destination := t.TempDir()
			require.NoError(t, release.Extract(platform, destination))
			raw, err := os.ReadFile(filepath.Join(destination, "bin", "plugin"))
			require.NoError(t, err)
			require.Equal(t, binary, string(raw))
		})
	}
	require.Error(t, release.Extract("windows/amd64", t.TempDir()))
	require.Error(t, pluginoci.Pack(directory, output, descriptors), "existing output must survive")
	second, err := pluginoci.BuildIndex(directory, descriptors)
	require.NoError(t, err)
	digest, err := second.Digest()
	require.NoError(t, err)
	require.Equal(t, release.Digest, digest.String(), "packing identical inputs is deterministic")
}

func TestPackRejectsMissingBinaryAndUnsupportedCapability(t *testing.T) {
	directory, descriptors := packageFixture(t)
	require.NoError(t, os.Remove(filepath.Join(directory, "bin", "mac")))
	_, err := pluginoci.BuildIndex(directory, descriptors)
	require.Error(t, err)
	_, err = pluginoci.BuildIndex(directory, extension.New())
	require.Error(t, err)
}

func TestInspectRejectsMixedMetadataAndChangedDigest(t *testing.T) {
	directory, descriptors := packageFixture(t)
	index, err := pluginoci.BuildIndex(directory, descriptors)
	require.NoError(t, err)
	_, err = pluginoci.Inspect(index, "sha256:changed")
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	changed := mutate.Annotations(index, map[string]string{"dev.atmos.plugin.version": "9.0.0"})
	// Annotations return the same interface shape as their input.
	_, err = pluginoci.Inspect(changed.(v1.ImageIndex), "")
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
}

func packageFixture(t *testing.T) (string, *extension.Registry) {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "linux"), []byte("linux-precompiled-fixture"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "mac"), []byte("mac-precompiled-fixture"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(fixtureManifest), 0o644))
	descriptors := extension.New()
	require.NoError(t, descriptors.Register(extension.Descriptor{Capability: "fixture", Path: []string{"fixtures"}, Version: 1, Validate: func(extension.Binding) error { return nil }}))
	return directory, descriptors
}

const fixtureManifest = `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: example/fixture
  version: 1.2.3
spec:
  runtime:
    type: native
    protocol: 1
    executable: bin/plugin
  compatibility:
    protocol: ">=1 <2"
  platforms:
    linux/amd64:
      executable: bin/linux
    darwin/arm64:
      executable: bin/mac
  capabilities:
    fixtures:
      example/fixture: {}
`
