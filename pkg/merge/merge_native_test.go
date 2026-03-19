package merge

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// ---------------------------------------------------------------------------
// Unit tests for safeAdd
// ---------------------------------------------------------------------------

func TestSafeAdd_Normal(t *testing.T) {
	assert.Equal(t, 5, safeAdd(2, 3))
	assert.Equal(t, 0, safeAdd(0, 0))
}

func TestSafeAdd_Overflow(t *testing.T) {
	// Adding two values that would overflow int should clamp to math.MaxInt.
	assert.Equal(t, math.MaxInt, safeAdd(math.MaxInt, 1))
	assert.Equal(t, math.MaxInt, safeAdd(math.MaxInt, math.MaxInt))
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

// TestMergeSlicesNative_TailElementsDeepCopied verifies that elements beyond len(src) in the
// result are deep copies of the corresponding dst elements, not aliases.
// Without deep-copying the tail, a subsequent merge pass could mutate shared inner maps.
func TestMergeSlicesNative_TailElementsDeepCopied(t *testing.T) {
	innerMap := map[string]any{"x": 1}
	dst := []any{map[string]any{"a": 1}, innerMap}
	src := []any{map[string]any{"b": 2}} // only one element; [1] is in the tail

	result, err := mergeSlicesNative(dst, src)
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

	result, err := mergeSlicesNative(dst, src)
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
		_, _ = Merge(cfg, inputs)
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
		_, _ = Merge(cfg, inputs)
	}
}

// ---------------------------------------------------------------------------
// Coverage tests for previously uncovered branches
// ---------------------------------------------------------------------------

// TestDeepMergeNative_TypedMapMergesWithMapDst covers lines 58-63:
// src contains a typed map (e.g. map[string]schema.Provider) that deepCopyValue
// normalises to map[string]any, while dst already has a map[string]any at the same
// key.  The two maps should be recursively merged, not replaced.
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
	result, err := mergeSlicesNative(dst, src)
	require.NoError(t, err)
	assert.Equal(t, "scalar-value", result[0], "dst scalar must be preserved when src[i] is a map")
}

// TestMergeSlicesNative_TypeMismatch_PropagatesError covers lines 158-160:
// When both slice elements are maps but an inner key has a type mismatch
// (slice vs non-slice), deepMergeNative returns an error that must propagate.
func TestMergeSlicesNative_TypeMismatch_PropagatesError(t *testing.T) {
	dst := []any{map[string]any{"subnets": []any{"10.0.1.0/24"}}}
	src := []any{map[string]any{"subnets": "10.0.100.0/24"}} // type mismatch: []any vs string
	_, err := mergeSlicesNative(dst, src)
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

// BenchmarkMergeNative_TenInputs measures merge performance for 10 inputs,
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
		_, _ = Merge(cfg, inputs)
	}
}
