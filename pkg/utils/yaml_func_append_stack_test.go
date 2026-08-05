package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	yaml "gopkg.in/yaml.v3"
)

// TestStackAppend_WrapsSequence verifies that an !append-tagged sequence in a stack
// manifest is rewritten into the append-metadata wrapper during unmarshaling, so the merge
// phase can later append it instead of replacing.
func TestStackAppend_WrapsSequence(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	input := "items: !append\n  - a\n  - b\n"

	got, err := UnmarshalYAMLFromFile[map[string]any](cfg, input, "stack.yaml")
	require.NoError(t, err)

	list, isAppend := ExtractAppendListValue(got["items"])
	require.True(t, isAppend, "items should carry the append wrapper, got %#v", got["items"])
	assert.Equal(t, []any{"a", "b"}, list)
}

// TestStackAppend_Nested verifies the rewrite works for deeply nested !append tags.
func TestStackAppend_Nested(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	input := `components:
  terraform:
    eks:
      settings:
        depends_on: !append
          - rds
          - elasticache
`
	got, err := UnmarshalYAMLFromFile[map[string]any](cfg, input, "stack.yaml")
	require.NoError(t, err)

	settings := got["components"].(map[string]any)["terraform"].(map[string]any)["eks"].(map[string]any)["settings"].(map[string]any)
	list, isAppend := ExtractAppendListValue(settings["depends_on"])
	require.True(t, isAppend, "nested depends_on should carry the append wrapper, got %#v", settings["depends_on"])
	assert.Equal(t, []any{"rds", "elasticache"}, list)
}

// TestStackAppend_ListOfMaps verifies !append preserves list elements that are maps.
func TestStackAppend_ListOfMaps(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	input := `node_groups: !append
  - name: spot
    instance_type: t3.large
`
	got, err := UnmarshalYAMLFromFile[map[string]any](cfg, input, "stack.yaml")
	require.NoError(t, err)

	list, isAppend := ExtractAppendListValue(got["node_groups"])
	require.True(t, isAppend)
	require.Len(t, list, 1)
	assert.Equal(t, map[string]any{"name": "spot", "instance_type": "t3.large"}, list[0])
}

// TestStackAppend_InnerCustomTagsPreserved verifies that custom tags inside an !append list
// are still converted to their string form for later resolution (not dropped by the rewrite).
func TestStackAppend_InnerCustomTagsPreserved(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	input := "items: !append\n  - !env SOME_VAR\n  - plain\n"

	got, err := UnmarshalYAMLFromFile[map[string]any](cfg, input, "stack.yaml")
	require.NoError(t, err)

	list, isAppend := ExtractAppendListValue(got["items"])
	require.True(t, isAppend)
	// The inner !env tag is converted to its string form ("!env SOME_VAR") so the
	// downstream YAML-function processor can resolve it after the merge.
	assert.Equal(t, []any{"!env SOME_VAR", "plain"}, list)
}

// TestStackAppend_NonSequenceClearsTag verifies that !append on a non-sequence value is a
// graceful no-op: the tag is cleared and the value decodes normally (no append wrapper, no
// decode error on the unknown tag).
func TestStackAppend_NonSequenceClearsTag(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	input := "thing: !append scalar-value\n"

	got, err := UnmarshalYAMLFromFile[map[string]any](cfg, input, "stack.yaml")
	require.NoError(t, err)

	assert.Equal(t, "scalar-value", got["thing"])
	assert.False(t, HasAppendTag(got["thing"]))
}

// TestHasCustomTags_DetectsAppend verifies the fast-path scanner recognizes !append so the
// node walker is not skipped for trees that only contain an !append tag.
func TestHasCustomTags_DetectsAppend(t *testing.T) {
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("items: !append\n  - a\n"), &node))
	assert.True(t, hasCustomTags(&node), "hasCustomTags must detect !append")
}
