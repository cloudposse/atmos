package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
)

// TestDescribePackageAdvertisesHostContractVersions verifies the handshake
// requires the current preparation contract. A plugin answering with the first
// preparation version fails negotiation before Prepare runs.
func TestDescribePackageAdvertisesHostContractVersions(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte("fixture"), 0o600))
	definition := &manifest.Config{}
	definition.Metadata.Name = "fixture/components"
	definition.Metadata.Version = "0.1.0"
	bindings := []extension.Binding{
		{Capability: extension.ComponentPreparation, Name: "fixture/hcl"},
		{Capability: extension.ComponentDiscovery, Name: "fixture/hcl"},
	}
	result, err := DescribePackage(directory, definition, bindings)
	require.NoError(t, err)
	versions := make(map[string][2]uint32)
	for _, service := range result.Services {
		versions[service.Service] = [2]uint32{service.Min, service.Max}
	}
	require.Equal(t, map[string][2]uint32{extension.ComponentDiscovery: {1, 1}, extension.ComponentPreparation: {2, 2}}, versions)

	registry, err := Descriptors()
	require.NoError(t, err)
	descriptor, ok := registry.Get(extension.ComponentPreparation)
	require.True(t, ok)
	require.Equal(t, uint32(2), descriptor.Version)
}
