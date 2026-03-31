package pro

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeForJSON(t *testing.T) {
	t.Run("converts map[interface{}]interface{} to map[string]interface{}", func(t *testing.T) {
		input := map[interface{}]interface{}{
			"key1": "value1",
			"key2": map[interface{}]interface{}{
				"nested": "value",
			},
		}
		result := sanitizeForJSON(input)

		// Should be JSON-marshalable now.
		data, err := json.Marshal(result)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"key1":"value1"`)
		assert.Contains(t, string(data), `"nested":"value"`)
	})

	t.Run("leaves map[string]interface{} unchanged", func(t *testing.T) {
		input := map[string]interface{}{
			"key1": "value1",
			"key2": map[string]interface{}{
				"nested": "value",
			},
		}
		result := sanitizeForJSON(input)

		data, err := json.Marshal(result)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"key1":"value1"`)
	})

	t.Run("handles slices with nested maps", func(t *testing.T) {
		input := []interface{}{
			map[interface{}]interface{}{"a": 1},
			"plain",
		}
		result := sanitizeForJSON(input)

		data, err := json.Marshal(result)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"a":1`)
	})

	t.Run("returns nil for nil input", func(t *testing.T) {
		result := sanitizeMapForJSON(nil)
		assert.Nil(t, result)
	})
}
