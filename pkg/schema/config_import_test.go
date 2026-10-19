package schema

import (
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigImports_Paths(t *testing.T) {
	t.Run("nil list returns nil", func(t *testing.T) {
		var c ConfigImports
		assert.Nil(t, c.Paths())
	})

	t.Run("returns paths in order", func(t *testing.T) {
		c := ConfigImports{
			{Path: "a.yaml", SkipIfMissing: true},
			{Path: "b.yaml"},
			{Path: "c.yaml"},
		}
		paths := c.Paths()
		require.Len(t, paths, 3)
		assert.Equal(t, "a.yaml", paths[0])
		assert.Equal(t, "c.yaml", paths[2])
	})
}

func TestConfigImportsFromPaths(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		assert.Nil(t, ConfigImportsFromPaths(nil))
		assert.Nil(t, ConfigImportsFromPaths([]string{}))
	})

	t.Run("wraps each path as optional (fail-open)", func(t *testing.T) {
		c := ConfigImportsFromPaths([]string{"first.yaml", "mid.yaml", "last.yaml"})
		require.Len(t, c, 3)
		assert.Equal(t, "first.yaml", c[0].Path)
		assert.True(t, c[0].SkipIfMissing, "string-form imports must be optional for backward compatibility")
		assert.Equal(t, "last.yaml", c[2].Path)
		assert.True(t, c[2].SkipIfMissing)
	})
}

// decodeConfigImports decodes a raw value into ConfigImports through the same hook chain the
// config loader composes (condition hook for `when`, then the config-import hook).
func decodeConfigImports(t *testing.T, raw any) ConfigImports {
	t.Helper()
	var out ConfigImports
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			ConditionDecodeHook(),
			ConfigImportsDecodeHook(),
		),
		Result: &out,
	})
	require.NoError(t, err)
	require.NoError(t, decoder.Decode(raw))
	return out
}

func TestConfigImportsDecodeHook(t *testing.T) {
	t.Run("bare string is optional (fail-open)", func(t *testing.T) {
		out := decodeConfigImports(t, []any{"plain.yaml"})
		require.Len(t, out, 1)
		assert.Equal(t, "plain.yaml", out[0].Path)
		assert.True(t, out[0].SkipIfMissing)
		assert.True(t, out[0].When.IsZero())
	})

	t.Run("object defaults to required (fail-closed)", func(t *testing.T) {
		out := decodeConfigImports(t, []any{
			map[string]any{"path": "required.yaml"},
		})
		require.Len(t, out, 1)
		assert.Equal(t, "required.yaml", out[0].Path)
		assert.False(t, out[0].SkipIfMissing, "object imports must default to fail-closed")
	})

	t.Run("object honors explicit skip_if_missing and when", func(t *testing.T) {
		out := decodeConfigImports(t, []any{
			map[string]any{"path": "opt.yaml", "skip_if_missing": true, "when": "local"},
		})
		require.Len(t, out, 1)
		assert.Equal(t, "opt.yaml", out[0].Path)
		assert.True(t, out[0].SkipIfMissing)
		assert.False(t, out[0].When.IsZero(), "when condition must decode")
	})

	t.Run("mixed string and object entries", func(t *testing.T) {
		out := decodeConfigImports(t, []any{
			"legacy.yaml",
			map[string]any{"path": "modern.yaml"},
		})
		require.Len(t, out, 2)
		assert.Equal(t, "legacy.yaml", out[0].Path)
		assert.True(t, out[0].SkipIfMissing)
		assert.Equal(t, "modern.yaml", out[1].Path)
		assert.False(t, out[1].SkipIfMissing)
	})
}

func TestConfigImportMarshal(t *testing.T) {
	// A string-equivalent entry (optional, no condition) marshals back to a bare string so
	// `atmos describe config` output is unchanged from the legacy string form.
	t.Run("string-equivalent entry marshals as a bare string", func(t *testing.T) {
		c := ConfigImport{Path: "configs.d/**/*", SkipIfMissing: true}

		y, err := c.MarshalYAML()
		require.NoError(t, err)
		assert.Equal(t, "configs.d/**/*", y)

		j, err := c.MarshalJSON()
		require.NoError(t, err)
		assert.JSONEq(t, `"configs.d/**/*"`, string(j))
	})

	// A required entry (fail closed) marshals as an object; omitempty drops skip_if_missing:false.
	t.Run("required entry marshals as an object", func(t *testing.T) {
		c := ConfigImport{Path: "required.yaml"}

		y, err := c.MarshalYAML()
		require.NoError(t, err)
		assert.Equal(t, configImportObject{Path: "required.yaml"}, y)

		j, err := c.MarshalJSON()
		require.NoError(t, err)
		assert.JSONEq(t, `{"path":"required.yaml"}`, string(j))
	})

	// A conditional entry keeps its `when` in the object form.
	t.Run("conditional entry keeps when", func(t *testing.T) {
		c := ConfigImport{Path: "local.yaml", SkipIfMissing: true, When: MustCondition("local")}

		j, err := c.MarshalJSON()
		require.NoError(t, err)
		assert.Contains(t, string(j), `"path":"local.yaml"`)
		assert.Contains(t, string(j), `"when"`)
	})
}
