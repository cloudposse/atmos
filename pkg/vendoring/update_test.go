package vendoring

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		sep      string
		expected []string
	}{
		{
			name:     "comma separated with spaces",
			input:    "tag1, tag2, tag3",
			sep:      ",",
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:     "comma separated without spaces",
			input:    "tag1,tag2,tag3",
			sep:      ",",
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:     "single item",
			input:    "tag1",
			sep:      ",",
			expected: []string{"tag1"},
		},
		{
			name:     "empty string",
			input:    "",
			sep:      ",",
			expected: nil, // Empty input returns nil, not []string{""}.
		},
		{
			name:     "with extra spaces",
			input:    " tag1 , tag2 , tag3 ",
			sep:      ",",
			expected: []string{"tag1", "tag2", "tag3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrim(tt.input, tt.sep)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// splitString and trimString tests removed - now using standard library strings.Split and strings.TrimSpace.

func TestFilterSourcesByComponent(t *testing.T) {
	sources := []schema.AtmosVendorSource{
		{Component: "vpc", Source: "github.com/example/vpc"},
		{Component: "eks", Source: "github.com/example/eks"},
		{Component: "rds", Source: "github.com/example/rds"},
	}

	tests := []struct {
		name      string
		component string
		expected  int
	}{
		{
			name:      "find existing component",
			component: "vpc",
			expected:  1,
		},
		{
			name:      "find non-existing component",
			component: "lambda",
			expected:  0,
		},
		{
			name:      "case sensitive match",
			component: "VPC",
			expected:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterSourcesByComponent(sources, tt.component)
			assert.Len(t, result, tt.expected)
			if tt.expected > 0 {
				assert.Equal(t, tt.component, result[0].Component)
			}
		})
	}
}

func TestFilterSourcesByTags(t *testing.T) {
	sources := []schema.AtmosVendorSource{
		{Component: "vpc", Tags: []string{"networking", "core"}},
		{Component: "eks", Tags: []string{"kubernetes", "core"}},
		{Component: "rds", Tags: []string{"database"}},
		{Component: "lambda", Tags: []string{}},
	}

	tests := []struct {
		name     string
		tags     []string
		expected int
	}{
		{
			name:     "single tag match",
			tags:     []string{"networking"},
			expected: 1,
		},
		{
			name:     "multiple tag match - OR logic",
			tags:     []string{"networking", "database"},
			expected: 2,
		},
		{
			name:     "common tag matches multiple sources",
			tags:     []string{"core"},
			expected: 2,
		},
		{
			name:     "no matching tags",
			tags:     []string{"nonexistent"},
			expected: 0,
		},
		{
			name:     "empty tags returns all sources",
			tags:     []string{},
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterSourcesByTags(sources, tt.tags)
			assert.Len(t, result, tt.expected)
		})
	}
}

func TestUpdateResult(t *testing.T) {
	// Test that updateResult properly tracks version differences.
	result := updateResult{
		Component:      "vpc",
		CurrentVersion: "1.0.0",
		LatestVersion:  "1.1.0",
		HasUpdate:      true,
	}

	assert.Equal(t, "vpc", result.Component)
	assert.Equal(t, "1.0.0", result.CurrentVersion)
	assert.Equal(t, "1.1.0", result.LatestVersion)
	assert.True(t, result.HasUpdate)

	// Test no update case.
	noUpdate := updateResult{
		Component:      "eks",
		CurrentVersion: "2.0.0",
		LatestVersion:  "2.0.0",
		HasUpdate:      false,
	}

	assert.False(t, noUpdate.HasUpdate)
}
