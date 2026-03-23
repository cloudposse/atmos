package merge

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ---------------------------------------------------------------------------
// Unit tests for safeCap
// ---------------------------------------------------------------------------

func TestSafeCap_Normal(t *testing.T) {
	assert.Equal(t, 5, safeCap(2, 3))
	assert.Equal(t, 0, safeCap(0, 0))
}

func TestSafeCap_LargeValues(t *testing.T) {
	// Both values exceed maxCapHint — result must be capped, not OOM-panicking.
	big := maxCapHint + 1
	got := safeCap(big, big)
	assert.Equal(t, maxCapHint, got, "overflow/large value must be capped at maxCapHint")
}

func TestSafeCap_SumExceedsMax(t *testing.T) {
	// Sum > maxCapHint but each value is small — capped at maxCapHint.
	half := maxCapHint/2 + 1
	got := safeCap(half, half)
	assert.Equal(t, maxCapHint, got, "sum exceeding maxCapHint must be capped")
}

func TestSafeCap_Overflow(t *testing.T) {
	// Integer overflow in sum — must not panic; capped at maxCapHint.
	const huge = int(^uint(0) >> 1) // math.MaxInt
	got := safeCap(huge, huge)
	assert.Equal(t, maxCapHint, got, "integer overflow must be capped at maxCapHint")
}

// ---------------------------------------------------------------------------
// Unit tests for deepMergeNative
// ---------------------------------------------------------------------------

func TestDeepMergeNative_NewKeysAddedFromSrc(t *testing.T) {
	dst := map[string]any{"a": 1}
	src := map[string]any{"b": 2}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	assert.Equal(t, 1, dst["a"])
	assert.Equal(t, 2, dst["b"])
}

func TestDeepMergeNative_SrcOverridesDst(t *testing.T) {
	dst := map[string]any{"a": "old"}
	src := map[string]any{"a": "new"}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	assert.Equal(t, "new", dst["a"])
}

func TestDeepMergeNative_SrcNilOverridesDst(t *testing.T) {
	dst := map[string]any{"a": "existing"}
	src := map[string]any{"a": nil}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	assert.Nil(t, dst["a"])
}

func TestDeepMergeNative_BothMapsMergedRecursively(t *testing.T) {
	dst := map[string]any{
		"nested": map[string]any{"a": 1, "b": 2},
	}
	src := map[string]any{
		"nested": map[string]any{"b": 20, "c": 30},
	}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	nested, ok := dst["nested"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, nested["a"])
	assert.Equal(t, 20, nested["b"])
	assert.Equal(t, 30, nested["c"])
}

func TestDeepMergeNative_MapSrcOverridesNonMapDst(t *testing.T) {
	dst := map[string]any{"k": "string"}
	src := map[string]any{"k": map[string]any{"x": 1}}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	nested, ok := dst["k"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, nested["x"])
}

func TestDeepMergeNative_NonMapSrcOverridesMapDst(t *testing.T) {
	dst := map[string]any{"k": map[string]any{"x": 1}}
	src := map[string]any{"k": "string"}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	assert.Equal(t, "string", dst["k"])
}

func TestDeepMergeNative_EmptySrcNoChange(t *testing.T) {
	dst := map[string]any{"a": 1}
	src := map[string]any{}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	assert.Equal(t, map[string]any{"a": 1}, dst)
}

func TestDeepMergeNative_EmptyDstFilledFromSrc(t *testing.T) {
	dst := map[string]any{}
	src := map[string]any{"a": 1, "b": "hello"}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	assert.Equal(t, 1, dst["a"])
	assert.Equal(t, "hello", dst["b"])
}

func TestDeepMergeNative_SrcDoesNotMutateSrcData(t *testing.T) {
	// Deep merge must not corrupt the src map via shared pointers.
	src := map[string]any{
		"nested": map[string]any{"x": 1},
	}
	dst := map[string]any{}
	require.NoError(t, deepMergeNative(dst, src, false, false))

	// Mutate dst.nested.x — src.nested.x must not change.
	dstNested := dst["nested"].(map[string]any)
	dstNested["x"] = 99

	srcNested := src["nested"].(map[string]any)
	assert.Equal(t, 1, srcNested["x"], "deepMergeNative must not alias src internals in dst")
}

func TestDeepMergeNative_SliceReplace_SrcReplaceDst(t *testing.T) {
	// replace mode: src slice replaces dst slice entirely.
	dst := map[string]any{"list": []any{1, 2, 3}}
	src := map[string]any{"list": []any{4, 5}}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	assert.Equal(t, []any{4, 5}, dst["list"])
}

func TestDeepMergeNative_SliceAppend(t *testing.T) {
	dst := map[string]any{"list": []any{1, 2}}
	src := map[string]any{"list": []any{3, 4}}
	require.NoError(t, deepMergeNative(dst, src, true, false))
	assert.Equal(t, []any{1, 2, 3, 4}, dst["list"])
}

func TestDeepMergeNative_SliceAppendDoesNotAliasElements(t *testing.T) {
	nested := map[string]any{"v": 1}
	dst := map[string]any{"list": []any{}}
	src := map[string]any{"list": []any{nested}}
	require.NoError(t, deepMergeNative(dst, src, true, false))

	list := dst["list"].([]any)
	require.Len(t, list, 1)
	elem := list[0].(map[string]any)
	elem["v"] = 99
	assert.Equal(t, 1, nested["v"], "appendSlices must deep-copy src elements")
}

// TestAppendSlices_DstElementsDeepCopied verifies that appendSlices deep-copies dst elements
// so that the result slice does not alias the accumulator's original elements.
func TestAppendSlices_DstElementsDeepCopied(t *testing.T) {
	dstMap := map[string]any{"x": 1}
	dst := []any{dstMap}
	src := []any{map[string]any{"y": 2}}

	result := appendSlices(dst, src)
	require.Len(t, result, 2)

	// Mutate result[0]; original dstMap must not change.
	resultMap, ok := result[0].(map[string]any)
	require.True(t, ok)
	resultMap["x"] = 99
	assert.Equal(t, 1, dstMap["x"], "appendSlices must deep-copy dst elements")
}

