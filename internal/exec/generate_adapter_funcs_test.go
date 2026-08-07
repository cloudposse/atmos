package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
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

// TestFindStacksMapForGenerate verifies the adapter wrapper forwards its arguments to
// FindStacksMap and returns the same stacksMap/rawStackConfigs, deliberately discarding only the
// deferred-contexts return value (see FindStacksMapForGenerate's doc comment: varfile/backend
// generation never resolves deferred YAML functions).
func TestFindStacksMapForGenerate(t *testing.T) {
	atmosConfig := setupSharedCacheFixture(t)

	stacksMap, rawStackConfigs, err := FindStacksMapForGenerate(&atmosConfig, false)
	require.NoError(t, err)
	require.NotEmpty(t, stacksMap, "must return the real stacks map, not an empty stub")
	require.NotEmpty(t, rawStackConfigs)

	expectedStacksMap, expectedRawStackConfigs, _, expectedErr := FindStacksMap(&atmosConfig, false)
	require.NoError(t, expectedErr)
	assert.Equal(t, expectedStacksMap, stacksMap, "must return the exact same stacksMap as FindStacksMap")
	assert.Equal(t, expectedRawStackConfigs, rawStackConfigs, "must return the exact same rawStackConfigs as FindStacksMap")
}

// TestProcessStacksForGenerate verifies the adapter wrapper forwards its arguments to
// ProcessStacks and resolves the same component configuration.
func TestProcessStacksForGenerate(t *testing.T) {
	atmosConfig := setupSharedCacheFixture(t)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "my-explicit-stack",
		ComponentType:    cfg.TerraformComponentType,
	}

	result, err := ProcessStacksForGenerate(&atmosConfig, info, true, false, false, nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.ComponentSection, "must return a real, populated component section")

	vars, ok := result.ComponentSection[cfg.VarsSectionName].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "10.0.0.0/16", vars["cidr"], "must resolve the component's own vars from the fixture stack")
}
