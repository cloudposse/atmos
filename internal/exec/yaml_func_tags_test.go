package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func stackInfoWithMetadata(metadata map[string]any) *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{
		ComponentSection: schema.AtmosSectionMapType{
			"metadata": metadata,
		},
	}
}

func TestProcessTagTags(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	t.Run("returns metadata.tags as a string slice", func(t *testing.T) {
		stackInfo := stackInfoWithMetadata(map[string]any{
			"tags": []any{"production", "networking"},
		})
		got := processTagTags(atmosConfig, "!tags", stackInfo)
		assert.Equal(t, []string{"production", "networking"}, got)
	})

	t.Run("returns empty slice when metadata.tags is unset", func(t *testing.T) {
		stackInfo := stackInfoWithMetadata(map[string]any{})
		got := processTagTags(atmosConfig, "!tags", stackInfo)
		assert.Equal(t, []string{}, got)
	})

	t.Run("returns empty slice when there is no metadata at all", func(t *testing.T) {
		stackInfo := &schema.ConfigAndStacksInfo{}
		got := processTagTags(atmosConfig, "!tags", stackInfo)
		assert.Equal(t, []string{}, got)
	})
}

func TestProcessTagLabels(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	t.Run("returns metadata.labels as a string map", func(t *testing.T) {
		stackInfo := stackInfoWithMetadata(map[string]any{
			"labels": map[string]any{"cost-center": "platform", "compliance": "sox"},
		})
		got := processTagLabels(atmosConfig, "!labels", stackInfo)
		assert.Equal(t, map[string]string{"cost-center": "platform", "compliance": "sox"}, got)
	})

	t.Run("returns empty map when metadata.labels is unset", func(t *testing.T) {
		stackInfo := stackInfoWithMetadata(map[string]any{})
		got := processTagLabels(atmosConfig, "!labels", stackInfo)
		assert.Equal(t, map[string]string{}, got)
	})
}

func TestProcessTagLabelsKeysAndValues(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stackInfo := stackInfoWithMetadata(map[string]any{
		"labels": map[string]any{"b": "2", "a": "1", "c": "3"},
	})

	t.Run("keys are sorted", func(t *testing.T) {
		got := processTagLabelsKeys(atmosConfig, "!labels.keys", stackInfo)
		assert.Equal(t, []string{"a", "b", "c"}, got)
	})

	t.Run("values are ordered by key, not insertion order", func(t *testing.T) {
		got := processTagLabelsValues(atmosConfig, "!labels.values", stackInfo)
		assert.Equal(t, []string{"1", "2", "3"}, got)
	})

	t.Run("empty labels yields empty keys and values", func(t *testing.T) {
		empty := stackInfoWithMetadata(map[string]any{})
		assert.Equal(t, []string{}, processTagLabelsKeys(atmosConfig, "!labels.keys", empty))
		assert.Equal(t, []string{}, processTagLabelsValues(atmosConfig, "!labels.values", empty))
	})
}