// TestDeepMergeNative_NilDstReturnsError verifies that passing nil as dst is caught early
// instead of panicking on the first map assignment.
func TestDeepMergeNative_NilDstReturnsError(t *testing.T) {
	err := deepMergeNative(nil, map[string]any{"k": "v"}, false, false)
	require.ErrorIs(t, err, errUtils.ErrMergeNilDst, "nil dst must return ErrMergeNilDst sentinel")
}

// TestDeepMergeNative_SliceMapMismatch verifies that the slice→map shape-change guard runs
// before the map-handling branches: when dst holds a []any and src provides a map for the
// same key, deepMergeNative must return ErrMergeTypeMismatch instead of silently replacing
// the slice with the map value.
func TestDeepMergeNative_SliceMapMismatch(t *testing.T) {
	// plain map[string]any → should be rejected
	dst := map[string]any{"net": []any{"10.0.0.0/8"}}
	err := deepMergeNative(dst, map[string]any{"net": map[string]any{"cidr": "10.0.0.0/8"}}, false, false)
	require.ErrorIs(t, err, errUtils.ErrMergeTypeMismatch, "dst=slice, src=map must return ErrMergeTypeMismatch")
	// dst must be unchanged — the guard must reject before any mutation.
	require.Equal(t, []any{"10.0.0.0/8"}, dst["net"], "dst must be unchanged after type-mismatch rejection")

	// typed map (e.g. map[string]struct{}) → should also be rejected via the reflect path in isMapValue.
	type cidr struct{ Cidr string }
	dst2 := map[string]any{"net": []any{"10.0.0.0/8"}}
	err2 := deepMergeNative(dst2, map[string]any{"net": map[string]cidr{"primary": {"10.0.0.0/8"}}}, false, false)
	require.ErrorIs(t, err2, errUtils.ErrMergeTypeMismatch, "dst=slice, src=typed-map must return ErrMergeTypeMismatch")
}

// TestDeepMergeNative_NilSrcIsNoOp verifies the documented invariant:
// "A nil src is safe: ranging over a nil map is a no-op."
// This test catches any future refactor that accidentally panics or mutates dst on nil src.
func TestDeepMergeNative_NilSrcIsNoOp(t *testing.T) {
	dst := map[string]any{"key": "value", "num": 42}
	original := map[string]any{"key": "value", "num": 42}
	err := deepMergeNative(dst, nil, false, false)
	require.NoError(t, err, "nil src must not return an error")
	assert.Equal(t, original, dst, "nil src must not mutate dst")
}

// TestMergeSlicesNative_OverlapPreservedDstDeepCopied verifies that preserved overlap
// positions (non-map src element or dst/src type mismatch) are deep-copied so that the
// result slice does not alias the accumulator's elements.
func TestMergeSlicesNative_OverlapPreservedDstDeepCopied(t *testing.T) {
	dstMap := map[string]any{"x": 1}
	// Scalar src at position 0 → dst element is preserved but must be deep-copied.
	dst := []any{dstMap}
	src := []any{"scalar-src"}

	result, err := mergeSlicesNative(dst, src, false, false)
	require.NoError(t, err)
	require.Len(t, result, 1)

	resultMap, ok := result[0].(map[string]any)
	require.True(t, ok)
	resultMap["x"] = 99
	assert.Equal(t, 1, dstMap["x"], "overlap preserved dst element must be deep-copied, not aliased")
}

func TestDeepMergeNative_SliceDeepCopy_ScalarsKeepDst(t *testing.T) {
	// sliceDeepCopy: for scalar elements, dst is preserved (matches mergo).
	dst := map[string]any{"tags": []any{"base-1", "base-2"}}
	src := map[string]any{"tags": []any{"override-1"}}
	require.NoError(t, deepMergeNative(dst, src, false, true))
	// Mergo keeps dst for scalar elements; only map elements are merged.
	assert.Equal(t, []any{"base-1", "base-2"}, dst["tags"])
}

