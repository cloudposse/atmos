package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
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
