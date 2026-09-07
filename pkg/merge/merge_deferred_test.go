package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMergeWithDeferred(t *testing.T) {
	t.Run("merges inputs and returns deferred context", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		inputs := []map[string]any{
			{
				"template": "!template 'value1'",
				"regular":  "string1",
			},
			{
				"template": "!template 'value2'",
				"regular":  "string2",
			},
		}

		result, dctx, err := MergeWithDeferred(cfg, inputs)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, dctx)

		// YAML function should be replaced with nil.
		assert.Nil(t, result["template"])
		// Regular value should be merged (last wins).
		assert.Equal(t, "string2", result["regular"])

		// Deferred context should have the YAML functions.
		assert.True(t, dctx.HasDeferredValues())
		values := dctx.GetDeferredValues()
		assert.Contains(t, values, "template")
		assert.Len(t, values["template"], 2)
	})

	t.Run("increments precedence for each input", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		inputs := []map[string]any{
			{"func": "!template 'first'"},
			{"func": "!template 'second'"},
			{"func": "!template 'third'"},
		}

		_, dctx, err := MergeWithDeferred(cfg, inputs)

		require.NoError(t, err)
		values := dctx.GetDeferredValues()["func"]
		assert.Len(t, values, 3)
		assert.Equal(t, 0, values[0].Precedence)
		assert.Equal(t, 1, values[1].Precedence)
		assert.Equal(t, 2, values[2].Precedence)
	})

	t.Run("handles inputs without YAML functions", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		inputs := []map[string]any{
			{"key1": "value1"},
			{"key2": "value2"},
		}

		result, dctx, err := MergeWithDeferred(cfg, inputs)

		require.NoError(t, err)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "value2", result["key2"])
		assert.False(t, dctx.HasDeferredValues())
	})
}

