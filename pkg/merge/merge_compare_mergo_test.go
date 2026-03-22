//go:build compare_mergo
// +build compare_mergo

// Package merge provides cross-validation tests that compare the native deep-merge
// implementation against dario.cat/mergo.  These tests are opt-in (behind the
// "compare_mergo" build tag) and are NOT run in CI.
//
// To run locally:
//
//	go test -tags compare_mergo ./pkg/merge/... -run TestCompareMergo -v
//
// The purpose of these tests is to document where native behavior MATCHES mergo
// and where it intentionally DIVERGES (defined contract).
// If they fail, it means either:
//   - A regression in the native implementation (unintentional divergence), or
//   - An expected divergence that should be explicitly annotated as "defined contract".
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
			name: "nil value in src map entry is skipped",
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

// TestCompareMergo_DefinedContractDivergences documents cases where native behavior
// intentionally DIFFERS from mergo.  These are our "defined contracts" — the expected
// behavior is the native implementation, not mergo.
func TestCompareMergo_DefinedContractDivergences(t *testing.T) {
	t.Run("sliceDeepCopy extra src elements are dropped (defined contract)", func(t *testing.T) {
		// Defined contract: when sliceDeepCopy=true, extra src elements beyond dst length
		// are ignored.  This differs from mergo.WithSliceDeepCopy which may extend the slice.
		// See: TestDeepMergeNative_SliceDeepCopy_ExtraSrcElementsIgnored
		cfg := &schema.AtmosConfiguration{}
		inputs := []map[string]any{
			{"list": []any{map[string]any{"id": 1}}},
			{"list": []any{map[string]any{"id": 2}, map[string]any{"id": 3}}},
		}
		result, err := MergeWithOptions(cfg, inputs, false, true)
		require.NoError(t, err)
		list, ok := result["list"].([]any)
		require.True(t, ok)
		assert.Len(t, list, 1,
			"defined contract: extra src elements are dropped in sliceDeepCopy mode")
		t.Logf("Defined contract: sliceDeepCopy drops extra src elements beyond dst len=%d", len(list))
	})
}
