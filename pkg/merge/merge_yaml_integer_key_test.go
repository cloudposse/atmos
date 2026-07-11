package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestDeepMergeNative_YAMLIntegerMapKey reproduces cloudposse/atmos#2376: a nested map whose
// parent key is an unquoted integer in YAML (e.g. `1:`) decodes as map[interface{}]interface{}
// rather than map[string]interface{}, per the isStringMap check in the yaml decoder library.
// DeepMergeNative only fast-recurses on map[string]any, so without normalizing
// map[interface{}]interface{} to map[string]any first, the whole integer-keyed submap was
// replaced instead of deep-merged, silently dropping catalog-only keys.
func TestDeepMergeNative_YAMLIntegerMapKey(t *testing.T) {
	catalogYAML := []byte(`
config:
  1:
    from_catalog: true
    shared_key: catalog_value
`)
	stackYAML := []byte(`
config:
  1:
    from_stack: true
    shared_key: stack_value
`)

	var catalog, stack map[string]any
	require.NoError(t, yaml.Unmarshal(catalogYAML, &catalog))
	require.NoError(t, yaml.Unmarshal(stackYAML, &stack))

	merged, err := DeepCopyMap(catalog)
	require.NoError(t, err)
	require.NoError(t, deepMergeNative(merged, stack, false, false))

	config, ok := merged["config"].(map[string]any)
	require.True(t, ok, "config should normalize to map[string]any")
	entry, ok := config["1"].(map[string]any)
	require.True(t, ok, "config.1 should normalize to map[string]any")

	assert.Equal(t, true, entry["from_catalog"], "catalog-only key must survive the merge")
	assert.Equal(t, true, entry["from_stack"])
	assert.Equal(t, "stack_value", entry["shared_key"], "stack value should override catalog value")
}

// TestNormalizeMapReflect_InterfaceKeyedMap covers the underlying normalization contract
// directly: a map[interface{}]interface{} with dynamic (non-string) keys — the shape yaml.v3
// produces for a mapping containing an unquoted non-string key — must stringify its keys into
// map[string]any, distinct from genuinely-typed maps with a concrete non-string key type (e.g.
// map[int]string, covered by TestDeepCopyMap_TypedMaps), which must keep their concrete type.
func TestNormalizeMapReflect_InterfaceKeyedMap(t *testing.T) {
	src := map[interface{}]interface{}{
		1:      "one",
		"name": "two",
	}

	result := deepCopyValue(src)

	stringKeyed, ok := result.(map[string]any)
	require.True(t, ok, "map[interface{}]interface{} with dynamic keys must normalize to map[string]any")
	assert.Equal(t, "one", stringKeyed["1"])
	assert.Equal(t, "two", stringKeyed["name"])
}

// TestNormalizeMapReflect_CollidingStringifiedKeysAreMerged covers a review finding on PR #2700:
// distinct original keys can stringify to the same string (e.g. int(1) and float64(1), both
// formatting to "1" via fmt.Sprintf("%v", ...)). Map[interface{}]interface{} iteration order is
// unspecified, so a plain overwrite on collision would silently and non-deterministically drop
// one entry. When both colliding values are maps, they must be merged so no data is lost.
func TestNormalizeMapReflect_CollidingStringifiedKeysAreMerged(t *testing.T) {
	src := map[interface{}]interface{}{
		1:          map[interface{}]interface{}{"from_int_key": true, "shared": "int_value"},
		float64(1): map[interface{}]interface{}{"from_float_key": true, "shared": "float_value"},
	}

	result := deepCopyValue(src)

	stringKeyed, ok := result.(map[string]any)
	require.True(t, ok, "map[interface{}]interface{} must normalize to map[string]any")
	require.Len(t, stringKeyed, 1, "colliding keys must merge into a single entry, not two")

	entry, ok := stringKeyed["1"].(map[string]any)
	require.True(t, ok, "merged collision entry should normalize to map[string]any")
	assert.Equal(t, true, entry["from_int_key"], "data from the int-keyed entry must survive the collision")
	assert.Equal(t, true, entry["from_float_key"], "data from the float-keyed entry must survive the collision")
	assert.Contains(t, []string{"int_value", "float_value"}, entry["shared"],
		"the overlapping subkey wins non-deterministically, but must hold one of the two values, not be dropped")
}

// TestDeepCopyMap_NonMapKeyCollisionReturnsError covers a follow-up CodeRabbit review finding
// on PR #2700 (upgraded to Major): when colliding stringified keys are NOT both maps (e.g. two
// scalars, or a scalar and a map), there is no safe way to combine them, so silently falling
// back to overwrite could non-deterministically drop data. Function normalizeMapReflect now
// raises a mapKeyCollisionPanic instead, which DeepCopyMap recovers into a normal wrapped
// error — the panic must never escape the public API.
func TestDeepCopyMap_NonMapKeyCollisionReturnsError(t *testing.T) {
	input := map[string]any{
		"config": map[interface{}]interface{}{
			1:          "from_int_key",
			float64(1): "from_float_key",
		},
	}

	result, err := DeepCopyMap(input)
	require.Error(t, err, "a non-map collision must be rejected, not silently resolved")
	assert.ErrorIs(t, err, errUtils.ErrMergeKeyCollision)
	assert.Nil(t, result, "no partial result should be returned alongside the error")
}

// TestMergeWithOptions_NonMapKeyCollisionReturnsError verifies the same collision rejection
// surfaces through MergeWithOptions (the multi-input merge path, via deepMergeNativeTopLevel)
// and not just through DeepCopyMap's single-input fast path.
func TestMergeWithOptions_NonMapKeyCollisionReturnsError(t *testing.T) {
	base := map[string]any{"config": map[string]any{"other": "value"}}
	overlay := map[string]any{
		"config": map[interface{}]interface{}{
			1:          "from_int_key",
			float64(1): "from_float_key",
		},
	}

	result, err := MergeWithOptions(nil, []map[string]any{base, overlay}, false, false)
	require.Error(t, err, "a non-map collision during merge must be rejected, not silently resolved")
	assert.ErrorIs(t, err, errUtils.ErrMergeKeyCollision)
	assert.Nil(t, result)
}

// TestRecoveredMapKeyCollision_DoesNotMaskOtherPanics ensures recoveredMapKeyCollision only
// converts our own deliberate mapKeyCollisionPanic into an error. Any other panic value (a
// genuine bug) must re-panic unchanged so it still reaches Atmos's global panic handler with
// its real value and stack trace, instead of being misreported as a merge-collision error.
func TestRecoveredMapKeyCollision_DoesNotMaskOtherPanics(t *testing.T) {
	run := func() (err error) {
		defer func() {
			if e := recoveredMapKeyCollision(recover()); e != nil {
				err = e
			}
		}()
		panic("unrelated bug: nil pointer dereference")
	}

	assert.PanicsWithValue(t, "unrelated bug: nil pointer dereference", func() {
		_ = run()
	})
}
