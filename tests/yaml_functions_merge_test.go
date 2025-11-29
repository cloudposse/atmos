package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDeferredMergeWithYAMLFunctions tests the deferred merge functionality
// with various YAML function scenarios.
func TestDeferredMergeWithYAMLFunctions(t *testing.T) {
	t.Run("defers template functions during merge", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}

		// Simulate base catalog with !template
		base := map[string]any{
			"template_config": "!template '{{ toJson .settings.base }}'",
			"regular_value":   "base",
		}

		// Override with concrete value (would cause type conflict without deferred merge)
		override := map[string]any{
			"template_config": map[string]interface{}{
				"custom_key": "value",
			},
			"regular_value": "override",
		}

		result, dctx, err := merge.MergeWithDeferred(cfg, []map[string]any{base, override})

		require.NoError(t, err)
		assert.NotNil(t, dctx)
		assert.True(t, dctx.HasDeferredValues())

		// YAML function should be deferred.
		deferred := dctx.GetDeferredValues()
		assert.Contains(t, deferred, "template_config")

		// Result should have the concrete map (from second input) since it wasn't a YAML function.
		// The YAML function from the first input was deferred.
		templateConfig, ok := result["template_config"].(map[string]interface{})
		assert.True(t, ok, "template_config should be a map after merge")
		assert.Equal(t, "value", templateConfig["custom_key"])
		assert.Equal(t, "override", result["regular_value"])

		// Apply deferred merges.
		err = merge.ApplyDeferredMerges(dctx, result, cfg)
		require.NoError(t, err)

		// After applying, the deferred YAML function string should be merged in.
		// Note: Actual YAML function processing is TODO, so it remains as string.
		// In a full implementation, this would process the YAML function and merge the result.
		assert.NotNil(t, result["template_config"])
	})

	t.Run("handles multiple yaml functions with precedence", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}

		inputs := []map[string]any{
			{
				"config": "!template 'value1'",
			},
			{
				"config": "!template 'value2'",
			},
			{
				"config": "!template 'value3'",
			},
		}

		result, dctx, err := merge.MergeWithDeferred(cfg, inputs)

		require.NoError(t, err)

		// All YAML functions should be deferred.
		deferred := dctx.GetDeferredValues()["config"]
		assert.Len(t, deferred, 3)

		// Verify precedence order.
		assert.Equal(t, 0, deferred[0].Precedence)
		assert.Equal(t, 1, deferred[1].Precedence)
		assert.Equal(t, 2, deferred[2].Precedence)

		// Apply deferred merges (with replace strategy, last wins).
		err = merge.ApplyDeferredMerges(dctx, result, cfg)
		require.NoError(t, err)

		assert.Equal(t, "!template 'value3'", result["config"])
	})

	t.Run("detects all yaml function types", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}

		input := map[string]any{
			"template":         "!template 'value'",
			"terraform_output": "!terraform.output vpc id",
			"terraform_state":  "!terraform.state vpc arn",
			"store_get":        "!store.get secret/key",
			"store":            "!store secret/key",
			"exec":             "!exec echo hello",
			"env":              "!env AWS_REGION",
			"regular":          "not a function",
		}

		result, dctx, err := merge.MergeWithDeferred(cfg, []map[string]any{input})

		require.NoError(t, err)

		// All YAML functions should be deferred.
		deferred := dctx.GetDeferredValues()
		assert.Contains(t, deferred, "template")
		assert.Contains(t, deferred, "terraform_output")
		assert.Contains(t, deferred, "terraform_state")
		assert.Contains(t, deferred, "store_get")
		assert.Contains(t, deferred, "store")
		assert.Contains(t, deferred, "exec")
		assert.Contains(t, deferred, "env")

		// Regular value should not be deferred.
		assert.NotContains(t, deferred, "regular")
		assert.Equal(t, "not a function", result["regular"])
	})

	t.Run("handles nested yaml functions", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}

		input := map[string]any{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"yaml_func": "!template 'nested'",
					"regular":   "value",
				},
			},
		}

		result, dctx, err := merge.MergeWithDeferred(cfg, []map[string]any{input})

		require.NoError(t, err)

		// Nested YAML function should be deferred.
		deferred := dctx.GetDeferredValues()
		assert.Contains(t, deferred, "level1.level2.yaml_func")

		// Navigate to nested result.
		level1 := result["level1"].(map[string]interface{})
		level2 := level1["level2"].(map[string]interface{})

		// YAML function should be nil placeholder.
		assert.Nil(t, level2["yaml_func"])
		// Regular value should be preserved.
		assert.Equal(t, "value", level2["regular"])
	})

	t.Run("respects list merge strategy with yaml functions", func(t *testing.T) {
		testCases := []struct {
			name     string
			strategy string
			inputs   []map[string]any
			expected interface{}
		}{
			{
				name:     "replace strategy",
				strategy: "replace",
				inputs: []map[string]any{
					{"list": []interface{}{1, 2}},
					{"list": []interface{}{3, 4}},
				},
				expected: []interface{}{3, 4},
			},
			{
				name:     "append strategy",
				strategy: "append",
				inputs: []map[string]any{
					{"list": []interface{}{1, 2}},
					{"list": []interface{}{3, 4}},
				},
				expected: []interface{}{1, 2, 3, 4},
			},
			{
				name:     "merge strategy",
				strategy: "merge",
				inputs: []map[string]any{
					{
						"list": []interface{}{
							map[string]interface{}{"a": 1, "b": 2},
							map[string]interface{}{"c": 3},
						},
					},
					{
						"list": []interface{}{
							map[string]interface{}{"b": 20, "d": 4},
						},
					},
				},
				expected: []interface{}{
					map[string]interface{}{"a": 1, "b": 20, "d": 4},
					map[string]interface{}{"c": 3},
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						ListMergeStrategy: tc.strategy,
					},
				}

				result, _, err := merge.MergeWithDeferred(cfg, tc.inputs)

				require.NoError(t, err)
				assert.Equal(t, tc.expected, result["list"])
			})
		}
	})

	t.Run("handles type conflicts gracefully", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}

		// Base has YAML function (will return list after processing)
		base := map[string]any{
			"config": "!terraform.output vpc ids",
		}

		// Override with concrete map (type conflict without deferred merge)
		override := map[string]any{
			"config": map[string]interface{}{
				"custom": "value",
			},
		}

		result, dctx, err := merge.MergeWithDeferred(cfg, []map[string]any{base, override})

		// Should not error - YAML function is deferred.
		require.NoError(t, err)
		assert.True(t, dctx.HasDeferredValues())

		// Apply deferred merges.
		err = merge.ApplyDeferredMerges(dctx, result, cfg)
		require.NoError(t, err)

		// The deferred YAML function is applied.
		// Note: Since YAML function processing is TODO, the string is merged as-is.
		// In practice, the YAML function would be processed first, then merged.
		assert.NotNil(t, result["config"])
	})
}

// TestDeferredMergeNilHandling tests that nil values are handled correctly.
func TestDeferredMergeNilHandling(t *testing.T) {
	t.Run("handles nil inputs", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}

		result, dctx, err := merge.MergeWithDeferred(cfg, []map[string]any{nil, nil})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, dctx.HasDeferredValues())
	})

	t.Run("handles empty inputs", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}

		result, dctx, err := merge.MergeWithDeferred(cfg, []map[string]any{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, dctx.HasDeferredValues())
	})

	t.Run("applies deferred merges with nil context", func(t *testing.T) {
		result := map[string]interface{}{"key": "value"}

		err := merge.ApplyDeferredMerges(nil, result, nil)

		assert.NoError(t, err)
		assert.Equal(t, "value", result["key"])
	})
}
