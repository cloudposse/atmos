package list

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveImportTreeFromProvenance tests the main entry point for import tree resolution.
func TestResolveImportTreeFromProvenance(t *testing.T) {
	tests := []struct {
		name        string
		stacksMap   map[string]interface{}
		setupMerge  func()
		expectEmpty bool
		expectStack string
	}{
		{
			name:        "Empty stacks map",
			stacksMap:   map[string]interface{}{},
			setupMerge:  func() {},
			expectEmpty: true,
		},
		{
			name: "Stack with no merge context",
			stacksMap: map[string]interface{}{
				"test-stack": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"atmos_stack_file": "test-stack.yaml",
							},
						},
					},
				},
			},
			setupMerge:  func() {},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMerge()

			atmosConfig := &schema.AtmosConfiguration{
				StacksBaseAbsolutePath: "/tmp/stacks",
			}

			result, err := ResolveImportTreeFromProvenance(tt.stacksMap, atmosConfig)
			require.NoError(t, err)

			if tt.expectEmpty {
				assert.Empty(t, result)
			} else if tt.expectStack != "" {
				assert.Contains(t, result, tt.expectStack)
			}
		})
	}
}

// TestFindStacksForFilePath tests the file path matching logic.
func TestFindStacksForFilePath(t *testing.T) {
	tests := []struct {
		name            string
		filePath        string
		stacksMap       map[string]interface{}
		atmosConfig     *schema.AtmosConfiguration
		expectedStacks  int
		expectComponent string
	}{
		{
			name:           "No matching stacks",
			filePath:       "/tmp/stacks/nonexistent.yaml",
			stacksMap:      map[string]interface{}{},
			atmosConfig:    &schema.AtmosConfiguration{StacksBaseAbsolutePath: "/tmp/stacks"},
			expectedStacks: 0,
		},
		{
			name:     "Exact absolute path match",
			filePath: "/tmp/stacks/test-stack.yaml",
			stacksMap: map[string]interface{}{
				"test-stack": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"atmos_stack_file": "/tmp/stacks/test-stack.yaml",
							},
						},
					},
				},
			},
			atmosConfig:     &schema.AtmosConfiguration{StacksBaseAbsolutePath: "/tmp/stacks"},
			expectedStacks:  1,
			expectComponent: "vpc",
		},
		{
			name:     "Relative path match",
			filePath: "/tmp/stacks/test-stack.yaml",
			stacksMap: map[string]interface{}{
				"test-stack": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"atmos_stack_file": "test-stack.yaml",
							},
						},
					},
				},
			},
			atmosConfig:     &schema.AtmosConfiguration{StacksBaseAbsolutePath: "/tmp/stacks"},
			expectedStacks:  1,
			expectComponent: "vpc",
		},
		{
			name:     "Match without .yaml extension",
			filePath: "/tmp/stacks/test-stack.yaml",
			stacksMap: map[string]interface{}{
				"test-stack": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"atmos_stack_file": "test-stack",
							},
						},
					},
				},
			},
			atmosConfig:     &schema.AtmosConfiguration{StacksBaseAbsolutePath: "/tmp/stacks"},
			expectedStacks:  1,
			expectComponent: "vpc",
		},
		{
			name:     "Match with .yml extension",
			filePath: "/tmp/stacks/test-stack.yml",
			stacksMap: map[string]interface{}{
				"test-stack": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"atmos_stack_file": "test-stack.yaml",
							},
						},
					},
				},
			},
			atmosConfig:     &schema.AtmosConfiguration{StacksBaseAbsolutePath: "/tmp/stacks"},
			expectedStacks:  1,
			expectComponent: "vpc",
		},
		{
			name:     "Multiple components in same stack",
			filePath: "/tmp/stacks/test-stack.yaml",
			stacksMap: map[string]interface{}{
				"test-stack": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"atmos_stack_file": "test-stack.yaml",
							},
							"database": map[string]interface{}{
								"atmos_stack_file": "test-stack.yaml",
							},
						},
					},
				},
			},
			atmosConfig:     &schema.AtmosConfiguration{StacksBaseAbsolutePath: "/tmp/stacks"},
			expectedStacks:  1,
			expectComponent: "vpc",
		},
		{
			name:     "Invalid stack data structure",
			filePath: "/tmp/stacks/test-stack.yaml",
			stacksMap: map[string]interface{}{
				"test-stack": "invalid",
			},
			atmosConfig:    &schema.AtmosConfiguration{StacksBaseAbsolutePath: "/tmp/stacks"},
			expectedStacks: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findStacksForFilePath(tt.filePath, tt.stacksMap, tt.atmosConfig)
			assert.Equal(t, tt.expectedStacks, len(result))

			if tt.expectComponent != "" && len(result) > 0 {
				for _, components := range result {
					assert.True(t, components[tt.expectComponent], "Expected component %s to be present", tt.expectComponent)
				}
			}
		})
	}
}

