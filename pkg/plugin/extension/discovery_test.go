package extension

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestDiscoveryValidatesShapeWithoutLoadingSchemas(t *testing.T) {
	t.Parallel()
	var registry Registry
	descriptor := Descriptor{Capability: "auth.provider", Path: []string{"auth", "providers"}, Version: 1, Validate: ValidateAuth}
	require.NoError(t, registry.Register(descriptor))
	descriptor.Path[0] = "mutated"
	actual, ok := registry.Get("auth.provider")
	require.True(t, ok)
	require.Equal(t, []string{"auth", "providers"}, actual.Path)
	capabilities := map[string]any{"auth": map[string]any{"providers": map[string]any{"zeta": map[string]any{"schema": "absent.json"}, "alpha": map[string]any{"schema": "absent.json"}}}}
	bindings, err := registry.Discover(capabilities)
	require.NoError(t, err)
	require.Len(t, bindings, 2)
	require.Equal(t, "alpha", bindings[0].Name)
	require.Equal(t, "zeta", bindings[1].Name)
	_, err = registry.Validate(t.TempDir(), capabilities)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	for _, input := range []map[string]any{
		nil,
		{"auth": "wrong shape"},
		{"auth": map[string]any{"providers": true}},
		{"auth": map[string]any{"providers": map[string]any{"": map[string]any{}}}},
		{"auth": map[string]any{"providers": map[string]any{"example": 42}}},
	} {
		_, err := registry.Discover(input)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
	_, ok = registry.Get("missing")
	require.False(t, ok)
	for _, path := range [][]string{{""}, {"auth.providers"}} {
		require.ErrorIs(t, registry.Register(Descriptor{Capability: "invalid", Path: path, Version: 1, Validate: ValidateAuth}), errUtils.ErrPluginInvalidConfig)
	}
}
