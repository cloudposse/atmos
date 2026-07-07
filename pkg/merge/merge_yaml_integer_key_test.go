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
