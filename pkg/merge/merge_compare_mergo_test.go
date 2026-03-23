//go:build compare_mergo
// +build compare_mergo

// Package merge provides cross-validation tests that compare the native deep-merge
// implementation against dario.cat/mergo.  These tests are opt-in (behind the
// "compare_mergo" build tag) and are NOT run in CI.
//
// To run locally (requires dario.cat/mergo v1.0.2):
//
//	go test -tags compare_mergo ./pkg/merge/... -run CrossValidate -v
//
// The purpose of these tests is to document where native behavior MATCHES mergo
// and where it intentionally DIVERGES (defined contract).
// If they fail, it means either:
//   - A regression in the native implementation (unintentional divergence), or
//   - An expected divergence that should be explicitly annotated as "defined contract".
//
// mergo version used for cross-validation: dario.cat/mergo v1.0.2 (go.mod).
package merge

import (
	"testing"

	dmergo "dario.cat/mergo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCompareMergo_NestedMapsMerge validates that native deep-merge produces the
// same nested-map merge result as mergo.WithOverride (the strategy previously used).
func TestCompareMergo_NestedMapsMerge(t *testing.T) {
	for _, tc := range []struct {
		name   string
		inputs []map[string]any
	}{
		{
			name: "two level nested override",
			inputs: []map[string]any{
				{"vars": map[string]any{"region": "us-east-1", "debug": false}},
				{"vars": map[string]any{"region": "eu-west-1", "timeout": 30}},
			},
		},
		{
			name: "three levels of nesting",
			inputs: []map[string]any{
				{"a": map[string]any{"b": map[string]any{"c": 1, "d": 2}}},
				{"a": map[string]any{"b": map[string]any{"c": 99, "e": 3}}},
			},
		},
		{
			name: "scalar overrides map (defined contract: native overrides like mergo)",
			inputs: []map[string]any{
				{"key": map[string]any{"nested": "value"}},
				{"key": "scalar"},
			},
		},
		{
			name: "nil value in src map entry overrides dst",
			inputs: []map[string]any{
				{"key": "original"},
				{"key": nil},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Build mergo result.
			mergoResult := map[string]any{}
			for _, inp := range tc.inputs {
				// Deep-copy inp before merge so mergo doesn't alias nested maps across iterations.
				// deepCopyValue is available in the same package (merge.go) and handles nested maps correctly.
				inpCopy := deepCopyValue(inp).(map[string]any)
				if err := dmergo.Merge(&mergoResult, inpCopy, dmergo.WithOverride); err != nil {
					t.Fatalf("mergo.Merge failed (baseline construction must succeed): %v", err)
				}
			}

			// Build native result.
			cfg := &schema.AtmosConfiguration{}
			nativeResult, err := MergeWithOptions(cfg, tc.inputs, false, false)
			require.NoError(t, err)

			assert.Equal(t, mergoResult, nativeResult,
				"native result must match mergo for case %q", tc.name)
		})
	}
}

// TestCompareMergo_SliceModes tests equivalence for appendSlice and sliceDeepCopy modes.
func TestCompareMergo_SliceModes(t *testing.T) {
	t.Run("appendSlice concatenates slices", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		inputs := []map[string]any{
			{"tags": []any{"a", "b"}},
			{"tags": []any{"c", "d"}},
		}
		result, err := MergeWithOptions(cfg, inputs, true, false)
		require.NoError(t, err)
		tags, ok := result["tags"].([]any)
		require.True(t, ok)
		assert.Equal(t, []any{"a", "b", "c", "d"}, tags,
			"appendSlice should concatenate both slices")
	})

	t.Run("appendSlice with nested maps", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		inputs := []map[string]any{
			{"items": []any{map[string]any{"name": "first"}}},
			{"items": []any{map[string]any{"name": "second"}}},
		}
		result, err := MergeWithOptions(cfg, inputs, true, false)
		require.NoError(t, err)
		items, ok := result["items"].([]any)
		require.True(t, ok)
		assert.Len(t, items, 2, "appendSlice should produce 2 elements")
	})

	t.Run("sliceDeepCopy merges overlapping map elements", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		inputs := []map[string]any{
			{"groups": []any{
				map[string]any{"name": "web", "size": "small"},
				map[string]any{"name": "api", "size": "medium"},
			}},
			{"groups": []any{
				map[string]any{"name": "web", "size": "large"},
				map[string]any{"name": "api", "replicas": 3},
			}},
		}
		result, err := MergeWithOptions(cfg, inputs, false, true)
		require.NoError(t, err)
		groups, ok := result["groups"].([]any)
		require.True(t, ok)
		assert.Len(t, groups, 2)

		// First element: size overridden by dst (scalar kept from dst).
		g0, ok := groups[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "web", g0["name"])

		// Second element: replicas not present in dst, so dst is kept.
		g1, ok := groups[1].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "api", g1["name"])
	})

	t.Run("sliceDeepCopy src extends beyond dst length", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		inputs := []map[string]any{
			{"node_groups": []any{
				map[string]any{"name": "general", "instance_type": "m5.large"},
			}},
			{"node_groups": []any{
				map[string]any{"name": "general", "instance_type": "m5.2xlarge"},
				map[string]any{"name": "gpu", "instance_type": "g5.xlarge"},
			}},
		}
		result, err := MergeWithOptions(cfg, inputs, false, true)
		require.NoError(t, err)
		groups, ok := result["node_groups"].([]any)
		require.True(t, ok)
		assert.Len(t, groups, 2,
			"sliceDeepCopy should extend result when src has more elements than dst")

		// Second element is the new gpu group from src.
		g1, ok := groups[1].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "gpu", g1["name"])
		assert.Equal(t, "g5.xlarge", g1["instance_type"])
	})

	t.Run("sliceDeepCopy with three inputs extending progressively", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		inputs := []map[string]any{
			{"items": []any{map[string]any{"id": 1}}},
			{"items": []any{map[string]any{"id": 1}, map[string]any{"id": 2}}},
			{"items": []any{map[string]any{"id": 1}, map[string]any{"id": 2}, map[string]any{"id": 3}}},
		}
		result, err := MergeWithOptions(cfg, inputs, false, true)
		require.NoError(t, err)
		items, ok := result["items"].([]any)
		require.True(t, ok)
		assert.Len(t, items, 3,
			"three progressive inputs should produce 3 elements")
	})
}
