package imports

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMetadataScope(t *testing.T) {
	for _, tc := range []struct {
		name, yaml string
		want       []string
	}{
		{"absent", "vars: {}", []string{}},
		{"literal", "metadata: {tags: [pci, production]}", []string{"pci", "production"}},
		{"alias", "defaults: &tags [pci]\nmetadata: {tags: *tags}", []string{"pci"}},
		{"scalar alias", "tag: &pci pci\nmetadata: {tags: [*pci]}", []string{"pci"}},
		{"append", "metadata: {tags: !append [pci]}", []string{"pci"}},
		{"null", "metadata: {tags: null}", []string{}},
		{"dynamic metadata", "metadata: !env METADATA", []string{}},
		{"dynamic map", "metadata: {tags: !env TAGS}", []string{}},
		{"dynamic member", "metadata: {tags: [pci, !env OTHER]}", []string{}},
		{"template", "metadata: {tags: ['[[ .vars.tag ]]']}", []string{}},
		{"wrong type", "metadata: {tags: [true]}", []string{}},
		{"malformed", "metadata: [", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := ReadMetadata(tc.yaml, []string{"[[", "]]"})
			ctx := condition.Context{}
			scope := MetadataScope{}
			require.NoError(t, scope.Activate(&ctx, schema.StackImportMetadata{}, metadata))
			assert.Equal(t, tc.want, ctx.Tags)
		})
	}
}

func TestMetadataScope_InvalidStrategy(t *testing.T) {
	scope := MetadataScope{Config: &schema.AtmosConfiguration{Settings: schema.AtmosSettings{ListMergeStrategy: "invalid"}}}
	metadata := ReadMetadata("metadata: {tags: [pci]}", nil)
	_, err := scope.Merge(metadata)
	require.ErrorIs(t, err, errUtils.ErrInvalidListMergeStrategy)
	err = scope.Activate(&condition.Context{}, metadata, schema.StackImportMetadata{})
	require.ErrorIs(t, err, errUtils.ErrInvalidListMergeStrategy)
	_, err = scope.Child(metadata, schema.StackImportMetadata{})
	require.ErrorIs(t, err, errUtils.ErrInvalidListMergeStrategy)
}

func TestMetadataScope_UnavailableTags(t *testing.T) {
	negative, err := condition.New("!('pci' in tags)")
	require.NoError(t, err)
	for _, content := range []string{
		"metadata: {tags: !env TAGS}",
		"metadata: {tags: [pci, !env OTHER]}",
		"metadata: !env METADATA",
		"metadata: [",
	} {
		ctx := condition.Context{}
		require.NoError(t, (MetadataScope{}).Activate(&ctx, schema.StackImportMetadata{}, ReadMetadata(content, nil)))
		assert.True(t, ctx.TagsUnavailable)
		_, err := negative.EvaluateE(ctx)
		require.ErrorIs(t, err, condition.ErrInvalidWhenCondition, "unknown membership must not enable negative conditions")
	}
}
