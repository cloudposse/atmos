package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestMergeWithProvenance_IntegrationE2E(t *testing.T) {
	// Test end-to-end provenance tracking with YAML parsing.
	yamlContent := `
vars:
  name: test-component
  enabled: true
  tags:
    environment: dev
    owner: platform
`

	// Convert to map for merging.
	data, err := u.UnmarshalYAML[map[string]any](yamlContent)
	require.NoError(t, err)

	// Simulate position information that would come from YAML parsing.
	positions := u.PositionMap{
		"vars.name":             {Line: 3, Column: 9},
		"vars.enabled":          {Line: 4, Column: 12},
		"vars.tags.environment": {Line: 6, Column: 18},
		"vars.tags.owner":       {Line: 7, Column: 13},
	}

	// Create merge context with provenance enabled.
	ctx := NewMergeContext()
	ctx.CurrentFile = "test.yaml"
	ctx.EnableProvenance()

	// Create atmos config.
	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: true,
	}

	// Merge with provenance tracking.
	result, err := MergeWithProvenance(atmosConfig, []map[string]any{data}, ctx, positions)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify provenance was recorded.
	assert.True(t, ctx.HasProvenance("vars.name"))
	assert.True(t, ctx.HasProvenance("vars.enabled"))
	assert.True(t, ctx.HasProvenance("vars.tags.environment"))
	assert.True(t, ctx.HasProvenance("vars.tags.owner"))

	// Verify provenance entry information.
	provenance := ctx.GetProvenance("vars.name")
	require.NotEmpty(t, provenance)
	assert.Equal(t, "test.yaml", provenance[0].File)
	assert.Equal(t, 3, provenance[0].Line)
	assert.Equal(t, 9, provenance[0].Column)
}

func TestMergeWithProvenance_DisabledIntegration(t *testing.T) {
	// Test that provenance tracking is skipped when disabled.
	data := map[string]any{
		"vars": map[string]any{
			"name": "test",
		},
	}

	// Create atmos config with provenance disabled.
	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: false,
	}

	ctx := NewMergeContext()
	ctx.CurrentFile = "test.yaml"
	// Don't enable provenance in context.

	// Merge should work but skip provenance tracking.
	result, err := MergeWithProvenance(atmosConfig, []map[string]any{data}, ctx, nil)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify no provenance was recorded.
	assert.False(t, ctx.HasProvenance("vars.name"))
}