// TestExtractComponentFolders tests component folder extraction.
func TestExtractComponentFolders(t *testing.T) {
	tests := []struct {
		name           string
		stackData      interface{}
		expectedFolder string
		expectEmpty    bool
	}{
		{
			name: "Component with metadata.component",
			stackData: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"vpc": map[string]interface{}{
							"metadata": map[string]interface{}{
								"component": "base-vpc",
							},
						},
					},
				},
			},
			expectedFolder: "components/terraform/base-vpc",
		},
		{
			name: "Component without metadata (uses component name)",
			stackData: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"vpc": map[string]interface{}{},
					},
				},
			},
			expectedFolder: "components/terraform/vpc",
		},
		{
			name: "Component with empty metadata.component",
			stackData: map[string]interface{}{
				"components": map[string]interface{}{
					"helmfile": map[string]interface{}{
						"monitoring": map[string]interface{}{
							"metadata": map[string]interface{}{
								"component": "",
							},
						},
					},
				},
			},
			expectedFolder: "components/helmfile/monitoring",
		},
		{
			name:        "Invalid stack data",
			stackData:   "invalid",
			expectEmpty: true,
		},
		{
			name: "No components section",
			stackData: map[string]interface{}{
				"vars": map[string]interface{}{
					"key": "value",
				},
			},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentFolders(tt.stackData)

			if tt.expectEmpty {
				assert.Empty(t, result)
			} else {
				assert.NotEmpty(t, result)
				if tt.expectedFolder != "" {
					found := false
					for _, folder := range result {
						if folder == tt.expectedFolder {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected folder %s not found in result", tt.expectedFolder)
				}
			}
		})
	}
}

// TestExtractComponentsFromStackData tests component name extraction.
func TestExtractComponentsFromStackData(t *testing.T) {
	tests := []struct {
		name            string
		stackData       interface{}
		expectedCount   int
		expectComponent string
	}{
		{
			name: "Multiple components across types",
			stackData: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"vpc":      map[string]interface{}{},
						"database": map[string]interface{}{},
					},
					"helmfile": map[string]interface{}{
						"monitoring": map[string]interface{}{},
					},
				},
			},
			expectedCount:   3,
			expectComponent: "vpc",
		},
		{
			name: "Single component",
			stackData: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"vpc": map[string]interface{}{},
					},
				},
			},
			expectedCount:   1,
			expectComponent: "vpc",
		},
		{
			name:          "Invalid stack data",
			stackData:     "invalid",
			expectedCount: 0,
		},
		{
			name: "No components section",
			stackData: map[string]interface{}{
				"vars": map[string]interface{}{},
			},
			expectedCount: 0,
		},
		{
			name: "Empty components section",
			stackData: map[string]interface{}{
				"components": map[string]interface{}{},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentsFromStackData(tt.stackData)
			assert.Equal(t, tt.expectedCount, len(result))

			if tt.expectComponent != "" {
				assert.True(t, result[tt.expectComponent], "Expected component %s to be present", tt.expectComponent)
			}
		})
	}
}

