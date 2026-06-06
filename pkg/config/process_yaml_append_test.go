package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestHandleAppend_WrapsList verifies that an !append-tagged sequence is preprocessed into
// the __atmos_append__ wrapper that the merge phase later unwraps and appends.
func TestHandleAppend_WrapsList(t *testing.T) {
	yamlContent := `mylist: !append
  - a
  - b`

	v := viper.New()
	require.NoError(t, preprocessAtmosYamlFunc([]byte(yamlContent), v))

	got := v.Get("mylist")
	list, isAppend := u.ExtractAppendListValue(got)
	require.True(t, isAppend, "value should carry the append metadata, got %#v", got)
	assert.Equal(t, []any{"a", "b"}, list)
}

// TestHandleAppend_Nested verifies !append works on a nested path.
func TestHandleAppend_Nested(t *testing.T) {
	yamlContent := `settings:
  depends_on: !append
    - vpc
    - iam-role`

	v := viper.New()
	require.NoError(t, preprocessAtmosYamlFunc([]byte(yamlContent), v))

	got := v.Get("settings.depends_on")
	list, isAppend := u.ExtractAppendListValue(got)
	require.True(t, isAppend, "nested value should carry the append metadata, got %#v", got)
	assert.Equal(t, []any{"vpc", "iam-role"}, list)
}

// TestHandleAppend_ListOfMaps verifies !append preserves list elements that are maps.
func TestHandleAppend_ListOfMaps(t *testing.T) {
	yamlContent := `node_groups: !append
  - name: spot
    instance_type: t3.large`

	v := viper.New()
	require.NoError(t, preprocessAtmosYamlFunc([]byte(yamlContent), v))

	got := v.Get("node_groups")
	list, isAppend := u.ExtractAppendListValue(got)
	require.True(t, isAppend, "value should carry the append metadata, got %#v", got)
	require.Len(t, list, 1)
	assert.Equal(t, map[string]any{"name": "spot", "instance_type": "t3.large"}, list[0])
}

// TestHandleAppend_EmptyList verifies that an empty !append sequence still produces the
// append wrapper around an empty list (a no-op append rather than an error).
func TestHandleAppend_EmptyList(t *testing.T) {
	yamlContent := `mylist: !append []`

	v := viper.New()
	require.NoError(t, preprocessAtmosYamlFunc([]byte(yamlContent), v))

	got := v.Get("mylist")
	list, isAppend := u.ExtractAppendListValue(got)
	require.True(t, isAppend, "empty !append should still carry the append metadata, got %#v", got)
	assert.Empty(t, list)
}

// TestHandleAppend_ResolvesInnerScalarTags verifies that custom scalar tags inside an
// !append list (e.g. !env) are resolved during atmos.yaml preprocessing, matching the
// stack-manifest path, instead of being left as unevaluated "!env ..." strings.
func TestHandleAppend_ResolvesInnerScalarTags(t *testing.T) {
	t.Setenv("APPEND_INNER_TAG_TEST", "resolved-value")

	yamlContent := "items: !append\n  - !env APPEND_INNER_TAG_TEST\n  - static-item"

	v := viper.New()
	require.NoError(t, preprocessAtmosYamlFunc([]byte(yamlContent), v))

	list, isAppend := u.ExtractAppendListValue(v.Get("items"))
	require.True(t, isAppend, "items should carry the append wrapper, got %#v", v.Get("items"))
	assert.Equal(t, []any{"resolved-value", "static-item"}, list,
		"the inner !env tag must be resolved, not left as a literal string")
}

// TestHandleAppend_ResolvesNestedTagInMapItem verifies that a custom scalar tag nested
// inside a map item of an !append list is also resolved (not just top-level scalar items).
func TestHandleAppend_ResolvesNestedTagInMapItem(t *testing.T) {
	t.Setenv("APPEND_NESTED_MAP_TEST", "resolved-ami")

	yamlContent := "node_groups: !append\n  - name: spot\n    ami: !env APPEND_NESTED_MAP_TEST"

	v := viper.New()
	require.NoError(t, preprocessAtmosYamlFunc([]byte(yamlContent), v))

	list, isAppend := u.ExtractAppendListValue(v.Get("node_groups"))
	require.True(t, isAppend, "node_groups should carry the append wrapper, got %#v", v.Get("node_groups"))
	require.Len(t, list, 1)
	assert.Equal(t, map[string]any{"name": "spot", "ami": "resolved-ami"}, list[0],
		"the !env nested inside the map item must be resolved")

	// No indexed keys (e.g. node_groups[0]) should leak into Viper.
	for _, k := range v.AllKeys() {
		assert.NotContains(t, k, "[", "no indexed array keys should leak, found %q", k)
	}
}

// TestHandleAppend_RoundTripsThroughMergeContract ties the preprocessing output to the
// merge-side contract: HasAppendTag must recognize what handleAppend produces.
func TestHandleAppend_RoundTripsThroughMergeContract(t *testing.T) {
	yamlContent := `tags: !append
  - one
  - two`

	v := viper.New()
	require.NoError(t, preprocessAtmosYamlFunc([]byte(yamlContent), v))

	assert.True(t, u.HasAppendTag(v.Get("tags")),
		"merge phase must recognize the wrapper handleAppend produced")
}
