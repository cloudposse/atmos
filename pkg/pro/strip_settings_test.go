package pro

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripToProSettings(t *testing.T) {
	t.Run("extracts pro key from settings", func(t *testing.T) {
		settings := map[string]any{
			"pro": map[string]any{
				"drift_detection": map[string]any{
					"enabled": true,
				},
			},
			"spacelift": map[string]any{
				"workspace_enabled": true,
			},
		}
		result := stripToProSettings(settings)

		require.NotNil(t, result)
		assert.Contains(t, result, "pro")
		assert.NotContains(t, result, "spacelift")

		// JSON-marshalable.
		data, err := json.Marshal(result)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"drift_detection"`)
	})

	t.Run("returns nil for nil settings", func(t *testing.T) {
		result := stripToProSettings(nil)
		assert.Nil(t, result)
	})

	t.Run("returns nil when no pro key", func(t *testing.T) {
		settings := map[string]any{
			"spacelift": map[string]any{"workspace_enabled": true},
		}
		result := stripToProSettings(settings)
		assert.Nil(t, result)
	})

	t.Run("sanitizes map[interface{}]interface{} in pro value", func(t *testing.T) {
		settings := map[string]any{
			"pro": map[interface{}]interface{}{
				"drift_detection": map[interface{}]interface{}{
					"enabled": true,
					"detect": map[interface{}]interface{}{
						"workflows": map[interface{}]interface{}{
							"plan.yml": map[interface{}]interface{}{
								"inputs": map[interface{}]interface{}{
									"component": "vpc",
								},
							},
						},
					},
				},
			},
		}
		result := stripToProSettings(settings)

		require.NotNil(t, result)
		data, err := json.Marshal(result)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"drift_detection"`)
		assert.Contains(t, string(data), `"plan.yml"`)
	})

	t.Run("result is isolated from input", func(t *testing.T) {
		settings := map[string]any{
			"pro": map[string]any{
				"enabled": true,
			},
		}
		result := stripToProSettings(settings)

		// result→src isolation: mutate result, verify input unchanged.
		result["pro"].(map[string]interface{})["enabled"] = false
		assert.Equal(t, true, settings["pro"].(map[string]any)["enabled"],
			"input must not be affected by mutating result")

		// src→result isolation: reset result, mutate input, verify result unchanged.
		result["pro"].(map[string]interface{})["enabled"] = true
		settings["pro"].(map[string]any)["enabled"] = false
		assert.Equal(t, true, result["pro"].(map[string]interface{})["enabled"],
			"result must not be affected by mutating input")
	})
}
