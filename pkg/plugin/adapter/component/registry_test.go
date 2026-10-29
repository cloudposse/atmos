package component

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func fixtureActivations(t *testing.T) []*extension.Activation {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schema.json"), []byte(`{"type":"object","additionalProperties":false,"properties":{"limit":{"type":"number"}}}`), 0o600))
	executable, err := os.Executable()
	require.NoError(t, err)
	expected := &pluginv1.DescribeResponse{PluginName: "fixture/components", PluginVersion: "0.1.0", ManifestDigest: "sha256:fixture"}
	var result []*extension.Activation
	for _, capability := range []string{extension.ComponentDiscovery, extension.ComponentValidation, extension.ComponentPreparation, extension.ComponentState} {
		expected.Services = append(expected.Services, &pluginv1.ServiceVersionRange{Service: capability, Min: 1, Max: 1})
		expected.Exports = append(expected.Exports, &pluginv1.Export{Service: capability, Kind: "fixture/hcl"})
		result = append(result, &extension.Activation{Binding: extension.Binding{Capability: capability, Name: "fixture/hcl", Directory: directory, Metadata: map[string]any{"component_type": "terraform", "schema": "schema.json"}}, Package: expected.PluginName, Executable: executable, Description: expected})
	}
	raw, err := proto.Marshal(expected)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "description.pb"), raw, 0o600))
	return result
}

func TestRegistryGroupsCapabilitiesWithoutReplacingEngines(t *testing.T) {
	t.Parallel()
	activations := fixtureActivations(t)
	registry, err := NewRegistry(activations)
	require.NoError(t, err)
	require.Equal(t, []string{"fixture/hcl"}, registry.Names())
	provider, ok := registry.Get("fixture/hcl")
	require.True(t, ok)
	require.Equal(t, "terraform", provider.ComponentType())
	require.True(t, provider.Supports(extension.ComponentState))
	require.False(t, provider.Supports("component.execution"))
	_, ok = registry.Get("terraform")
	require.False(t, ok, "provider names do not occupy the engine namespace")
	activations[0].Description.PluginName = "changed"
	activations[0].Metadata["component_type"] = "changed"
	require.Equal(t, "fixture/components", provider.expected.PluginName)
	require.Equal(t, "terraform", provider.ComponentType())
	names := registry.Names()
	names[0] = "changed"
	require.Equal(t, []string{"fixture/hcl"}, registry.Names())
	_, ok = (*Registry)(nil).Get("anything")
	require.False(t, ok)
	require.Nil(t, (*Registry)(nil).Names())
}

func TestRegistryRejectsPartialAndConflictingBatches(t *testing.T) {
	t.Parallel()
	for name, modify := range map[string]func(*extension.Activation){
		"owner":              func(a *extension.Activation) { a.Package = "other/package" },
		"family":             func(a *extension.Activation) { a.Metadata["component_type"] = "other" },
		"executable":         func(a *extension.Activation) { a.Executable = "other" },
		"timeout":            func(a *extension.Activation) { a.Timeout = time.Second },
		"duplicate":          func(a *extension.Activation) { a.Capability = extension.ComponentDiscovery },
		"bad schema":         func(a *extension.Activation) { a.Metadata["schema"] = "missing.json" },
		"bad type":           func(a *extension.Activation) { a.Metadata["component_type"] = "" },
		"unknown key":        func(a *extension.Activation) { a.Metadata["typo"] = true },
		"unknown capability": func(a *extension.Activation) { a.Capability = "unknown" },
		"missing handshake":  func(a *extension.Activation) { a.Description = nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			items := fixtureActivations(t)
			modify(items[1])
			registry, err := NewRegistry(items)
			require.Error(t, err)
			require.Nil(t, registry, "no partial batch may become visible")
		})
	}
	_, err := NewRegistry([]*extension.Activation{nil})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	empty, err := NewRegistry(nil)
	require.NoError(t, err)
	require.Empty(t, empty.Names())
}

func TestUnsupportedAndInvalidCallsDoNotStartExecutable(t *testing.T) {
	t.Parallel()
	items := fixtureActivations(t)[:1]
	items[0].Executable = filepath.Join(t.TempDir(), "does-not-exist")
	registry, err := NewRegistry(items)
	require.NoError(t, err)
	p, _ := registry.Get("fixture/hcl")
	request := &pluginv1.ComponentRequest{RootDirectory: t.TempDir(), Address: fixtureAddress()}
	_, err = p.ResolveState(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	_, err = p.Validate(t.Context(), nil)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	_, err = p.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: "relative"})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = p.Discover(ctx, &pluginv1.DiscoverComponentsRequest{RootDirectory: request.RootDirectory})
	require.ErrorIs(t, err, context.Canceled)
}
