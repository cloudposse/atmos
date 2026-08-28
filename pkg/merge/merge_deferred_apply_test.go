package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestApplyDeferredMerges(t *testing.T) {
	t.Run("returns nil error when context is nil", func(t *testing.T) {
		result := map[string]interface{}{}
		err := ApplyDeferredMerges(nil, result, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("returns nil error when no deferred values", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		result := map[string]interface{}{}
		err := ApplyDeferredMerges(dctx, result, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("applies deferred values to result", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		dctx := NewDeferredMergeContext()
		// Simulate deferred YAML function strings.
		// Note: These won't be processed (processor is nil),
		// but will be merged as strings.
		dctx.AddDeferred([]string{"config"}, "!template 'value'")

		result := map[string]interface{}{}

		err := ApplyDeferredMerges(dctx, result, cfg, nil)

		require.NoError(t, err)
		// The value should be set (as the string, since no processor was provided).
		assert.Equal(t, "!template 'value'", result["config"])
	})

	t.Run("sorts by precedence before merging", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		dctx := NewDeferredMergeContext()
		// Add in non-sorted order.
		dctx.precedence = 2
		dctx.AddDeferred([]string{"key"}, "third")
		dctx.precedence = 0
		dctx.AddDeferred([]string{"key"}, "first")
		dctx.precedence = 1
		dctx.AddDeferred([]string{"key"}, "second")

		result := map[string]interface{}{}

		err := ApplyDeferredMerges(dctx, result, cfg, nil)

		require.NoError(t, err)
		// With replace strategy, last (highest precedence) should win.
		assert.Equal(t, "third", result["key"])
	})

	t.Run("handles nested paths", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"level1", "level2", "key"}, "value")

		result := map[string]interface{}{}

		err := ApplyDeferredMerges(dctx, result, cfg, nil)

		require.NoError(t, err)

		// Use require guards for type assertions to provide clear test failures instead of panics.
		level1, ok := result["level1"].(map[string]interface{})
		require.True(t, ok, "level1 should be a map")
		level2, ok := level1["level2"].(map[string]interface{})
		require.True(t, ok, "level2 should be a map")

		assert.Equal(t, "value", level2["key"])
	})

	t.Run("uses default strategy when atmosConfig is nil", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"key"}, []interface{}{1, 2})
		dctx.IncrementPrecedence()
		dctx.AddDeferred([]string{"key"}, []interface{}{3, 4})

		result := map[string]interface{}{}

		err := ApplyDeferredMerges(dctx, result, nil, nil)

		require.NoError(t, err)
		// Default is replace strategy, so last value wins.
		assert.Equal(t, []interface{}{3, 4}, result["key"])
	})

	t.Run("does not mutate the caller's context when a processor is provided", func(t *testing.T) {
		// Regression guard for the deferred-context cache-safety fix: a dctx recovered from
		// shared/cached storage (e.g. a FindStacksMap cache entry) must remain safe for a
		// concurrent caller to resolve independently. ApplyDeferredMerges must never mutate
		// the DeferredValue.Value/IsFunction fields of the dctx it was given when a processor
		// resolves them — it must operate on a clone internally.
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"config"}, "!template 'value'")

		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				return "resolved", nil
			},
		}

		result := map[string]interface{}{}
		err := ApplyDeferredMerges(dctx, result, cfg, processor)
		require.NoError(t, err)
		assert.Equal(t, "resolved", result["config"], "the result should reflect the processed value")

		// The original context handed in by the caller must be untouched.
		originalValues := dctx.GetDeferredValues()["config"]
		require.Len(t, originalValues, 1)
		assert.True(t, originalValues[0].IsFunction, "caller's DeferredValue.IsFunction must not be mutated")
		assert.Equal(t, "!template 'value'", originalValues[0].Value, "caller's DeferredValue.Value must not be mutated")

		// A second, independent resolution of the same original context must resolve again
		// from the unresolved string, not reuse (or be corrupted by) the first resolution.
		result2 := map[string]interface{}{}
		err = ApplyDeferredMerges(dctx, result2, cfg, processor)
		require.NoError(t, err)
		assert.Equal(t, "resolved", result2["config"])
	})

	// Regression test for a nondeterministic data-loss bug found by field-testing PR #2892
	// (github.com/cloudposse/atmos/issues/2888's fix): when a deferred function occupies a
	// PARENT path in one layer (e.g. "vars.combo": !template producing a map) while a DIFFERENT
	// deferred function occupies a CHILD key of that same path in a higher-precedence layer
	// (e.g. "vars.combo.nested": !template), the two paths get separate DeferredMergeContext
	// entries ("vars.combo" and "vars.combo.nested"). ApplyDeferredMerges previously iterated
	// dctx.GetDeferredValues() — a plain Go map — directly; Go randomizes map iteration order
	// per range, so "vars.combo" and "vars.combo.nested" could be processed in either order.
	// Each pathKey's resolution ends in an unconditional SetValueAtPath call that replaces
	// whatever currently exists at that exact path: if "vars.combo" (parent) is processed AFTER
	// "vars.combo.nested" (child), the parent's wholesale replace of the "combo" map silently
	// discards the child's already-resolved "nested" value. Live CLI reproduction against the
	// atmos-yaml-functions-merge fixture showed this failing ~40% of runs — not just discarding
	// data (final value nil) but sometimes leaking the child's raw, still-unresolved function
	// string straight into command output. Run many iterations here since a single run can pass
	// by chance depending on that random map order.
	t.Run("resolves deterministically when a parent path and a child path both defer functions", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				switch value {
				case "!template parent":
					return map[string]interface{}{"a": "1", "b": "2"}, nil
				case "!template child":
					return "nested-value", nil
				default:
					return value, nil
				}
			},
		}

		expected := map[string]interface{}{
			"a":      "1",
			"b":      "2",
			"nested": "nested-value",
			"plain":  "concrete",
		}

		const iterations = 200
		for i := 0; i < iterations; i++ {
			inputs := []map[string]any{
				{
					"vars": map[string]any{
						"combo": "!template parent",
					},
				},
				{
					"vars": map[string]any{
						"combo": map[string]any{
							"nested": "!template child",
							"plain":  "concrete",
						},
					},
				},
			}

			result, dctx, err := MergeWithDeferred(cfg, inputs)
			require.NoError(t, err)

			err = ApplyDeferredMerges(dctx, result, cfg, processor)
			require.NoError(t, err, "iteration %d", i)

			vars, ok := result["vars"].(map[string]any)
			require.True(t, ok, "iteration %d: vars should be a map", i)
			combo, ok := vars["combo"].(map[string]interface{})
			require.True(t, ok, "iteration %d: combo should be a map", i)

			assert.Equal(t, expected, combo, "iteration %d: parent function's map and child function's leaf must both survive the deep merge, in every iteration order", i)
		}
	})
}

