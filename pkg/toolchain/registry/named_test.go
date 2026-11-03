package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	_ = schema.AtmosConfiguration{Toolchain: schema.Toolchain{}}
	_ = schema.Toolchain{Registries: []schema.ToolchainRegistry{}}
	_ = schema.ToolchainRegistry{Name: "", Type: "", Priority: 0, Tools: map[string]any{}}
)

func TestLoadNamedRegistryFallbacks(t *testing.T) {
	originalDefault, originalBuiltin := defaultRegistryFactory, builtinAtmosFactory
	t.Cleanup(func() { defaultRegistryFactory, builtinAtmosFactory = originalDefault, originalBuiltin })
	defaultRegistryFactory, builtinAtmosFactory = nil, nil
	for _, name := range []string{"aqua", "aqua-public"} {
		reg, err := LoadNamedFromConfig(nil, name)
		require.ErrorIs(t, err, ErrRegistryNotRegistered)
		assert.Nil(t, reg)
	}
	for _, name := range []string{"atmos-builtin", "unknown"} {
		reg, err := LoadNamedFromConfig(nil, name)
		require.ErrorIs(t, err, ErrUnknownRegistry)
		assert.Nil(t, reg)
	}
	defaultRegistry := &mockRegistry{}
	builtinRegistry := &mockRegistry{}
	defaultRegistryFactory = func() ToolRegistry { return defaultRegistry }
	builtinAtmosFactory = func() (ToolRegistry, error) { return builtinRegistry, nil }
	for _, name := range []string{"aqua", "aqua-public"} {
		reg, err := LoadNamedFromConfig(nil, name)
		require.NoError(t, err)
		assert.Same(t, defaultRegistry, reg)
	}
	reg, err := LoadNamedFromConfig(nil, "atmos-builtin")
	require.NoError(t, err)
	assert.Same(t, builtinRegistry, reg)
	builtinAtmosFactory = func() (ToolRegistry, error) { return nil, ErrRegistryParse }
	_, err = LoadNamedFromConfig(nil, "atmos-builtin")
	require.ErrorIs(t, err, ErrRegistryParse)
}

func TestNamedRegistryConfiguredPrecedence(t *testing.T) {
	original := atmosRegistryConstructor
	t.Cleanup(func() { atmosRegistryConstructor = original })
	configured := &mockRegistry{}
	atmosRegistryConstructor = func(tools map[string]any) (ToolRegistry, error) {
		assert.Equal(t, map[string]any{"marker": "configured"}, tools)
		return configured, nil
	}
	config := &schema.AtmosConfiguration{Toolchain: schema.Toolchain{Registries: []schema.ToolchainRegistry{
		{Name: "unrelated", Type: "unknown"},
		{Name: "aqua-public", Type: "atmos", Tools: map[string]any{"marker": "configured"}},
	}}}
	for _, name := range []string{"aqua", "aqua-public"} {
		reg, err := LoadNamedFromConfig(config, name)
		require.NoError(t, err)
		assert.Same(t, configured, reg)
	}
	_, err := LoadNamedFromConfig(config, "unrelated")
	require.ErrorIs(t, err, ErrUnknownRegistry)
	_, err = LoadNamedFromConfig(config, "missing")
	require.ErrorIs(t, err, ErrUnknownRegistry)
}

func TestConfiguredRegistryInventoryDoesNotLoadCatalogs(t *testing.T) {
	original := builtinAtmosFactory
	t.Cleanup(func() { builtinAtmosFactory = original })
	builtinAtmosFactory = nil
	assert.Equal(t, []ConfiguredRegistryInfo{{Name: "aqua-public", Type: "aqua", Priority: 10}}, ConfiguredRegistries(nil))
	builtinAtmosFactory = func() (ToolRegistry, error) {
		t.Fatal("inventory must not load a registry")
		return nil, ErrRegistryParse
	}
	builtin := ConfiguredRegistryInfo{Name: "atmos-builtin", Type: "atmos", Priority: BuiltinAtmosRegistryPriority}
	assert.Equal(t, []ConfiguredRegistryInfo{builtin, {Name: "aqua-public", Type: "aqua", Priority: 10}}, ConfiguredRegistries(&schema.AtmosConfiguration{}))
	config := &schema.AtmosConfiguration{Toolchain: schema.Toolchain{Registries: []schema.ToolchainRegistry{
		{Name: "private", Type: "atmos", Priority: 200},
	}}}
	assert.Equal(t, []ConfiguredRegistryInfo{builtin, {Name: "private", Type: "atmos", Priority: 200}}, ConfiguredRegistries(config))
}
