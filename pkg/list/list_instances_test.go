package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestBuildInstanceFilters_TagsLabels verifies filter construction from the --filter,
// --tags, and --labels flags.
func TestBuildInstanceFilters_TagsLabels(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name          string
		filterSpec    string
		tags          []string
		labelsRaw     string
		expectedCount int
		expectErr     bool
	}{
		{
			name:          "empty inputs produce no filters",
			expectedCount: 0,
		},
		{
			name:          "filter spec only",
			filterSpec:    `.component == "vpc"`,
			expectedCount: 1,
		},
		{
			name:          "tags only",
			tags:          []string{"network"},
			expectedCount: 1,
		},
		{
			name:          "labels only",
			labelsRaw:     "team=platform",
			expectedCount: 1,
		},
		{
			name:          "labels with colon separator",
			labelsRaw:     "team:platform",
			expectedCount: 1,
		},
		{
			name:          "filter spec plus tags plus labels",
			filterSpec:    `.component == "vpc"`,
			tags:          []string{"network"},
			labelsRaw:     "team=platform",
			expectedCount: 3,
		},
		{
			name:      "invalid labels error",
			labelsRaw: "no-separator",
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filters, err := buildInstanceFilters(tc.filterSpec, tc.tags, tc.labelsRaw, atmosConfig)
			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, filters, tc.expectedCount)
		})
	}
}

// TestBuildInstanceFilters_FiltersRows verifies the produced tag/label filters
// actually narrow rows on the flattened tags/labels fields.
func TestBuildInstanceFilters_FiltersRows(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	rows := []map[string]any{
		{"component": "vpc", "stack": "dev", "tags": []string{"network"}, "labels": map[string]string{"team": "platform"}},
		{"component": "rds", "stack": "dev", "tags": []string{"database"}, "labels": map[string]string{"team": "data"}},
		{"component": "eks", "stack": "dev", "tags": []string{"network", "compute"}, "labels": map[string]string{"team": "platform", "env": "dev"}},
	}

	filters, err := buildInstanceFilters("", []string{"network"}, "team:platform", atmosConfig)
	require.NoError(t, err)
	require.Len(t, filters, 2)

	result := any(rows)
	for _, f := range filters {
		result, err = f.Apply(result)
		require.NoError(t, err)
	}

	filtered, ok := result.([]map[string]any)
	require.True(t, ok)
	require.Len(t, filtered, 2)
	assert.Equal(t, "vpc", filtered[0]["component"])
	assert.Equal(t, "eks", filtered[1]["component"])
}
