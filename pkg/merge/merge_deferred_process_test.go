package merge

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestProcessYAMLFunctions tests the processYAMLFunctions helper function.
func TestProcessYAMLFunctions(t *testing.T) {
	t.Run("processes YAML functions successfully", func(t *testing.T) {
		// Create a mock processor.
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				// Simple processor that converts "!template X" to uppercase.
				if strings.HasPrefix(value, "!template ") {
					return strings.ToUpper(strings.TrimPrefix(value, "!template ")), nil
				}
				return value, nil
			},
		}

		deferredValues := []*DeferredValue{
			{Value: "!template hello", IsFunction: true},
			{Value: "!template world", IsFunction: true},
		}

		err := processYAMLFunctions(deferredValues, processor, "test.path")

		require.NoError(t, err)
		assert.Equal(t, "HELLO", deferredValues[0].Value)
		assert.False(t, deferredValues[0].IsFunction)
		assert.Equal(t, "WORLD", deferredValues[1].Value)
		assert.False(t, deferredValues[1].IsFunction)
	})

	t.Run("skips non-function values", func(t *testing.T) {
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				t.Fatal("processor should not be called for non-function values")
				return nil, nil
			},
		}

		deferredValues := []*DeferredValue{
			{Value: "regular string", IsFunction: false},
			{Value: 123, IsFunction: false},
		}

		err := processYAMLFunctions(deferredValues, processor, "test.path")

		require.NoError(t, err)
		assert.Equal(t, "regular string", deferredValues[0].Value)
		assert.Equal(t, 123, deferredValues[1].Value)
	})

	t.Run("skips non-string function values", func(t *testing.T) {
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				t.Fatal("processor should not be called for non-string values")
				return nil, nil
			},
		}

		deferredValues := []*DeferredValue{
			{Value: 123, IsFunction: true},           // Non-string but marked as function.
			{Value: []string{"a"}, IsFunction: true}, // Non-string but marked as function.
		}

		err := processYAMLFunctions(deferredValues, processor, "test.path")

		require.NoError(t, err)
		// Values should remain unchanged.
		assert.Equal(t, 123, deferredValues[0].Value)
		assert.Equal(t, []string{"a"}, deferredValues[1].Value)
	})

	t.Run("returns error on processing failure", func(t *testing.T) {
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				return nil, errors.New("processing failed")
			},
		}

		deferredValues := []*DeferredValue{
			{Value: "!template error", IsFunction: true},
		}

		err := processYAMLFunctions(deferredValues, processor, "test.path")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to process YAML function at test.path")
		assert.Contains(t, err.Error(), "processing failed")
	})

	t.Run("processes mixed values correctly", func(t *testing.T) {
		processCount := 0
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				processCount++
				return "processed", nil
			},
		}

		deferredValues := []*DeferredValue{
			{Value: "!template func1", IsFunction: true}, // Should process.
			{Value: "regular", IsFunction: false},        // Should skip.
			{Value: "!template func2", IsFunction: true}, // Should process.
			{Value: 123, IsFunction: true},               // Should skip (non-string).
		}

		err := processYAMLFunctions(deferredValues, processor, "test.path")

		require.NoError(t, err)
		assert.Equal(t, 2, processCount, "should process exactly 2 values")
		assert.Equal(t, "processed", deferredValues[0].Value)
		assert.False(t, deferredValues[0].IsFunction)
		assert.Equal(t, "regular", deferredValues[1].Value)
		assert.Equal(t, "processed", deferredValues[2].Value)
		assert.False(t, deferredValues[2].IsFunction)
		assert.Equal(t, 123, deferredValues[3].Value)
	})

	t.Run("handles empty deferred values", func(t *testing.T) {
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				t.Fatal("processor should not be called for empty slice")
				return nil, nil
			},
		}

		var deferredValues []*DeferredValue

		err := processYAMLFunctions(deferredValues, processor, "test.path")

		require.NoError(t, err)
	})
}

