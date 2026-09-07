package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolvePreset(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	t.Run("self", func(t *testing.T) {
		preset, ok := ResolvePreset("self")
		require.True(t, ok)
		assert.Equal(t, "atmos", preset.DefaultServerName)
		assert.True(t, preset.RequiresMCPEnabled)

		cfg := preset.Resolve(atmosConfig)
		assert.Equal(t, "atmos", cfg.Command)
		assert.Equal(t, []string{"mcp", "start"}, cfg.Args)
	})

	t.Run("atmos-pro", func(t *testing.T) {
		preset, ok := ResolvePreset("atmos-pro")
		require.True(t, ok)
		assert.Equal(t, "atmos-pro", preset.DefaultServerName)
		assert.False(t, preset.RequiresMCPEnabled)

		cfg := preset.Resolve(atmosConfig)
		assert.Equal(t, schema.MCPTransportHTTP, cfg.Type)
		assert.Equal(t, "https://atmos-pro.com/mcp", cfg.URL)
	})

	t.Run("unknown name falls through", func(t *testing.T) {
		_, ok := ResolvePreset("uvx")
		assert.False(t, ok)

		_, ok = ResolvePreset("https://mcp.example.com/mcp")
		assert.False(t, ok)

		_, ok = ResolvePreset("")
		assert.False(t, ok)
	})
}

func TestPresets(t *testing.T) {
	all := Presets()
	require.Len(t, all, 2)

	names := map[string]bool{}
	for _, preset := range all {
		names[preset.Name] = true
	}
	assert.True(t, names[PresetSelf])
	assert.True(t, names[PresetAtmosPro])

	// Presets() must return a copy -- mutating the result must not affect the registry.
	all[0].Name = "mutated"
	fresh, ok := ResolvePreset(PresetSelf)
	require.True(t, ok)
	assert.Equal(t, PresetSelf, fresh.Name)
}
