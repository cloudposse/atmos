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

// TestGetGenerateFilenamesForComponent_NestedGenerate tests nested generate structure.
func TestGetGenerateFilenamesForComponent_NestedGenerate(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		expectedLen      int
	}{
		{
			name: "Generate with string templates",
			componentSection: map[string]any{
				"generate": map[string]any{
					"output.txt": "This is a template {{ .var }}",
				},
			},
			expectedLen: 1,
		},
		{
			name: "Generate with HCL file",
			componentSection: map[string]any{
				"generate": map[string]any{
					"main.tf": map[string]any{
						"resource": map[string]any{
							"aws_s3_bucket": map[string]any{},
						},
					},
				},
			},
			expectedLen: 1,
		},
		{
			name: "Generate with JSON file",
			componentSection: map[string]any{
				"generate": map[string]any{
					"config.json": map[string]any{
						"key": "value",
					},
				},
			},
			expectedLen: 1,
		},
		{
			name: "Generate with YAML file",
			componentSection: map[string]any{
				"generate": map[string]any{
					"config.yaml": map[string]any{
						"setting": true,
					},
				},
			},
			expectedLen: 1,
		},
		{
			name: "Generate with mixed file types",
			componentSection: map[string]any{
				"generate": map[string]any{
					"locals.tf":   map[string]any{},
					"config.json": map[string]any{},
					"readme.md":   "# README",
				},
			},
			expectedLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetGenerateFilenamesForComponent(tt.componentSection)
			assert.Len(t, result, tt.expectedLen)
		})
	}
}

// TestGetGenerateFilenamesForComponent_EdgeCases tests edge cases in generate section.
func TestGetGenerateFilenamesForComponent_EdgeCases(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		checkNil         bool
		expectedLen      int
	}{
		{
			name: "Generate with nil value",
			componentSection: map[string]any{
				"generate": map[string]any{
					"file.txt": nil,
				},
			},
			expectedLen: 1,
		},
		{
			name: "Generate with empty string value",
			componentSection: map[string]any{
				"generate": map[string]any{
					"empty.txt": "",
				},
			},
			expectedLen: 1,
		},
		{
			name: "Generate key is not a string",
			componentSection: map[string]any{
				"generate": 12345,
			},
			checkNil: true,
		},
		{
			name: "Generate is nil",
			componentSection: map[string]any{
				"generate": nil,
			},
			checkNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetGenerateFilenamesForComponent(tt.componentSection)
			if tt.checkNil {
				assert.Nil(t, result)
			} else {
				assert.Len(t, result, tt.expectedLen)
			}
		})
	}
}

// TestGetGenerateFilenamesForComponent_FilenamePatterns tests various filename patterns.
func TestGetGenerateFilenamesForComponent_FilenamePatterns(t *testing.T) {
	componentSection := map[string]any{
		"generate": map[string]any{
			"locals.tf":          map[string]any{},
			".hidden":            "hidden file",
			"deeply/nested.json": map[string]any{},
			"file-with-dash.tf":  map[string]any{},
			"file_with_under.tf": map[string]any{},
		},
	}

	result := GetGenerateFilenamesForComponent(componentSection)
	assert.Len(t, result, 5)

	// Verify all filenames are present.
	expectedFiles := []string{"locals.tf", ".hidden", "deeply/nested.json", "file-with-dash.tf", "file_with_under.tf"}
	for _, expected := range expectedFiles {
		assert.Contains(t, result, expected)
	}
}