// TestMergeWithDeferred_TrivialInputShortCircuits exercises the all-empty
// fast path and asserts that the single-non-empty-input case follows the
// regular merge path (which always returns a deep-copied, caller-mutable
// map). The 1-input shortcut was tried in an earlier iteration and reverted
// after it broke TestSpaceliftStackProcessor by returning a shared reference
// to the caller — downstream mutation of the result then corrupted upstream
// cached settings/vars/auth for sibling components.
func TestMergeWithDeferred_TrivialInputShortCircuits(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: ListMergeStrategyReplace,
		},
	}

	t.Run("zero non-empty inputs returns empty result", func(t *testing.T) {
		result, dctx, err := MergeWithDeferred(cfg, []map[string]any{nil, {}, nil})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result)
		assert.NotNil(t, dctx)
		assert.False(t, dctx.HasDeferredValues())
	})

	t.Run("single non-empty input returns a fresh map (not shared with input)", func(t *testing.T) {
		single := map[string]any{
			"region":      "us-east-1",
			"environment": "prod",
		}
		inputs := []map[string]any{nil, {}, single, nil}
		result, dctx, err := MergeWithDeferred(cfg, inputs)
		require.NoError(t, err)
		assert.False(t, sameMapHeader(single, result),
			"single non-empty input must NOT share a map header with the caller — Merge contract is that the result is independent and caller-mutable")
		// Values still round-trip.
		assert.Equal(t, "us-east-1", result["region"])
		assert.Equal(t, "prod", result["environment"])
		assert.False(t, dctx.HasDeferredValues())
	})

	t.Run("single non-empty input with YAML functions defers correctly", func(t *testing.T) {
		single := map[string]any{
			"region":   "us-east-1",
			"template": "!template 'prod-{{ .region }}'",
		}
		inputs := []map[string]any{nil, single, nil}
		result, dctx, err := MergeWithDeferred(cfg, inputs)
		require.NoError(t, err)
		assert.False(t, sameMapHeader(single, result),
			"input with YAML functions must be walked into a fresh map")
		assert.Equal(t, "us-east-1", result["region"])
		assert.Nil(t, result["template"], "YAML function should be deferred to nil placeholder")
		assert.True(t, dctx.HasDeferredValues())
		deferred := dctx.GetDeferredValues()
		require.Contains(t, deferred, "template")
	})

	t.Run("two non-empty inputs use the full merge path", func(t *testing.T) {
		a := map[string]any{"key": "value-a", "shared": "from-a"}
		b := map[string]any{"key": "value-b", "extra": "from-b"}
		inputs := []map[string]any{a, b}
		result, dctx, err := MergeWithDeferred(cfg, inputs)
		require.NoError(t, err)
		// Full merge: later wins for overlapping keys.
		assert.Equal(t, "value-b", result["key"])
		assert.Equal(t, "from-a", result["shared"])
		assert.Equal(t, "from-b", result["extra"])
		// Result is a fresh map, not shared with either input.
		assert.False(t, sameMapHeader(a, result))
		assert.False(t, sameMapHeader(b, result))
		assert.False(t, dctx.HasDeferredValues())
	})

	t.Run("MergeWithDeferred does not mutate function-free input", func(t *testing.T) {
		single := map[string]any{"key": "value"}
		original := map[string]any{"key": "value"}
		inputs := []map[string]any{nil, single}

		_, _, err := MergeWithDeferred(cfg, inputs)
		require.NoError(t, err)

		// Input must be unchanged after the call.
		assert.Equal(t, original, single,
			"MergeWithDeferred must not mutate its input")
	})

	t.Run("mutating the result does not mutate the input (regression guard)", func(t *testing.T) {
		// Regression guard for the original Phase 5 1-input shortcut, which
		// returned the input map directly when WalkAndDeferYAMLFunctions's
		// short-circuit kicked in. Downstream mutation of the result then
		// corrupted upstream cached settings, dropping spacelift stacks in
		// TestSpaceliftStackProcessor.
		single := map[string]any{
			"region": "us-east-1",
			"nested": map[string]any{"inner": "original"},
		}
		inputs := []map[string]any{single}

		result, _, err := MergeWithDeferred(cfg, inputs)
		require.NoError(t, err)

		// Mutate every level of the result.
		result["region"] = "MUTATED"
		result["newKey"] = "added"
		if nested, ok := result["nested"].(map[string]any); ok {
			nested["inner"] = "MUTATED"
		}

		// The input must be unchanged.
		assert.Equal(t, "us-east-1", single["region"],
			"mutating Merge's result must not mutate the input")
		_, hasNewKey := single["newKey"]
		assert.False(t, hasNewKey, "input map must not gain keys added to the result")
		if nestedSingle, ok := single["nested"].(map[string]any); ok {
			assert.Equal(t, "original", nestedSingle["inner"],
				"nested maps must also be deep-copied")
		}
	})

	t.Run("mutating the input after merge does not mutate the result (regression guard)", func(t *testing.T) {
		// Mirror of the result→src isolation test above: also verify the
		// src→result direction. Mutating the source map after the merge
		// must not propagate into the returned result. Per CLAUDE.md
		// testing convention: aliasing tests must verify BOTH directions.
		single := map[string]any{
			"region": "us-east-1",
			"nested": map[string]any{"inner": "original"},
		}
		inputs := []map[string]any{single}

		result, _, err := MergeWithDeferred(cfg, inputs)
		require.NoError(t, err)

		// Mutate every level of the input AFTER the merge.
		single["region"] = "MUTATED_SRC"
		single["newKey"] = "added-src"
		if nested, ok := single["nested"].(map[string]any); ok {
			nested["inner"] = "MUTATED_SRC"
		}

		// The result must be unchanged.
		assert.Equal(t, "us-east-1", result["region"],
			"mutating the input after Merge must not mutate the result")
		_, hasNewKey := result["newKey"]
		assert.False(t, hasNewKey, "result must not gain keys added to the input post-merge")
		if nestedResult, ok := result["nested"].(map[string]any); ok {
			assert.Equal(t, "original", nestedResult["inner"],
				"nested maps must be deep-copied so post-merge src mutation cannot reach the result")
		}
	})
}