// TestBuildImportTreeFromChain tests import tree construction.
func TestBuildImportTreeFromChain(t *testing.T) {
	tests := []struct {
		name           string
		importChain    []string
		expectNodes    int
		expectCircular bool
	}{
		{
			name:        "Empty chain",
			importChain: []string{},
			expectNodes: 0,
		},
		{
			name:        "Single file (no imports)",
			importChain: []string{"/tmp/stacks/stack.yaml"},
			expectNodes: 0,
		},
		{
			name: "Simple import chain",
			importChain: []string{
				"/tmp/stacks/stack.yaml",
				"/tmp/stacks/catalog/base.yaml",
			},
			expectNodes: 1,
		},
		{
			name: "Multiple imports",
			importChain: []string{
				"/tmp/stacks/stack.yaml",
				"/tmp/stacks/catalog/base.yaml",
				"/tmp/stacks/catalog/network.yaml",
			},
			expectNodes: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				StacksBaseAbsolutePath: "/tmp/stacks",
			}

			result := buildImportTreeFromChain(tt.importChain, atmosConfig)
			assert.Equal(t, tt.expectNodes, len(result))
		})
	}
}

// TestStripBasePath tests base path removal.
func TestStripBasePath(t *testing.T) {
	tests := []struct {
		name         string
		absolutePath string
		basePath     string
		expected     string
	}{
		{
			name:         "Simple path",
			absolutePath: "/tmp/stacks/catalog/base.yaml",
			basePath:     "/tmp/stacks",
			expected:     "catalog/base",
		},
		{
			name:         "Base path with trailing slash",
			absolutePath: "/tmp/stacks/catalog/base.yaml",
			basePath:     "/tmp/stacks/",
			expected:     "catalog/base",
		},
		{
			name:         "Remove .yml extension",
			absolutePath: "/tmp/stacks/catalog/base.yml",
			basePath:     "/tmp/stacks",
			expected:     "catalog/base",
		},
		{
			name:         "Path already relative",
			absolutePath: "catalog/base.yaml",
			basePath:     "/tmp/stacks",
			expected:     "catalog/base",
		},
		{
			name:         "Nested directories",
			absolutePath: "/tmp/stacks/catalog/network/vpc.yaml",
			basePath:     "/tmp/stacks",
			expected:     "catalog/network/vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripBasePath(tt.absolutePath, tt.basePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestResolveImportPath tests import path resolution.
func TestResolveImportPath(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		expected   string
	}{
		{
			name:       "Simple import",
			importPath: "catalog/base",
			expected:   "/tmp/stacks/catalog/base.yaml",
		},
		{
			name:       "Import with .yaml extension",
			importPath: "catalog/base.yaml",
			expected:   "/tmp/stacks/catalog/base.yaml",
		},
		{
			name:       "Import with .yml extension",
			importPath: "catalog/base.yml",
			expected:   "/tmp/stacks/catalog/base.yml",
		},
		{
			name:       "Nested import",
			importPath: "catalog/network/vpc",
			expected:   "/tmp/stacks/catalog/network/vpc.yaml",
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: "/tmp/stacks",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveImportPath(tt.importPath, "", atmosConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestReadImportsFromYAMLFile tests YAML import extraction.
func TestReadImportsFromYAMLFile(t *testing.T) {
	// Create temporary directory for test files.
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		yamlContent string
		expected    []string
		expectError bool
	}{
		{
			name: "Single import field",
			yamlContent: `
import: catalog/base
vars:
  environment: prod
`,
			expected: []string{"catalog/base"},
		},
		{
			name: "Multiple imports field",
			yamlContent: `
imports:
  - catalog/base
  - catalog/network
  - catalog/security
`,
			expected: []string{"catalog/base", "catalog/network", "catalog/security"},
		},
		{
			name: "Both import and imports fields",
			yamlContent: `
import: catalog/base
imports:
  - catalog/network
  - catalog/security
`,
			expected: []string{"catalog/base", "catalog/network", "catalog/security"},
		},
		{
			name: "No imports",
			yamlContent: `
vars:
  environment: prod
`,
			expected: []string{},
		},
		{
			name:        "Invalid YAML",
			yamlContent: "invalid: [unclosed",
			expectError: true,
		},
		{
			name: "Empty imports array",
			yamlContent: `
imports: []
`,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary YAML file.
			tmpFile := filepath.Join(tmpDir, "test.yaml")
			err := os.WriteFile(tmpFile, []byte(tt.yamlContent), 0o644)
			require.NoError(t, err)

			result, err := readImportsFromYAMLFile(tmpFile)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}

	// Test missing file.
	t.Run("Missing file", func(t *testing.T) {
		_, err := readImportsFromYAMLFile("/nonexistent/file.yaml")
		assert.Error(t, err)
	})
}

// TestExtractImportStringsHelper tests import string extraction from various types.
func TestExtractImportStringsHelper(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected []string
	}{
		{
			name:     "String value",
			input:    "catalog/base",
			expected: []string{"catalog/base"},
		},
		{
			name:     "Array of strings",
			input:    []interface{}{"catalog/base", "catalog/network"},
			expected: []string{"catalog/base", "catalog/network"},
		},
		{
			name:     "Empty array",
			input:    []interface{}{},
			expected: nil, // Function returns nil for empty input
		},
		{
			name:     "Array with mixed types (strings only extracted)",
			input:    []interface{}{"catalog/base", 123, "catalog/network"},
			expected: []string{"catalog/base", "catalog/network"},
		},
		{
			name:     "Nil value",
			input:    nil,
			expected: nil, // Function returns nil for nil input
		},
		{
			name:     "Integer (non-string, non-array)",
			input:    123,
			expected: nil, // Function returns nil for non-string/array
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractImportStringsHelper(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildNodesFromImportPaths tests node building from import paths.
func TestBuildNodesFromImportPaths(t *testing.T) {
	tests := []struct {
		name        string
		imports     []string
		expectNodes int
	}{
		{
			name:        "Empty imports",
			imports:     []string{},
			expectNodes: 0,
		},
		{
			name:        "Single import",
			imports:     []string{"catalog/base"},
			expectNodes: 1,
		},
		{
			name:        "Multiple imports",
			imports:     []string{"catalog/base", "catalog/network", "catalog/security"},
			expectNodes: 3,
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: "/tmp/stacks",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visited := make(map[string]bool)
			cache := make(map[string][]string)
			result := buildNodesFromImportPaths(tt.imports, "/tmp/stacks/stack.yaml", atmosConfig, visited, cache)
			assert.Equal(t, tt.expectNodes, len(result))
		})
	}
}

// TestCircularImportDetection tests that circular imports are detected.
func TestCircularImportDetection(t *testing.T) {
	// Create temporary directory for test files.
	tmpDir := t.TempDir()

	// Create circular import files.
	file1Content := `
imports:
  - file2
vars:
  name: file1
`
	file2Content := `
imports:
  - file1
vars:
  name: file2
`

	file1Path := filepath.Join(tmpDir, "file1.yaml")
	file2Path := filepath.Join(tmpDir, "file2.yaml")

	err := os.WriteFile(file1Path, []byte(file1Content), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(file2Path, []byte(file2Content), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	// Test circular detection.
	visited := make(map[string]bool)
	cache := make(map[string][]string)

	visited[file1Path] = true
	nodes := resolveImportFileImports(file1Path, atmosConfig, visited, cache)

	// Should have one node for file2.
	assert.Equal(t, 1, len(nodes))

	// The node for file1 should be marked as circular when file2 tries to import it.
	if len(nodes) > 0 && len(nodes[0].Children) > 0 {
		assert.True(t, nodes[0].Children[0].Circular, "Expected circular reference to be detected")
	}
}

// TestImportCaching tests that import caching works correctly.
func TestImportCaching(t *testing.T) {
	tmpDir := t.TempDir()

	baseContent := `
imports:
  - network
vars:
  env: prod
`
	basePath := filepath.Join(tmpDir, "base.yaml")
	err := os.WriteFile(basePath, []byte(baseContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	visited := make(map[string]bool)
	cache := make(map[string][]string)

	// First call - should read from file and populate cache.
	nodes1 := resolveImportFileImports(basePath, atmosConfig, visited, cache)
	assert.Equal(t, 1, len(nodes1))
	assert.Contains(t, cache, basePath)

	// Second call - should use cached value.
	visited2 := make(map[string]bool)
	nodes2 := resolveImportFileImports(basePath, atmosConfig, visited2, cache)
	assert.Equal(t, 1, len(nodes2))
}
