package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetGenerateFilenamesForComponent tests that the function extracts filenames correctly.
func TestGetGenerateFilenamesForComponent(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		expectedFiles    []string
	}{
		{
			name:             "Nil section",
			componentSection: nil,
			expectedFiles:    nil,
		},
		{
			name:             "No generate section",
			componentSection: map[string]any{"vars": map[string]any{}},
			expectedFiles:    nil,
		},
		{
			name: "Empty generate section",
			componentSection: map[string]any{
				"generate": map[string]any{},
			},
			expectedFiles: []string{}, // Empty slice, not nil.
		},
		{
			name: "Single file in generate section",
			componentSection: map[string]any{
				"generate": map[string]any{
					"locals.tf": map[string]any{"locals": map[string]any{}},
				},
			},
			expectedFiles: []string{"locals.tf"},
		},
		{
			name: "Multiple files in generate section",
			componentSection: map[string]any{
				"generate": map[string]any{
					"locals.tf":    map[string]any{"locals": map[string]any{}},
					"backend.tf":   "terraform { backend \"s3\" {} }",
					"providers.tf": map[string]any{},
				},
			},
			expectedFiles: []string{"locals.tf", "backend.tf", "providers.tf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetGenerateFilenamesForComponent(tt.componentSection)

			switch {
			case tt.expectedFiles == nil:
				assert.Nil(t, result)
			case len(tt.expectedFiles) == 0:
				// Empty slice case.
				assert.Empty(t, result)
			default:
				assert.Len(t, result, len(tt.expectedFiles))
				for _, expected := range tt.expectedFiles {
					assert.Contains(t, result, expected)
				}
			}
		})
	}
}

// TestGetGenerateFilenamesForComponent_InvalidGenerateSection tests with non-map generate section.
func TestGetGenerateFilenamesForComponent_InvalidGenerateSection(t *testing.T) {
	componentSection := map[string]any{
		"generate": "not a map",
	}

	result := GetGenerateFilenamesForComponent(componentSection)
	assert.Nil(t, result)
}
