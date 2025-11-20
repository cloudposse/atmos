package importresolver

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
				return
			}

			assert.NotEmpty(t, result)
			if tt.expectedFolder == "" {
				return
			}

			found := false
			for _, folder := range result {
				if folder == tt.expectedFolder {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected folder %s not found in result", tt.expectedFolder)
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

// TestResolveImportFileImports_FileNotFound tests handling of missing import file.
func TestResolveImportFileImports_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	visited := make(map[string]bool)
	cache := make(map[string][]string)

	// Try to resolve imports from non-existent file.
	filePath := filepath.Join(tmpDir, "nonexistent.yaml")
	nodes := resolveImportFileImports(filePath, atmosConfig, visited, cache)

	// Should return nil (no nodes) when file can't be read.
	assert.Nil(t, nodes)
}

// TestResolveImportFileImports_InvalidYAML tests handling of invalid YAML file.
func TestResolveImportFileImports_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	invalidContent := `
imports: [unclosed
vars: broken
`
	filePath := filepath.Join(tmpDir, "invalid.yaml")
	err := os.WriteFile(filePath, []byte(invalidContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	visited := make(map[string]bool)
	cache := make(map[string][]string)

	nodes := resolveImportFileImports(filePath, atmosConfig, visited, cache)

	// Should return nil when YAML parsing fails.
	assert.Nil(t, nodes)
}

// TestResolveImportFileImports_EmptyFile tests file with no imports.
func TestResolveImportFileImports_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()

	emptyContent := `
vars:
  environment: prod
`
	filePath := filepath.Join(tmpDir, "empty.yaml")
	err := os.WriteFile(filePath, []byte(emptyContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	visited := make(map[string]bool)
	cache := make(map[string][]string)

	nodes := resolveImportFileImports(filePath, atmosConfig, visited, cache)

	// Should handle empty imports gracefully.
	assert.Empty(t, nodes)
	assert.Contains(t, cache, filePath)
	assert.Empty(t, cache[filePath])
}

// TestResolveImportFileImports_DeepRecursion tests deep import chains.
func TestResolveImportFileImports_DeepRecursion(t *testing.T) {
	tmpDir := t.TempDir()

	// Create chain: file1 → file2 → file3 → file4 → file5.
	for i := 1; i <= 5; i++ {
		var content string
		if i < 5 {
			content = `
imports:
  - file` + string(rune('0'+i+1)) + `
vars:
  level: ` + string(rune('0'+i))
		} else {
			content = `
vars:
  level: 5
`
		}
		filePath := filepath.Join(tmpDir, "file"+string(rune('0'+i))+".yaml")
		err := os.WriteFile(filePath, []byte(content), 0o644)
		require.NoError(t, err)
	}

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	visited := make(map[string]bool)
	cache := make(map[string][]string)

	file1Path := filepath.Join(tmpDir, "file1.yaml")
	nodes := resolveImportFileImports(file1Path, atmosConfig, visited, cache)

	// Should resolve all levels.
	assert.Len(t, nodes, 1)
	node := nodes[0]
	assert.Equal(t, "file2", node.Path)

	// Navigate through all levels.
	for i := 2; i < 5; i++ {
		assert.Len(t, node.Children, 1)
		node = node.Children[0]
		assert.Equal(t, "file"+string(rune('0'+i+1)), node.Path)
	}

	// Last node should have no children.
	assert.Empty(t, node.Children)
}

// TestResolveImportFileImports_VisitedBacktracking tests visited map backtracking.
func TestResolveImportFileImports_VisitedBacktracking(t *testing.T) {
	tmpDir := t.TempDir()

	// Create structure:
	//   parent imports both a and b
	//   a imports common
	//   b imports common
	// 'common' should appear in both branches (not marked circular).

	parentContent := `
imports:
  - a
  - b
`
	err := os.WriteFile(filepath.Join(tmpDir, "parent.yaml"), []byte(parentContent), 0o644)
	require.NoError(t, err)

	aContent := `
imports:
  - common
vars:
  name: a
`
	err = os.WriteFile(filepath.Join(tmpDir, "a.yaml"), []byte(aContent), 0o644)
	require.NoError(t, err)

	bContent := `
imports:
  - common
vars:
  name: b
`
	err = os.WriteFile(filepath.Join(tmpDir, "b.yaml"), []byte(bContent), 0o644)
	require.NoError(t, err)

	commonContent := `
vars:
  shared: true
`
	err = os.WriteFile(filepath.Join(tmpDir, "common.yaml"), []byte(commonContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	visited := make(map[string]bool)
	cache := make(map[string][]string)

	parentPath := filepath.Join(tmpDir, "parent.yaml")
	nodes := resolveImportFileImports(parentPath, atmosConfig, visited, cache)

	// Should have 2 top-level nodes (a and b).
	assert.Len(t, nodes, 2)

	// Both a and b should have 'common' as child (not marked circular).
	for _, node := range nodes {
		assert.Len(t, node.Children, 1)
		assert.Equal(t, "common", node.Children[0].Path)
		assert.False(t, node.Children[0].Circular, "Expected backtracking to allow same import in different branches")
	}
}

// TestResolveImportFileImports_WithCache tests cache population and reuse.
func TestResolveImportFileImports_WithCache(t *testing.T) {
	tmpDir := t.TempDir()

	baseContent := `
imports:
  - common
vars:
  base: true
`
	basePath := filepath.Join(tmpDir, "base.yaml")
	err := os.WriteFile(basePath, []byte(baseContent), 0o644)
	require.NoError(t, err)

	commonContent := `
vars:
  common: true
`
	commonPath := filepath.Join(tmpDir, "common.yaml")
	err = os.WriteFile(commonPath, []byte(commonContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	visited := make(map[string]bool)
	cache := make(map[string][]string)

	// First call - populates cache.
	nodes1 := resolveImportFileImports(basePath, atmosConfig, visited, cache)
	assert.Len(t, nodes1, 1)
	assert.Contains(t, cache, basePath)

	// Modify file on disk.
	err = os.WriteFile(basePath, []byte("imports:\n  - different"), 0o644)
	require.NoError(t, err)

	// Second call - should use cache (not re-read file).
	visited2 := make(map[string]bool)
	nodes2 := resolveImportFileImports(basePath, atmosConfig, visited2, cache)
	assert.Len(t, nodes2, 1)
	assert.Equal(t, "common", nodes2[0].Path, "Expected cached import, not re-read file")
}

// TestBuildImportTreeFromChain_LongChain tests processing of long import chains.
func TestBuildImportTreeFromChain_LongChain(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: "/tmp/stacks",
	}

	// Create a chain with 10 imports.
	importChain := []string{"/tmp/stacks/parent.yaml"}
	for i := 1; i <= 10; i++ {
		importChain = append(importChain, "/tmp/stacks/import"+string(rune('0'+i))+".yaml")
	}

	nodes := buildImportTreeFromChain(importChain, atmosConfig)

	// Should have 10 nodes (skipping first element which is parent).
	assert.Len(t, nodes, 10)

	// Verify paths are correctly stripped.
	for i, node := range nodes {
		expected := "import" + string(rune('0'+i+1))
		assert.Equal(t, expected, node.Path)
	}
}

// TestBuildImportTreeFromChain_DuplicateInChain tests duplicate imports in chain.
func TestBuildImportTreeFromChain_DuplicateInChain(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files.
	baseContent := `vars: {}`
	basePath := filepath.Join(tmpDir, "base.yaml")
	err := os.WriteFile(basePath, []byte(baseContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	// Import chain with duplicate.
	importChain := []string{
		filepath.Join(tmpDir, "parent.yaml"),
		basePath,
		filepath.Join(tmpDir, "other.yaml"),
		basePath, // Duplicate - visited is cleared after each import so not marked circular
	}

	nodes := buildImportTreeFromChain(importChain, atmosConfig)

	// Should have 3 nodes.
	assert.Len(t, nodes, 3)

	// All nodes get their paths.
	assert.Equal(t, "base", nodes[0].Path)
	assert.Equal(t, "other", nodes[1].Path)
	assert.Equal(t, "base", nodes[2].Path)

	// Duplicates in chain are allowed because visited is cleared after processing each import.
	// This allows the same file to appear multiple times in the merge chain.
	assert.False(t, nodes[0].Circular)
	assert.False(t, nodes[2].Circular)
}

// TestBuildNodesFromImportPaths_LargeImportList tests handling of many imports.
func TestBuildNodesFromImportPaths_LargeImportList(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	// Create 20 import paths.
	var imports []string
	for i := 1; i <= 20; i++ {
		imports = append(imports, "catalog/import"+string(rune('0'+i)))
	}

	visited := make(map[string]bool)
	cache := make(map[string][]string)

	nodes := buildNodesFromImportPaths(imports, filepath.Join(tmpDir, "parent.yaml"), atmosConfig, visited, cache)

	// Should have 20 nodes.
	assert.Len(t, nodes, 20)

	// Verify all imports are present.
	for i, node := range nodes {
		expected := "catalog/import" + string(rune('0'+i+1))
		assert.Equal(t, expected, node.Path)
	}
}

// TestBuildNodesFromImportPaths_WithRealFiles tests node building with actual files.
func TestBuildNodesFromImportPaths_WithRealFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create catalog directory.
	catalogDir := filepath.Join(tmpDir, "catalog")
	err := os.MkdirAll(catalogDir, 0o755)
	require.NoError(t, err)

	// Create base.yaml with import.
	baseContent := `
imports:
  - catalog/network
vars:
  base: true
`
	basePath := filepath.Join(catalogDir, "base.yaml")
	err = os.WriteFile(basePath, []byte(baseContent), 0o644)
	require.NoError(t, err)

	// Create network.yaml (no imports).
	networkContent := `
vars:
  network: true
`
	networkPath := filepath.Join(catalogDir, "network.yaml")
	err = os.WriteFile(networkPath, []byte(networkContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	imports := []string{"catalog/base"}
	visited := make(map[string]bool)
	cache := make(map[string][]string)

	nodes := buildNodesFromImportPaths(imports, filepath.Join(tmpDir, "parent.yaml"), atmosConfig, visited, cache)

	// Should have 1 top-level node.
	assert.Len(t, nodes, 1)
	assert.Equal(t, "catalog/base", nodes[0].Path)

	// base.yaml should have network as child.
	assert.Len(t, nodes[0].Children, 1)
	assert.Equal(t, "catalog/network", nodes[0].Children[0].Path)
}