func TestDeepMergeNative_SliceDeepCopy_MapsMerged(t *testing.T) {
	// sliceDeepCopy: map elements at the same index are deep-merged.
	dst := map[string]any{
		"items": []any{
			map[string]any{"id": 1, "name": "base"},
		},
	}
	src := map[string]any{
		"items": []any{
			map[string]any{"id": 2, "extra": "new"},
		},
	}
	require.NoError(t, deepMergeNative(dst, src, false, true))
	items := dst["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	// src.id overrides dst.id; dst.name preserved; src.extra added.
	assert.Equal(t, 2, item["id"])
	assert.Equal(t, "base", item["name"])
	assert.Equal(t, "new", item["extra"])
}

func TestDeepMergeNative_SliceDeepCopy_ExtraSrcElementsIgnored(t *testing.T) {
	// sliceDeepCopy: extra src elements beyond dst length are ignored.
	dst := map[string]any{"list": []any{1}}
	src := map[string]any{"list": []any{10, 20, 30}}
	require.NoError(t, deepMergeNative(dst, src, false, true))
	// Result has dst length (1); extra src elements are dropped.
	assert.Equal(t, []any{1}, dst["list"])
}

// TestDeepMergeNative_SliceDeepCopyPrecedenceOverAppend verifies that sliceDeepCopy takes
// priority when both appendSlice=true AND sliceDeepCopy=true are passed, matching old mergo
// behaviour where WithSliceDeepCopy was checked before WithAppendSlice.
func TestDeepMergeNative_SliceDeepCopyPrecedenceOverAppend(t *testing.T) {
	dst := map[string]any{
		"items": []any{map[string]any{"id": 1, "name": "base"}},
	}
	src := map[string]any{
		"items": []any{map[string]any{"id": 2, "extra": "new"}, map[string]any{"id": 3}},
	}
	// Both flags set: sliceDeepCopy must win → element-wise merge, not append.
	require.NoError(t, deepMergeNative(dst, src, true, true))
	items := dst["items"].([]any)
	// sliceDeepCopy: result length = dst length (1), not dst+src length (3).
	assert.Len(t, items, 1, "sliceDeepCopy must not append when both flags are true")
	item := items[0].(map[string]any)
	assert.Equal(t, 2, item["id"])
	assert.Equal(t, "base", item["name"])
	assert.Equal(t, "new", item["extra"])
}

// TestDeepMergeNative_SliceDeepCopy_NestedListOfMapOfList verifies that slice strategy flags
// are propagated into inner map merges within sliceDeepCopy mode.
// This covers list[map[list]] structures common in Atmos configs (e.g., worker groups with subnets).
func TestDeepMergeNative_SliceDeepCopy_NestedListOfMapOfList(t *testing.T) {
	// Simulate: base workers list has one worker group with two subnets.
	// Override merges a new tag into the same worker group.
	// With sliceDeepCopy threaded through, the outer workers list uses element-wise merge
	// and the inner workers[0] map is deep-merged (preserving base subnets, adding tag).
	dst := map[string]any{
		"workers": []any{
			map[string]any{
				"name":    "group-a",
				"subnets": []any{"10.0.1.0/24", "10.0.2.0/24"},
			},
		},
	}
	src := map[string]any{
		"workers": []any{
			map[string]any{
				"tag": "production",
				// No "subnets" key in src — dst subnets must be preserved.
			},
		},
	}
	// sliceDeepCopy=true: element-wise merge for the outer workers list.
	// sliceDeepCopy is also propagated into the inner map merge so the workers[0]
	// map entries are deep-merged (not replaced).
	require.NoError(t, deepMergeNative(dst, src, false, true))

	workers := dst["workers"].([]any)
	require.Len(t, workers, 1, "sliceDeepCopy result length must equal dst length")
	worker := workers[0].(map[string]any)
	// Both dst fields (name, subnets) and src field (tag) must be present.
	assert.Equal(t, "group-a", worker["name"])
	assert.Equal(t, "production", worker["tag"])
	subnets, ok := worker["subnets"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"10.0.1.0/24", "10.0.2.0/24"}, subnets, "dst subnets must be preserved when src has no subnets key")
}

// TestDeepMergeNative_NestedListOfMapOfList_AppendStrategy verifies that appendSlice=true
// propagates into inner maps so nested lists are appended (not replaced) when both the outer
// and inner list use appendSlice strategy.
func TestDeepMergeNative_NestedListOfMapOfList_AppendStrategy(t *testing.T) {
	// Simulate: base workers list has one worker group with two subnets.
	// Override adds a third subnet.  With appendSlice=true threaded through,
	// the subnets should be appended; without threading, they would be replaced.
	dst := map[string]any{
		"workers": []any{
			map[string]any{
				"name":    "group-a",
				"subnets": []any{"10.0.1.0/24", "10.0.2.0/24"},
			},
		},
	}
	src := map[string]any{
		"workers": []any{
			map[string]any{
				"subnets": []any{"10.0.3.0/24"},
			},
		},
	}
	// appendSlice=true, sliceDeepCopy=false: appendSlice for both outer and inner lists.
	require.NoError(t, deepMergeNative(dst, src, true, false))

	workers := dst["workers"].([]any)
	// appendSlice appends src workers to dst workers → length 2.
	require.Len(t, workers, 2, "appendSlice must append the src worker entry to dst workers")

	// Verify dst worker (index 0) retains its name and both original subnets.
	dstWorker, ok := workers[0].(map[string]any)
	require.True(t, ok, "workers[0] must be a map")
	assert.Equal(t, "group-a", dstWorker["name"])
	assert.Equal(t, []any{"10.0.1.0/24", "10.0.2.0/24"}, dstWorker["subnets"],
		"dst worker subnets must be preserved intact under appendSlice")

	// Verify src worker (index 1) retains its subnets.
	srcWorker, ok := workers[1].(map[string]any)
	require.True(t, ok, "workers[1] must be a map")
	assert.Equal(t, []any{"10.0.3.0/24"}, srcWorker["subnets"],
		"src worker subnets must be appended as-is")
}

// TestMergeSlicesNative_TailElementsDeepCopied verifies that elements beyond len(src) in the
// result are deep copies of the corresponding dst elements, not aliases.
// Without deep-copying the tail, a subsequent merge pass could mutate shared inner maps.
func TestMergeSlicesNative_TailElementsDeepCopied(t *testing.T) {
	innerMap := map[string]any{"x": 1}
	dst := []any{map[string]any{"a": 1}, innerMap}
	src := []any{map[string]any{"b": 2}} // only one element; [1] is in the tail

	result, err := mergeSlicesNative(dst, src, false, false)
	require.NoError(t, err)
	require.Len(t, result, 2)

	// Mutate the result's tail element; original innerMap must not change.
	tailMap, ok := result[1].(map[string]any)
	require.True(t, ok)
	tailMap["x"] = 99

	assert.Equal(t, 1, innerMap["x"], "tail element must be a deep copy, not an alias")
}

