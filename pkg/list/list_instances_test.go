package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/list/filter"
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

// applyInstanceFilters runs rows through every filter in sequence, returning the final result.
func applyInstanceFilters(t *testing.T, rows []map[string]any, filters []filter.Filter) []map[string]any {
	t.Helper()
	result := any(rows)
	var err error
	for _, f := range filters {
		result, err = f.Apply(result)
		require.NoError(t, err)
	}
	filtered, ok := result.([]map[string]any)
	require.True(t, ok)
	return filtered
}

// instanceComponentsOf returns the "component" field of each row, for order-preserving assertions.
func instanceComponentsOf(rows []map[string]any) []string {
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r["component"].(string))
	}
	return names
}

// TestBuildInstanceFilters_FiltersRows verifies the produced tag/label filters
// actually narrow rows on the flattened tags/labels fields, and that
// multi-value tags are any-match while multi-value labels are all-match.
func TestBuildInstanceFilters_FiltersRows(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	rows := []map[string]any{
		{"component": "vpc", "stack": "dev", "tags": []string{"network"}, "labels": map[string]string{"team": "platform"}},
		{"component": "rds", "stack": "dev", "tags": []string{"database"}, "labels": map[string]string{"team": "data"}},
		{"component": "eks", "stack": "dev", "tags": []string{"network", "compute"}, "labels": map[string]string{"team": "platform", "env": "dev"}},
	}

	tests := []struct {
		name           string
		tags           []string
		labelsRaw      string
		wantFilterLen  int
		wantComponents []string
	}{
		{
			// "database" and "compute" never co-occur on one row, but requesting
			// both as an any-match still returns every row matching either one.
			name:           "multi-tag any-match",
			tags:           []string{"database", "compute"},
			wantFilterLen:  1,
			wantComponents: []string{"rds", "eks"},
		},
		{
			// eks has both team=platform and env=dev; vpc has team=platform but no env.
			// All-match must require every requested label, not just one.
			name:           "multi-label all-match",
			labelsRaw:      "team:platform,env:dev",
			wantFilterLen:  1,
			wantComponents: []string{"eks"},
		},
		{
			name:           "tags and labels combined match only the intersection",
			tags:           []string{"network"},
			labelsRaw:      "team:platform",
			wantFilterLen:  2,
			wantComponents: []string{"vpc", "eks"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filters, err := buildInstanceFilters("", tc.tags, tc.labelsRaw, atmosConfig)
			require.NoError(t, err)
			require.Len(t, filters, tc.wantFilterLen)

			filtered := applyInstanceFilters(t, rows, filters)
			assert.Equal(t, tc.wantComponents, instanceComponentsOf(filtered))
		})
	}
}