// TestFillMissingLayerValues tests the fillMissingLayerValues function.
func TestFillMissingLayerValues(t *testing.T) {
	t.Run("adds a concrete value from a higher-precedence layer not yet represented", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"key"}, "!template 'value1'")
		dctx.IncrementPrecedence()

		processedInputs := []map[string]any{
			{"key": "!template 'value1'"}, // precedence 0, already recorded above
			{"key": "existing_value"},     // precedence 1, concrete, not yet recorded
		}

		fillMissingLayerValues(dctx, processedInputs)

		values := dctx.GetDeferredValues()["key"]
		require.Len(t, values, 2)
		assert.Equal(t, "existing_value", values[1].Value)
		assert.Equal(t, 1, values[1].Precedence)
		assert.False(t, values[1].IsFunction)
	})

	t.Run("adds a concrete value from a LOWER-precedence layer that the function would structurally overwrite", func(t *testing.T) {
		// This is the mirror-precedence case: the deferred function is the higher-precedence
		// layer, so without this backfill the lower-precedence concrete value would never survive
		// the raw structural merge for ApplyDeferredMerges to find later.
		dctx := NewDeferredMergeContext()
		dctx.IncrementPrecedence()
		dctx.AddDeferred([]string{"key"}, "!template 'value2'") // precedence 1

		processedInputs := []map[string]any{
			{"key": "existing_value"},     // precedence 0, concrete, not yet recorded
			{"key": "!template 'value2'"}, // precedence 1, already recorded above
		}

		fillMissingLayerValues(dctx, processedInputs)

		values := dctx.GetDeferredValues()["key"]
		require.Len(t, values, 2)
		// The backfilled entry is appended after the pre-existing function entry, so it's at
		// index 1, not 0 — find it by precedence rather than assuming slice order.
		var backfilled *DeferredValue
		for _, dv := range values {
			if dv.Precedence == 0 {
				backfilled = dv
			}
		}
		require.NotNil(t, backfilled, "expected a backfilled entry at precedence 0")
		assert.Equal(t, "existing_value", backfilled.Value)
		assert.False(t, backfilled.IsFunction)
	})

	t.Run("does not add anything when every layer is already represented", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"key"}, "!template 'value1'")
		dctx.IncrementPrecedence()
		dctx.AddDeferred([]string{"key"}, "!template 'value2'")

		processedInputs := []map[string]any{
			{"key": "!template 'value1'"},
			{"key": "!template 'value2'"},
		}

		fillMissingLayerValues(dctx, processedInputs)

		assert.Len(t, dctx.GetDeferredValues()["key"], 2)
	})

	t.Run("does not add anything when the other layer has no value at that path", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"key"}, "!template 'value1'")
		dctx.IncrementPrecedence()

		processedInputs := []map[string]any{
			{"key": "!template 'value1'"},
			{}, // no "key" at all in this layer
		}

		fillMissingLayerValues(dctx, processedInputs)

		assert.Len(t, dctx.GetDeferredValues()["key"], 1)
	})

	t.Run("skips a nil placeholder at an unrepresented layer (that layer's own value wasn't a function, so it can't produce a placeholder there — defensive)", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"key"}, "!template 'value1'")
		dctx.IncrementPrecedence()

		processedInputs := []map[string]any{
			{"key": "!template 'value1'"},
			{"key": nil},
		}

		fillMissingLayerValues(dctx, processedInputs)

		assert.Len(t, dctx.GetDeferredValues()["key"], 1)
	})

	t.Run("skips a pathKey whose deferred slice is empty (defensive guard)", func(t *testing.T) {
		// Under the current API, AddDeferred always appends at least one element, so a pathKey
		// mapping to an empty slice can't naturally arise from public methods — seed it directly
		// to exercise the defensive len(values) == 0 guard without panicking on values[0].
		dctx := NewDeferredMergeContext()
		dctx.deferredValues["key"] = []*DeferredValue{}

		processedInputs := []map[string]any{
			{"key": "existing_value"},
		}

		assert.NotPanics(t, func() {
			fillMissingLayerValues(dctx, processedInputs)
		})
		assert.Empty(t, dctx.GetDeferredValues()["key"])
	})
}

// TestProcessDeferredField tests the processDeferredField function.
func TestProcessDeferredField(t *testing.T) {
	t.Run("processes field with yaml functions", func(t *testing.T) {
		result := map[string]interface{}{}
		deferredValues := []*DeferredValue{
			{
				Path:       []string{"config"},
				Value:      "!template 'value1'",
				Precedence: 0,
				IsFunction: true,
			},
			{
				Path:       []string{"config"},
				Value:      "!template 'value2'",
				Precedence: 1,
				IsFunction: true,
			},
		}
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				// Simulate processing templates.
				if value == "!template 'value1'" {
					return "processed1", nil
				}
				return "processed2", nil
			},
		}

		err := processDeferredField("config", deferredValues, result, cfg, processor)

		assert.NoError(t, err)
		assert.Equal(t, "processed2", result["config"]) // Higher precedence wins.
	})

	t.Run("processes field without yaml functions", func(t *testing.T) {
		result := map[string]interface{}{}
		deferredValues := []*DeferredValue{
			{
				Path:       []string{"config"},
				Value:      "value1",
				Precedence: 0,
				IsFunction: false,
			},
			{
				Path:       []string{"config"},
				Value:      "value2",
				Precedence: 1,
				IsFunction: false,
			},
		}
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}

		err := processDeferredField("config", deferredValues, result, cfg, nil)

		assert.NoError(t, err)
		assert.Equal(t, "value2", result["config"]) // Higher precedence wins.
	})

	t.Run("higher-precedence concrete value in deferredValues wins over a lower-precedence function", func(t *testing.T) {
		// Discovering a competing concrete value from another layer is MergeWithDeferred's job
		// (via fillMissingLayerValues, tested separately) — by the time processDeferredField runs,
		// every layer's contribution, concrete or function, is already present in deferredValues.
		result := map[string]interface{}{}
		deferredValues := []*DeferredValue{
			{
				Path:       []string{"config"},
				Value:      "!template 'deferred'",
				Precedence: 0,
				IsFunction: true,
			},
			{
				Path:       []string{"config"},
				Value:      "existing",
				Precedence: 1,
				IsFunction: false,
			},
		}
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}

		err := processDeferredField("config", deferredValues, result, cfg, nil)

		assert.NoError(t, err)
		// Higher-precedence concrete value wins over the (unresolved, since processor is nil)
		// lower-precedence function string.
		assert.Equal(t, "existing", result["config"])
	})

	t.Run("handles processor error", func(t *testing.T) {
		result := map[string]interface{}{}
		deferredValues := []*DeferredValue{
			{
				Path:       []string{"config"},
				Value:      "!template 'invalid'",
				Precedence: 0,
				IsFunction: true,
			},
		}
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				return nil, fmt.Errorf("template processing failed")
			},
		}

		err := processDeferredField("config", deferredValues, result, cfg, processor)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "template processing failed")
	})

	t.Run("handles path navigation error", func(t *testing.T) {
		// Create a result where the path cannot be set (non-map intermediate value).
		result := map[string]interface{}{
			"level1": "string_value", // This is not a map, so we can't navigate deeper.
		}
		deferredValues := []*DeferredValue{
			{
				Path:       []string{"level1", "level2", "key"},
				Value:      "value",
				Precedence: 0,
				IsFunction: false,
			},
		}
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}

		err := processDeferredField("level1.level2.key", deferredValues, result, cfg, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to set value at level1.level2.key")
	})
}