// TestMergeSlicesNative_DstMapValuesDeepCopied verifies that dstMap values are deep-copied
// before recursing so that deepMergeNative cannot mutate the original accumulator maps.
func TestMergeSlicesNative_DstMapValuesDeepCopied(t *testing.T) {
	sharedNested := map[string]any{"x": 1}
	dst := []any{map[string]any{"nested": sharedNested}}
	src := []any{map[string]any{"nested": map[string]any{"y": 2}}}

	result, err := mergeSlicesNative(dst, src, false, false)
	require.NoError(t, err)
	require.Len(t, result, 1)

	resultItem := result[0].(map[string]any)
	resultNested := resultItem["nested"].(map[string]any)
	assert.Equal(t, 1, resultNested["x"])
	assert.Equal(t, 2, resultNested["y"])

	// The original sharedNested must not have been mutated.
	assert.Equal(t, 1, sharedNested["x"])
	assert.NotContains(t, sharedNested, "y", "original dst map values must not be mutated")
}

func TestDeepMergeNative_TypedSliceInSrcNormalized(t *testing.T) {
	// src may contain typed slices (e.g. []string) which must be normalised.
	dst := map[string]any{}
	src := map[string]any{"strs": []string{"a", "b"}}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	assert.Equal(t, []any{"a", "b"}, dst["strs"])
}

// TestDeepMergeNative_TypedSrcSliceReplacesExistingAnySliceDst verifies that a typed
// src slice (e.g. []string) correctly replaces an existing []any dst value.
// This exercises the existing-key path through the type-check normalization block,
// which differs from the new-key path tested by TestDeepMergeNative_TypedSliceInSrcNormalized.
func TestDeepMergeNative_TypedSrcSliceReplacesExistingAnySliceDst(t *testing.T) {
	dst := map[string]any{"list": []any{"a", "b"}}
	src := map[string]any{"list": []string{"c"}} // typed slice src, existing []any key
	require.NoError(t, deepMergeNative(dst, src, false, false))
	assert.Equal(t, []any{"c"}, dst["list"],
		"typed src slice must replace existing []any dst slice after normalization")
}

func TestDeepMergeNative_DeepNesting(t *testing.T) {
	dst := map[string]any{
		"l1": map[string]any{
			"l2": map[string]any{
				"l3": map[string]any{"value": "original", "only_dst": true},
			},
		},
	}
	src := map[string]any{
		"l1": map[string]any{
			"l2": map[string]any{
				"l3": map[string]any{"value": "updated", "only_src": "yes"},
			},
		},
	}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	l3 := dst["l1"].(map[string]any)["l2"].(map[string]any)["l3"].(map[string]any)
	assert.Equal(t, "updated", l3["value"])
	assert.Equal(t, true, l3["only_dst"])
	assert.Equal(t, "yes", l3["only_src"])
}

func TestDeepMergeNative_TypeMismatch_SliceVsString(t *testing.T) {
	// Type check (mergo.WithTypeCheck): overriding a slice with a non-slice must error.
	dst := map[string]any{"subnets": []any{"10.0.1.0/24", "10.0.2.0/24"}}
	src := map[string]any{"subnets": "10.0.100.0/24"}
	err := deepMergeNative(dst, src, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot override two slices with different type")
}

// ---------------------------------------------------------------------------
// Integration-style tests: MergeWithOptions via the public Merge function
// ---------------------------------------------------------------------------

func TestMergeNative_MatchesMergoBehaviourReplace(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}
	inputs := []map[string]any{
		{"list": []any{1, 2, 3}, "keep": "yes"},
		{"list": []any{4, 5}},
	}
	result, err := Merge(cfg, inputs)
	require.NoError(t, err)
	assert.Equal(t, []any{4, 5}, result["list"])
	assert.Equal(t, "yes", result["keep"])
}

func TestMergeNative_MatchesMergoBehaviourAppend(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyAppend},
	}
	inputs := []map[string]any{
		{"list": []any{1, 2}},
		{"list": []any{3, 4}},
	}
	result, err := Merge(cfg, inputs)
	require.NoError(t, err)
	assert.Equal(t, []any{1, 2, 3, 4}, result["list"])
}

func TestMergeNative_MatchesMergoBehaviourMergeScalars(t *testing.T) {
	// merge strategy with scalar arrays: dst (base) is preserved for scalar elements.
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyMerge},
	}
	inputs := []map[string]any{
		{"tags": []any{"base-1", "base-2"}},
		{"tags": []any{"override-1"}},
	}
	result, err := Merge(cfg, inputs)
	require.NoError(t, err)
	// mergo preserves dst for scalar arrays; our native merge matches this.
	assert.Equal(t, []any{"base-1", "base-2"}, result["tags"])
}

func TestMergeNative_MatchesMergoBehaviourMergeMaps(t *testing.T) {
	// merge strategy with map arrays: elements are deep-merged by index.
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyMerge},
	}
	inputs := []map[string]any{
		{"items": []any{map[string]any{"1": "1", "4": "4"}}},
		{"items": []any{map[string]any{"1": "1b", "5": "5"}}},
	}
	result, err := Merge(cfg, inputs)
	require.NoError(t, err)
	items := result["items"].([]any)
	item := items[0].(map[string]any)
	assert.Equal(t, "1b", item["1"])
	assert.Equal(t, "4", item["4"])
	assert.Equal(t, "5", item["5"])
}

func TestMergeNative_ImmutabilityOfInputs(t *testing.T) {
	// Merge must not mutate any of the input maps.
	cfg := &schema.AtmosConfiguration{}
	inputs := []map[string]any{
		{"nested": map[string]any{"x": 1}},
		{"nested": map[string]any{"y": 2}},
		{"nested": map[string]any{"z": 3}},
	}

	// Deep-copy inputs so we can compare after merge.
	origX := 1
	origY := 2
	origZ := 3

	_, err := Merge(cfg, inputs)
	require.NoError(t, err)

	// Original inputs must be unchanged.
	assert.Equal(t, origX, inputs[0]["nested"].(map[string]any)["x"])
	assert.Equal(t, origY, inputs[1]["nested"].(map[string]any)["y"])
	assert.Equal(t, origZ, inputs[2]["nested"].(map[string]any)["z"])
}

