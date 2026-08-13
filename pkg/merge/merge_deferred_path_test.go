package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsMap(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected bool
	}{
		{
			name:     "returns true for map[string]interface{}",
			input:    map[string]interface{}{"key": "value"},
			expected: true,
		},
		{
			name:     "returns false for string",
			input:    "string",
			expected: false,
		},
		{
			name:     "returns false for slice",
			input:    []interface{}{1, 2, 3},
			expected: false,
		},
		{
			name:     "returns false for nil",
			input:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected bool
	}{
		{
			name:     "returns true for []interface{}",
			input:    []interface{}{1, 2, 3},
			expected: true,
		},
		{
			name:     "returns false for string",
			input:    "string",
			expected: false,
		},
		{
			name:     "returns false for map",
			input:    map[string]interface{}{"key": "value"},
			expected: false,
		},
		{
			name:     "returns false for nil",
			input:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSlice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetValueAtPath(t *testing.T) {
	t.Run("sets value at simple path", func(t *testing.T) {
		data := map[string]interface{}{}
		err := SetValueAtPath(data, []string{"key"}, "value")

		require.NoError(t, err)
		assert.Equal(t, "value", data["key"])
	})

	t.Run("sets value at nested path", func(t *testing.T) {
		data := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{},
			},
		}

		err := SetValueAtPath(data, []string{"level1", "level2", "key"}, "value")

		require.NoError(t, err)
		level1 := data["level1"].(map[string]interface{})
		level2 := level1["level2"].(map[string]interface{})
		assert.Equal(t, "value", level2["key"])
	})

	t.Run("creates intermediate maps if missing", func(t *testing.T) {
		data := map[string]interface{}{}

		err := SetValueAtPath(data, []string{"new", "nested", "key"}, "value")

		require.NoError(t, err)
		level1 := data["new"].(map[string]interface{})
		level2 := level1["nested"].(map[string]interface{})
		assert.Equal(t, "value", level2["key"])
	})

	t.Run("overwrites existing value", func(t *testing.T) {
		data := map[string]interface{}{
			"key": "old",
		}

		err := SetValueAtPath(data, []string{"key"}, "new")

		require.NoError(t, err)
		assert.Equal(t, "new", data["key"])
	})

	t.Run("returns error for empty path", func(t *testing.T) {
		data := map[string]interface{}{}
		err := SetValueAtPath(data, []string{}, "value")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty path")
	})

	t.Run("returns error when path encounters non-map", func(t *testing.T) {
		data := map[string]interface{}{
			"level1": "string value",
		}

		err := SetValueAtPath(data, []string{"level1", "level2", "key"}, "value")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a map")
	})
}

// TestGetValueAtPath tests the GetValueAtPath function.
func TestGetValueAtPath(t *testing.T) {
	t.Run("gets value at top-level path", func(t *testing.T) {
		data := map[string]interface{}{
			"key": "value",
		}
		path := []string{"key"}

		value, exists := GetValueAtPath(data, path)

		assert.True(t, exists)
		assert.Equal(t, "value", value)
	})

	t.Run("gets value at nested path", func(t *testing.T) {
		data := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"key": "nested_value",
				},
			},
		}
		path := []string{"level1", "level2", "key"}

		value, exists := GetValueAtPath(data, path)

		assert.True(t, exists)
		assert.Equal(t, "nested_value", value)
	})

	t.Run("returns false for non-existent path", func(t *testing.T) {
		data := map[string]interface{}{
			"key": "value",
		}
		path := []string{"nonexistent"}

		value, exists := GetValueAtPath(data, path)

		assert.False(t, exists)
		assert.Nil(t, value)
	})

	t.Run("returns false for partial path", func(t *testing.T) {
		data := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": "value",
			},
		}
		path := []string{"level1", "level2", "level3"}

		value, exists := GetValueAtPath(data, path)

		assert.False(t, exists)
		assert.Nil(t, value)
	})

	t.Run("returns false for nil data", func(t *testing.T) {
		var data map[string]interface{}
		path := []string{"key"}

		value, exists := GetValueAtPath(data, path)

		assert.False(t, exists)
		assert.Nil(t, value)
	})

	t.Run("returns false for empty path", func(t *testing.T) {
		data := map[string]interface{}{
			"key": "value",
		}
		path := []string{}

		value, exists := GetValueAtPath(data, path)

		assert.False(t, exists)
		assert.Nil(t, value)
	})

	t.Run("handles nil values", func(t *testing.T) {
		data := map[string]interface{}{
			"key": nil,
		}
		path := []string{"key"}

		value, exists := GetValueAtPath(data, path)

		assert.True(t, exists)
		assert.Nil(t, value)
	})
}

// TestGetConfigOrDefault tests the getConfigOrDefault function.
func TestGetConfigOrDefault(t *testing.T) {
	t.Run("returns provided config when not nil", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "append",
			},
		}

		result := getConfigOrDefault(cfg)

		assert.Equal(t, cfg, result)
		assert.Equal(t, "append", result.Settings.ListMergeStrategy)
	})

	t.Run("returns default config when nil", func(t *testing.T) {
		result := getConfigOrDefault(nil)

		assert.NotNil(t, result)
		assert.Equal(t, "", result.Settings.ListMergeStrategy)
	})
}
