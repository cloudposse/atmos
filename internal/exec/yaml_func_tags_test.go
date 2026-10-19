package exec

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// stackInfoWithMetadata builds the component context used by tag and label tests.
func stackInfoWithMetadata(metadata map[string]any) *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{
		ComponentSection: schema.AtmosSectionMapType{
			"metadata": metadata,
		},
	}
}

// TestProcessTagTags checks generic slice results and missing metadata behavior.
func TestProcessTagTags(t *testing.T) {
	t.Parallel()

	atmosConfig := &schema.AtmosConfiguration{}

	// Return type is []any (not []string): the deferred-merge machinery's isSlice/mergeSlices
	// (pkg/merge/merge_yaml_functions.go) only recognize []any, matching every other post-merge
	// YAML function's JSON-decoded result shape — see anySlice's doc comment in yaml_func_tags.go.
	t.Run("returns metadata.tags as a string slice", func(t *testing.T) {
		t.Parallel()
		stackInfo := stackInfoWithMetadata(map[string]any{
			"tags": []any{"production", "networking"},
		})
		got := processTagTags(atmosConfig, "!tags", stackInfo)
		assert.Equal(t, []any{"production", "networking"}, got)
	})

	t.Run("returns empty slice when metadata.tags is unset", func(t *testing.T) {
		t.Parallel()
		stackInfo := stackInfoWithMetadata(map[string]any{})
		got := processTagTags(atmosConfig, "!tags", stackInfo)
		assert.Equal(t, []any{}, got)
	})

	t.Run("returns empty slice when there is no metadata at all", func(t *testing.T) {
		t.Parallel()
		stackInfo := &schema.ConfigAndStacksInfo{}
		got := processTagTags(atmosConfig, "!tags", stackInfo)
		assert.Equal(t, []any{}, got)
	})
}

// TestProcessTagLabels verifies that bare !labels returns the complete map or an empty map when unset.
func TestProcessTagLabels(t *testing.T) {
	t.Parallel()

	atmosConfig := &schema.AtmosConfiguration{}

	// Return type is map[string]any (not map[string]string): see TestProcessTagTags' comment above.
	t.Run("returns metadata.labels as a string map", func(t *testing.T) {
		t.Parallel()
		stackInfo := stackInfoWithMetadata(map[string]any{
			"labels": map[string]any{"cost-center": "platform", "compliance": "sox"},
		})
		got, err := processTagLabels(atmosConfig, "!labels", stackInfo)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"cost-center": "platform", "compliance": "sox"}, got)
	})

	t.Run("returns empty map when metadata.labels is unset", func(t *testing.T) {
		t.Parallel()
		stackInfo := stackInfoWithMetadata(map[string]any{})
		got, err := processTagLabels(atmosConfig, "!labels", stackInfo)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{}, got)
	})
}

// TestProcessTagLabelsKeysAndValues verifies deterministic ordering and empty-label results.
func TestProcessTagLabelsKeysAndValues(t *testing.T) {
	t.Parallel()

	atmosConfig := &schema.AtmosConfiguration{}
	stackInfo := stackInfoWithMetadata(map[string]any{
		"labels": map[string]any{"b": "2", "a": "1", "c": "3"},
	})

	// Return type is []any (not []string): see TestProcessTagTags' comment above.
	t.Run("keys are sorted", func(t *testing.T) {
		t.Parallel()
		got := processTagLabelsKeys(atmosConfig, "!labels.keys", stackInfo)
		assert.Equal(t, []any{"a", "b", "c"}, got)
	})

	t.Run("values are ordered by key, not insertion order", func(t *testing.T) {
		t.Parallel()
		got := processTagLabelsValues(atmosConfig, "!labels.values", stackInfo)
		assert.Equal(t, []any{"1", "2", "3"}, got)
	})

	t.Run("empty labels yields empty keys and values", func(t *testing.T) {
		t.Parallel()
		empty := stackInfoWithMetadata(map[string]any{})
		assert.Equal(t, []any{}, processTagLabelsKeys(atmosConfig, "!labels.keys", empty))
		assert.Equal(t, []any{}, processTagLabelsValues(atmosConfig, "!labels.values", empty))
	})
}

// TestLabelsLookupDispatch checks label values, fallbacks, errors, tag boundaries,
// and function skipping through the shared YAML function dispatcher.
func TestLabelsLookupDispatch(t *testing.T) {
	t.Parallel()
	info := stackInfoWithMetadata(map[string]any{"labels": map[string]any{
		"runner": "self-hosted-large", "cost-center": "platform", "a.b/c": "literal", "empty": "",
	}})
	info.ComponentFromArg = "cluster/prod"
	info.Stack = "prod"
	for _, tc := range []struct {
		input string
		skip  []string
		info  *schema.ConfigAndStacksInfo
		want  any
		err   error
	}{
		{input: "!labels runner", info: info, want: "self-hosted-large"},
		{input: "!labels\trunner", info: info, want: "self-hosted-large"},
		{input: "!labels\nrunner", info: info, want: "self-hosted-large"},
		{input: "!labels runner ubuntu-latest", info: info, want: "self-hosted-large"},
		{input: "!labels cost-center", info: info, want: "platform"},
		{input: "!labels a.b/c", info: info, want: "literal"},
		{input: "!labels empty fallback", info: info, want: ""},
		{input: "!labels missing fallback", info: info, want: "fallback"},
		{input: `!labels missing ""`, info: info, want: ""},
		{input: "!labels missing", info: info, err: errUtils.ErrLabelNotFound},
		{input: "!labels Runner", info: info, err: errUtils.ErrLabelNotFound},
		{input: "!labels runner", err: errUtils.ErrLabelNotFound},
		{input: "!labels runner fallback", want: "fallback"},
		{input: "!labels", want: map[string]any{}},
		{input: "!labels runner fallback extra", info: info, err: errUtils.ErrInvalidArguments},
		{input: "!labels missing", skip: []string{"labels"}, info: info, want: "!labels missing"},
		{input: "!labels", skip: []string{"labels"}, info: info, want: "!labels"},
		{input: "!labels.keys", info: info, want: []any{"a.b/c", "cost-center", "empty", "runner"}},
		{input: "!labels.values", info: info, want: []any{"literal", "platform", "", "self-hosted-large"}},
		{input: "!labels.other runner", info: info, want: "!labels.other runner"},
		{input: "!labels-other runner", info: info, want: "!labels-other runner"},
		{input: "!labels/runner", info: info, want: "!labels/runner"},
		{input: "!labels:runner", info: info, want: "!labels:runner"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := processCustomTagsWithContext(&schema.AtmosConfiguration{}, tc.input, "prod", tc.skip, nil, tc.info)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