// mockYAMLProcessor is a mock implementation of YAMLFunctionProcessor for testing.
type mockYAMLProcessor struct {
	processFunc func(value string) (any, error)
}

func (m *mockYAMLProcessor) ProcessYAMLFunctionString(value string) (any, error) {
	if m.processFunc != nil {
		return m.processFunc(value)
	}
	return value, nil
}

// TestApplyDeferredMerges_SingleContributionReuse pins the fix for the double-execution regression
// (docs/fixes/2026-08-13-deferred-merge-double-execution.md): when a deferred path has a single
// contributing layer and `result` already holds a resolved value there (as it does after the
// document-wide ProcessCustomYamlTags pass in exec.processStacks), ApplyDeferredMerges must NOT
// invoke the processor again — otherwise uncached side-effecting functions like !exec run twice.
func TestApplyDeferredMerges_SingleContributionReuse(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}

	newProcessor := func(calls *int) *mockYAMLProcessor {
		return &mockYAMLProcessor{processFunc: func(value string) (any, error) {
			*calls++
			// Stands in for a side-effecting function such as !exec. The `calls` counter is the
			// observable signal: a spurious second invocation increments it.
			return "resolved", nil
		}}
	}

	t.Run("does not re-invoke the processor when the single value is already resolved", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"vars", "foo"}, "!exec echo hi")

		// `result` mirrors what ProcessCustomYamlTags already produced for this no-collision path.
		result := map[string]any{"vars": map[string]any{"foo": "already-resolved"}}

		calls := 0
		require.NoError(t, ApplyDeferredMerges(dctx, result, cfg, newProcessor(&calls)))

		assert.Equal(t, 0, calls, "processor must not run again for an already-resolved single-contribution path")
		vars := result["vars"].(map[string]any)
		assert.Equal(t, "already-resolved", vars["foo"], "the earlier pass's resolved value must be preserved untouched")
	})

	t.Run("still resolves when result holds a nil placeholder (no prior pass)", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"vars", "foo"}, "!exec echo hi")

		// No prior resolution pass: the placeholder nil is what WalkAndDeferYAMLFunctions left.
		result := map[string]any{"vars": map[string]any{"foo": nil}}

		calls := 0
		require.NoError(t, ApplyDeferredMerges(dctx, result, cfg, newProcessor(&calls)))

		assert.Equal(t, 1, calls, "a real-processor caller with no prior resolution must still resolve exactly once")
		vars := result["vars"].(map[string]any)
		assert.Equal(t, "resolved", vars["foo"])
	})

	t.Run("still resolves when result holds the raw function string (no prior pass)", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"vars", "foo"}, "!exec echo hi")

		result := map[string]any{"vars": map[string]any{"foo": "!exec echo hi"}}

		calls := 0
		require.NoError(t, ApplyDeferredMerges(dctx, result, cfg, newProcessor(&calls)))

		assert.Equal(t, 1, calls, "an unresolved raw function string must not be mistaken for a resolved value")
		vars := result["vars"].(map[string]any)
		assert.Equal(t, "resolved", vars["foo"])
	})

	t.Run("still resolves a genuine collision so the #2888 deep-merge is unaffected", func(t *testing.T) {
		// Two competing layers at the same path: a concrete override (higher precedence) and a
		// deferred function (lower precedence). This is a real merge, so the processor MUST run.
		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"vars", "tags"}, "!labels") // precedence 0 (function).
		dctx.IncrementPrecedence()                            //nolint:wsl
		dctx.deferredValues["vars.tags"] = append(dctx.deferredValues["vars.tags"], &DeferredValue{
			Path:       []string{"vars", "tags"},
			Value:      map[string]any{"Z": "override"},
			Precedence: 1,
			IsFunction: false,
		})

		// Even though `result` has a resolved value, len > 1 means we must NOT take the reuse path.
		result := map[string]any{"vars": map[string]any{"tags": map[string]any{"Z": "override"}}}

		calls := 0
		proc := &mockYAMLProcessor{processFunc: func(value string) (any, error) {
			calls++
			return map[string]any{"X": "1", "Y": "2"}, nil
		}}
		require.NoError(t, ApplyDeferredMerges(dctx, result, cfg, proc))

		assert.Equal(t, 1, calls, "the !labels function must still be resolved to deep-merge against the concrete override")
		vars := result["vars"].(map[string]any)
		tags := vars["tags"].(map[string]any)
		assert.Equal(t, map[string]any{"X": "1", "Y": "2", "Z": "override"}, tags, "collision must deep-merge, preserving both the function's keys and the override")
	})
}