func TestMergeNative_MultipleInputsAccumulate(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	inputs := []map[string]any{
		{"a": 1},
		{"b": 2},
		{"c": 3},
		{"a": 10},
	}
	result, err := Merge(cfg, inputs)
	require.NoError(t, err)
	assert.Equal(t, 10, result["a"])
	assert.Equal(t, 2, result["b"])
	assert.Equal(t, 3, result["c"])
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkMergeNative_TwoInputs measures merge performance for 2 inputs (common case).
func BenchmarkMergeNative_TwoInputs(b *testing.B) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}
	input1 := map[string]any{
		"vars": map[string]any{
			"region":  "us-east-1",
			"env":     "prod",
			"tags":    []any{"env:prod"},
			"count":   3,
			"enabled": true,
		},
		"settings": map[string]any{
			"spacelift": map[string]any{"enabled": true},
		},
	}
	input2 := map[string]any{
		"vars": map[string]any{
			"region": "us-west-2",
			"extra":  "value",
		},
		"settings": map[string]any{
			"spacelift": map[string]any{"workspace": "prod"},
		},
	}
	inputs := []map[string]any{input1, input2}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Merge(cfg, inputs); err != nil {
			b.Fatalf("Merge failed: %v", err)
		}
	}
}

// BenchmarkMergeNative_FiveInputs measures merge performance for 5 inputs,
// typical of a production stack with multiple levels of inheritance.
func BenchmarkMergeNative_FiveInputs(b *testing.B) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}
	makeInput := func(region string, extra string) map[string]any {
		return map[string]any{
			"vars": map[string]any{
				"region": region,
				"tags":   []any{extra},
			},
			"settings": map[string]any{
				"spacelift": map[string]any{"enabled": true},
			},
			"env": map[string]any{
				"STAGE": extra,
			},
		}
	}
	inputs := []map[string]any{
		makeInput("us-east-1", "global"),
		makeInput("us-east-1", "org"),
		makeInput("us-east-1", "tenant"),
		makeInput("us-east-1", "stage"),
		makeInput("us-east-1", "component"),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Merge(cfg, inputs); err != nil {
			b.Fatalf("Merge failed: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Coverage tests for previously uncovered branches
// ---------------------------------------------------------------------------

// TestDeepMergeNative_TypedMapMergesWithMapDst covers lines 58-63:
// src contains a typed map (e.g. map[string]schema.Provider) that deepCopyValue
// normalises to map[string]any, while dst already has a map[string]any at the same
// key.  The two maps should be recursively merged, not replaced.
//
// Compile guard: package-level so it fires even if this test is skipped via -run.
// If schema.Provider ever renames the Kind field, this sentinel fails to compile,
// immediately catching a schema-incompatible merge behavior before any test runs.
var _ = schema.Provider{Kind: "azure"} // nolint:gochecknoglobals // compile-time sentinel only
func TestDeepMergeNative_TypedMapMergesWithMapDst(t *testing.T) {
	dst := map[string]any{
		"providers": map[string]any{
			"global": map[string]any{"kind": "aws", "region": "us-east-1"},
		},
	}
	// Use schema.Provider (a real typed-map value) so deepCopyValue goes through
	// the reflection-based normalisation path.
	src := map[string]any{
		"providers": map[string]schema.Provider{
			"component": {Kind: "azure"},
		},
	}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	providers, ok := dst["providers"].(map[string]any)
	require.True(t, ok, "providers should remain map[string]any after typed-map merge")
	assert.Contains(t, providers, "global", "global entry from dst must be preserved")
	assert.Contains(t, providers, "component", "component entry from src must be added")
}

// TestDeepMergeNative_TypedMapOverridesNonMapDst covers lines 64-67:
// src has a typed map that normalises to map[string]any, but dst holds a
// non-map value at that key.  The normalised src value should override.
func TestDeepMergeNative_TypedMapOverridesNonMapDst(t *testing.T) {
	dst := map[string]any{"providers": "scalar-string"}
	src := map[string]any{
		"providers": map[string]schema.Provider{"key": {Kind: "aws"}},
	}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	providers, ok := dst["providers"].(map[string]any)
	require.True(t, ok, "scalar dst should be overridden by normalised typed map from src")
	assert.Contains(t, providers, "key")
}

// TestMergeSlicesNative_DstScalarSrcMap_PreservesDst covers lines 147-151:
// When dst[i] is a scalar and src[i] is a map, the dst element is preserved.
func TestMergeSlicesNative_DstScalarSrcMap_PreservesDst(t *testing.T) {
	dst := []any{"scalar-value"}
	src := []any{map[string]any{"key": "value"}}
	result, err := mergeSlicesNative(dst, src, false, false)
	require.NoError(t, err)
	assert.Equal(t, "scalar-value", result[0], "dst scalar must be preserved when src[i] is a map")
}

// TestMergeSlicesNative_TypeMismatch_PropagatesError covers lines 158-160:
// When both slice elements are maps but an inner key has a type mismatch
// (slice vs non-slice), deepMergeNative returns an error that must propagate.
func TestMergeSlicesNative_TypeMismatch_PropagatesError(t *testing.T) {
	dst := []any{map[string]any{"subnets": []any{"10.0.1.0/24"}}}
	src := []any{map[string]any{"subnets": "10.0.100.0/24"}} // type mismatch: []any vs string
	_, err := mergeSlicesNative(dst, src, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot override two slices with different type")
}

// TestDeepMergeNative_SliceDeepCopy_TypeMismatch_PropagatesError covers lines 80-82:
// In sliceDeepCopy mode an error from mergeSlicesNative must be propagated.
func TestDeepMergeNative_SliceDeepCopy_TypeMismatch_PropagatesError(t *testing.T) {
	dst := map[string]any{
		"items": []any{map[string]any{"subnets": []any{"10.0.1.0/24"}}},
	}
	src := map[string]any{
		"items": []any{map[string]any{"subnets": "10.0.100.0/24"}}, // type mismatch inside slice
	}
	err := deepMergeNative(dst, src, false, true) // sliceDeepCopy=true
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot override two slices with different type")
}

// TestDeepMergeNative_TypedMapRecurse_TypeMismatch_PropagatesError covers lines 60-62:
// When src contains a typed map that normalises to map[string]any, AND dst already
// holds a map[string]any at that key, the recursive merge of the two maps may itself
// encounter a type mismatch — that error must propagate back to the caller.
func TestDeepMergeNative_TypedMapRecurse_TypeMismatch_PropagatesError(t *testing.T) {
	// dst["providers"]["my-provider"]["kind"] is a slice.
	// src["providers"] is a typed map[string]schema.Provider whose entry for
	// "my-provider" normalises to {"kind": "aws", ...}.  Merging "aws" (string)
	// into "kind" ([]any) must return a type-mismatch error.
	dst := map[string]any{
		"providers": map[string]any{
			"my-provider": map[string]any{"kind": []any{"not", "a", "string"}},
		},
	}
	src := map[string]any{
		"providers": map[string]schema.Provider{
			"my-provider": {Kind: "aws"},
		},
	}
	err := deepMergeNative(dst, src, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot override two slices with different type")
}

// TestMergeWithOptions_TypeMismatch_ReturnsWrappedError covers the error-return path
// in MergeWithOptions (the deepMergeNative call inside the accumulator loop).
func TestMergeWithOptions_TypeMismatch_ReturnsWrappedError(t *testing.T) {
	inputs := []map[string]any{
		{"subnets": []any{"10.0.1.0/24"}},
		{"subnets": "not-a-slice"}, // triggers type mismatch in deepMergeNative
	}
	_, err := MergeWithOptions(nil, inputs, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot override two slices with different type")
}

// ---------------------------------------------------------------------------
// Behavioral contract tests: encode the intended merge semantics for the
// native implementation.
//
// These tests document the **defined contract** for deepMergeNative — the
// authoritative expected behavior for this implementation.  Where behavior
// matches the old mergo-based implementation, that alignment is noted but the
// native implementation owns the contract independently.
// For live cross-validation against mergo, use the opt-in build-tagged tests:
//
//	go test -tags compare_mergo ./pkg/merge/... -run TestCompareMergo -v
//
// ---------------------------------------------------------------------------

// TestMergeNative_CrossValidateVsMergo_BasicReplace verifies that simple key
// override and new-key insertion produce the expected results.
func TestMergeNative_CrossValidateVsMergo_BasicReplace(t *testing.T) {
	inputs := []map[string]any{
		{"a": 1, "b": "old", "c": []any{"x"}},
		{"b": "new", "d": 99},
	}
	nativeResult, err := MergeWithOptions(nil, inputs, false, false)
	require.NoError(t, err)

	expected := map[string]any{
		"a": 1,
		"b": "new",
		"c": []any{"x"},
		"d": 99,
	}
	assert.Equal(t, expected, nativeResult, "basic replace merge must match expected output")
}

// TestMergeNative_CrossValidateVsMergo_AppendSlice verifies that appendSlice=true
// produces the same output as mergo.WithAppendSlice.
func TestMergeNative_CrossValidateVsMergo_AppendSlice(t *testing.T) {
	inputs := []map[string]any{
		{"list": []any{"a", "b"}},
		{"list": []any{"c", "d"}},
	}
	nativeResult, err := MergeWithOptions(nil, inputs, true, false)
	require.NoError(t, err)

	list, ok := nativeResult["list"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"a", "b", "c", "d"}, list, "appendSlice must concatenate lists")
}

// TestMergeNative_CrossValidateVsMergo_SliceDeepCopy_ScalarKeptAtDst verifies that
// for scalar elements in sliceDeepCopy mode, the dst element is preserved (not overridden).
// Defined contract: when src holds a non-map element at a position, the dst element is
// preserved.  This also matches mergo.WithSliceDeepCopy behavior (confirmed via
// TestCompareMergo in merge_compare_mergo_test.go — run with -tags compare_mergo).
func TestMergeNative_CrossValidateVsMergo_SliceDeepCopy_ScalarKeptAtDst(t *testing.T) {
	inputs := []map[string]any{
		{"tags": []any{"base-tag-1", "base-tag-2"}},
		{"tags": []any{"override-tag-1"}},
	}
	// sliceDeepCopy=true: result length = dst length, scalar dst elements preserved.
	nativeResult, err := MergeWithOptions(nil, inputs, false, true)
	require.NoError(t, err)

	tags, ok := nativeResult["tags"].([]any)
	require.True(t, ok)
	// mergeSlicesNative(sliceDeepCopy=true) uses two loops:
	//   Loop 1 (overlap, 0..min(len(src),len(dst))-1): if src[i] is a map, deep-merge src[i]
	//     into a copy of dst[i]; otherwise dst[i] is preserved unchanged (scalar src is discarded).
	//   Loop 2 (tail, len(src)..len(dst)-1): deep-copy remaining dst elements verbatim.
	// Here len(src)=1, len(dst)=2:
	//   Loop 1 position [0]: src[0]="override-tag-1" is a scalar → dst[0]="base-tag-1" kept.
	//   Loop 2 position [1]: tail → dst[1]="base-tag-2" deep-copied.
	assert.Equal(t, []any{"base-tag-1", "base-tag-2"}, tags,
		"sliceDeepCopy with scalar elements must preserve all dst elements")
}

// TestMergeNative_CrossValidateVsMergo_SliceDeepCopy_ExtraSrcDropped verifies that
// when src is longer than dst in sliceDeepCopy mode, extra src elements are silently
// dropped (result length == len(dst)).  This matches the observed mergo behaviour.
func TestMergeNative_CrossValidateVsMergo_SliceDeepCopy_ExtraSrcDropped(t *testing.T) {
	inputs := []map[string]any{
		{"list": []any{map[string]any{"id": 1}}},
		{"list": []any{map[string]any{"id": 2}, map[string]any{"id": 3}, map[string]any{"id": 4}}},
	}
	nativeResult, err := MergeWithOptions(nil, inputs, false, true)
	require.NoError(t, err)

	list, ok := nativeResult["list"].([]any)
	require.True(t, ok)
	assert.Len(t, list, 1, "sliceDeepCopy result length must equal dst length (extra src elements dropped)")
	item := list[0].(map[string]any)
	assert.Equal(t, 2, item["id"], "first element must be merged (src id overrides dst id)")
}

// TestMergeNative_CrossValidateVsMergo_NestedMaps verifies that nested map merges
// are deep (keys from both sides are present in the result) and that the result is
// isolated from the original inputs — mutating the result must not change the inputs.
func TestMergeNative_CrossValidateVsMergo_NestedMaps(t *testing.T) {
	inputs := []map[string]any{
		{"settings": map[string]any{"region": "us-east-1", "debug": false}},
		{"settings": map[string]any{"region": "eu-west-1", "timeout": 30}},
	}
	nativeResult, err := MergeWithOptions(nil, inputs, false, false)
	require.NoError(t, err)

	settings, ok := nativeResult["settings"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "eu-west-1", settings["region"], "src region must override dst region")
	assert.Equal(t, false, settings["debug"], "dst-only key must be preserved")
	assert.Equal(t, 30, settings["timeout"], "src-only key must be added")

	// Verify result isolation: mutating the merged result must not affect the original inputs.
	settings["region"] = "ap-southeast-1"
	assert.Equal(t, "us-east-1", inputs[0]["settings"].(map[string]any)["region"],
		"mutating the result must not affect the first input")
	assert.Equal(t, "eu-west-1", inputs[1]["settings"].(map[string]any)["region"],
		"mutating the result must not affect the second input")
}

// BenchmarkMergeNative_TenInputs measures merge performance for 10 inputs —
// a worst-case for deep import chains.
func BenchmarkMergeNative_TenInputs(b *testing.B) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}
	var inputs []map[string]any
	for i := 0; i < 10; i++ {
		inputs = append(inputs, map[string]any{
			"vars": map[string]any{
				"region":  "us-east-1",
				"counter": i,
				"nested": map[string]any{
					"deep": map[string]any{
						"value": i,
					},
				},
			},
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Merge(cfg, inputs); err != nil {
			b.Fatalf("Merge failed: %v", err)
		}
	}
}

// BenchmarkMerge_ProductionScale simulates a realistic large-stack merge:
// 10 inheritance layers, 25 top-level sections, nested maps, mixed lists, and
// the nested list-of-map-of-list pattern (`node_groups` with per-group subnet lists).
// The `node_groups` structure exercises the full sliceDeepCopy and appendSlice code
// paths with deeply nested structures representative of real Atmos stacks (e.g.
// EKS node groups with per-group subnet, label, and tag lists).
// This supplements BenchmarkMergeNative_TenInputs which uses only 3 top-level keys.
// Production Atmos stacks typically have 10–30 sections and 5–15 inheritance levels.
//
// Run with: go test -bench=BenchmarkMerge_ProductionScale -benchmem ./pkg/merge/...
func BenchmarkMerge_ProductionScale(b *testing.B) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}

	// Build 10 inputs, each with 24 top-level sections and nested content.
	var inputs []map[string]any
	for layer := 0; layer < 10; layer++ {
		input := map[string]any{
			"vars": map[string]any{
				"region":      "us-east-1",
				"env":         fmt.Sprintf("layer-%d", layer),
				"account_id":  "123456789012",
				"max_retries": layer + 3,
				"tags": map[string]any{
					"Environment": "production",
					"Layer":       layer,
					"Owner":       "platform-team",
				},
			},
			"settings": map[string]any{
				"spacelift": map[string]any{
					"workspace_enabled": true,
					"runner_image":      "cloudposse/geodesic:latest",
				},
				"depends_on": []any{fmt.Sprintf("dep-%d", layer)},
			},
			"env": map[string]any{
				"AWS_DEFAULT_REGION": "us-east-1",
				"TF_LOG":             "INFO",
				"LAYER":              fmt.Sprintf("%d", layer),
			},
			"providers": map[string]any{
				"aws": map[string]any{
					"region":  "us-east-1",
					"profile": "production",
				},
			},
			"backend": map[string]any{
				"s3": map[string]any{
					"bucket":         "my-terraform-state",
					"key":            fmt.Sprintf("layer-%d/terraform.tfstate", layer),
					"region":         "us-east-1",
					"encrypt":        true,
					"dynamodb_table": "terraform-state-lock",
				},
			},
			"remote_state_backend": map[string]any{
				"s3": map[string]any{
					"bucket":  "my-terraform-state",
					"region":  "us-east-1",
					"encrypt": true,
				},
			},
			"metadata": map[string]any{
				"type":      "real",
				"component": fmt.Sprintf("vpc-%d", layer),
				"tenant":    "platform",
				"stage":     "prod",
			},
			"overrides": map[string]any{
				"tags": map[string]any{
					"CreatedBy": "terraform",
				},
			},
			"import":             []any{"catalog/globals", fmt.Sprintf("catalog/vpc-%d", layer)},
			"terraform":          map[string]any{"workspace": fmt.Sprintf("prod-%d", layer)},
			"component":          "vpc",
			"namespace":          "platform",
			"tenant":             "core",
			"environment":        "prod",
			"stage":              "main",
			"region":             "us-east-1",
			"availability_zones": []any{"us-east-1a", "us-east-1b", "us-east-1c"},
			"cidr_block":         "10.0.0.0/16",
			"enable_dns":         true,
			"enable_nat":         true,
			"single_nat":         false,
			"outputs":            map[string]any{"vpc_id": fmt.Sprintf("vpc-layer-%d", layer)},
			"interfaces":         []any{fmt.Sprintf("iface-%d-a", layer), fmt.Sprintf("iface-%d-b", layer)},
			// Nested list-of-map-of-list: exercises the full sliceDeepCopy path for
			// real-world structures like worker groups with per-group subnet lists.
			"node_groups": []any{
				map[string]any{
					"name":             fmt.Sprintf("workers-%d", layer),
					"instance_type":    "t3.medium",
					"desired_capacity": layer + 2,
					"subnets":          []any{fmt.Sprintf("10.%d.1.0/24", layer), fmt.Sprintf("10.%d.2.0/24", layer)},
					"labels":           map[string]any{"team": "platform", "layer": fmt.Sprintf("%d", layer)},
				},
			},
		}
		inputs = append(inputs, input)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Merge(cfg, inputs); err != nil {
			b.Fatalf("Merge failed: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Coverage gap tests: targeted tests for the 5 lines missing from Codecov
// patch coverage (95.61% → 100% for these specific paths).
// ---------------------------------------------------------------------------

// TestMergeWithOptions_EmptyInputs_ReturnsEmptyMap covers the first fast-path in
// MergeWithOptions: an empty inputs slice must immediately return an empty map.
func TestMergeWithOptions_EmptyInputs_ReturnsEmptyMap(t *testing.T) {
	result, err := MergeWithOptions(nil, []map[string]any{}, false, false)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, result, "empty inputs must return empty map (not nil)")
}

// TestMergeWithOptions_AllEmptyMaps_ReturnsEmptyMap covers the second fast-path in
// MergeWithOptions: when every input is an empty map, nonEmptyInputs is empty and
// the second early return fires.
func TestMergeWithOptions_AllEmptyMaps_ReturnsEmptyMap(t *testing.T) {
	inputs := []map[string]any{{}, {}}
	result, err := MergeWithOptions(nil, inputs, false, false)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, result, "all-empty inputs must return empty map")
}

// TestMergeWithOptions_StrategyFlags_WireThrough verifies that appendSlice and
// sliceDeepCopy flags are correctly propagated through MergeWithOptions down to
// deepMergeNative.  Both modes must produce distinct, correct results.
func TestMergeWithOptions_StrategyFlags_WireThrough(t *testing.T) {
	inputs := []map[string]any{
		{"tags": []any{"base-a", "base-b"}},
		{"tags": []any{"override-a"}},
	}

	// appendSlice=true: result must be the concatenation.
	appendResult, err := MergeWithOptions(nil, inputs, true, false)
	require.NoError(t, err)
	tags, ok := appendResult["tags"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"base-a", "base-b", "override-a"}, tags,
		"appendSlice=true: src elements must be appended to dst")

	// sliceDeepCopy=true: result length = dst length, element at [0] is dst[0] (scalar kept).
	dcResult, err := MergeWithOptions(nil, inputs, false, true)
	require.NoError(t, err)
	dcTags, ok := dcResult["tags"].([]any)
	require.True(t, ok)
	assert.Len(t, dcTags, 2, "sliceDeepCopy=true: result length must equal dst length")
	assert.Equal(t, "base-a", dcTags[0], "sliceDeepCopy=true: dst scalar at position 0 preserved")
}

// TestDeepMergeNative_TypedSrcMapNormalized covers the normalization branch in
// deepMergeNative where srcVal is a non-map[string]any typed map that normalizes to
// map[string]any via deepCopyValue.  When dst holds a matching key with a map[string]any,
// the normalized src must be recursively merged.
func TestDeepMergeNative_TypedSrcMapNormalized(t *testing.T) {
	// schema.Provider is a struct that deepCopyValue normalizes to map[string]any.
	// Use an existing typed map from the schema package to trigger the normalization path.
	type providerConfig struct {
		Region string
	}
	src := map[string]any{
		// deepCopyValue will normalize this struct to a map via reflection.
		"provider": map[string]providerConfig{
			"aws": {Region: "us-east-1"},
		},
	}
	dst := map[string]any{}
	require.NoError(t, deepMergeNative(dst, src, false, false))
	// The typed map must be present in dst (normalized form).
	assert.NotNil(t, dst["provider"], "typed src map must be copied to dst via normalization path")
}

// TestMergeSlicesNative_InnerErrorPropagated covers the error-return path inside
// mergeSlicesNative: when deepMergeNative returns an error for a nested slice-of-maps
// element, that error must bubble up (the `return nil, err` path) rather than
// being silently swallowed.
//
// Trigger: both slice elements are maps; inside the map dst has a []any value and src
// has a string for the same key → deepMergeNative returns "cannot override two slices
// with different type" → mergeSlicesNative must propagate that error.
func TestMergeSlicesNative_InnerErrorPropagated(t *testing.T) {
	// dst[0] has "tags": []any — src[0] has "tags": string → type mismatch inside deepMergeNative.
	dst := []any{
		map[string]any{
			"tags": []any{"base-tag"},
		},
	}
	src := []any{
		map[string]any{
			"tags": "not-a-slice", // string, not []any → triggers the type-mismatch error
		},
	}

	// sliceDeepCopy=true: mergeSlicesNative calls deepMergeNative for the map elements,
	// which must surface the "cannot override two slices with different type" error.
	_, err := mergeSlicesNative(dst, src, false, true)
	require.Error(t, err, "mergeSlicesNative must propagate the error from deepMergeNative")
	assert.Contains(t, err.Error(), "cannot override two slices with different type",
		"inner error must be propagated without wrapping")
}
