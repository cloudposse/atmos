package list

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFilterAndListVendor tests the vendor listing functionality

func TestFilterAndListVendor(t *testing.T) {
	tempDir := t.TempDir()

	vendorDir := filepath.Join(tempDir, "vendor.d")
	err := os.Mkdir(vendorDir, 0o755)
	if err != nil {
		t.Fatalf("Error creating vendor dir: %v", err)
	}

	componentsDir := filepath.Join(tempDir, "components")
	err = os.Mkdir(componentsDir, 0o755)
	if err != nil {
		t.Fatalf("Error creating components dir: %v", err)
	}

	terraformDir := filepath.Join(componentsDir, "terraform")
	err = os.Mkdir(terraformDir, 0o755)
	if err != nil {
		t.Fatalf("Error creating terraform dir: %v", err)
	}

	vpcDir := filepath.Join(terraformDir, "vpc", "v1")
	err = os.MkdirAll(vpcDir, 0o755)
	if err != nil {
		t.Fatalf("Error creating vpc dir: %v", err)
	}

	componentYaml := `apiVersion: atmos/v1
kind: Component
metadata:
  name: vpc
  description: VPC component
spec:
  source:
    type: git
    uri: github.com/cloudposse/terraform-aws-vpc
    version: 1.0.0
`
	err = os.WriteFile(filepath.Join(vpcDir, "component.yaml"), []byte(componentYaml), 0o644)
	if err != nil {
		t.Fatalf("Error writing component.yaml: %v", err)
	}

	vendorYaml := `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: eks
  description: EKS component
spec:
  sources:
    - component: eks/cluster
      source: github.com/cloudposse/terraform-aws-eks-cluster
      version: 1.0.0
      file: vendor.d/eks
      targets:
        - components/terraform/eks/cluster
    - component: ecs/cluster
      source: github.com/cloudposse/terraform-aws-ecs-cluster
      version: 1.0.0
      file: vendor.d/ecs
      targets:
        - components/terraform/ecs/cluster
`
	err = os.WriteFile(filepath.Join(vendorDir, "vendor.yaml"), []byte(vendorYaml), 0o644)
	if err != nil {
		t.Fatalf("Error writing vendor.yaml: %v", err)
	}

	atmosConfig := schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
		Vendor: schema.Vendor{
			BasePath: "vendor.d",
			List: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{
						Name:  "Component",
						Value: "{{ .atmos_component }}",
					},
					{
						Name:  "Type",
						Value: "{{ .atmos_vendor_type }}",
					},
					{
						Name:  "Manifest",
						Value: "{{ .atmos_vendor_file }}",
					},
					{
						Name:  "Folder",
						Value: "{{ .atmos_vendor_target }}",
					},
				},
			},
		},
	}

	// Test table format (default)
	t.Run("TableFormat", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr: string(format.FormatTable),
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "Component")
		assert.Contains(t, output, "Type")
		assert.Contains(t, output, "Manifest")
		assert.Contains(t, output, "Folder")
		assert.Contains(t, output, "vpc/v1")
		assert.Contains(t, output, "Component")
		assert.Contains(t, output, "eks/cluster")
		assert.Contains(t, output, "Vendor Manifest")
		assert.Contains(t, output, "ecs/cluster")
	})

	// Test JSON format
	t.Run("JSONFormat", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr: string(format.FormatJSON),
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "\"Component\": \"vpc/v1\"")
		assert.Contains(t, output, "\"Type\": \"Component Manifest\"")
		assert.Contains(t, output, "\"Component\": \"eks/cluster\"")
		assert.Contains(t, output, "\"Type\": \"Vendor Manifest\"")
		assert.Contains(t, output, "\"Component\": \"ecs/cluster\"")
	})

	// Test YAML format
	t.Run("YAMLFormat", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr: string(format.FormatYAML),
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "component: vpc/v1")
		assert.Contains(t, output, "type: Component Manifest")
		assert.Contains(t, output, "component: eks/cluster")
		assert.Contains(t, output, "type: Vendor Manifest")
		assert.Contains(t, output, "component: ecs/cluster")
	})

	// Test CSV format
	t.Run("CSVFormat", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr: string(format.FormatCSV),
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "Component,Type,Manifest,Folder")
		assert.Contains(t, output, "vpc/v1,Component Manifest")
		assert.Contains(t, output, "eks/cluster,Vendor Manifest")
		assert.Contains(t, output, "ecs/cluster,Vendor Manifest")
	})

	// Test TSV format
	t.Run("TSVFormat", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr: string(format.FormatTSV),
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "Component\tType\tManifest\tFolder")
		assert.Contains(t, output, "vpc/v1\tComponent Manifest")
		assert.Contains(t, output, "eks/cluster\tVendor Manifest")
		assert.Contains(t, output, "ecs/cluster\tVendor Manifest")
	})

	// Test stack pattern filtering
	t.Run("StackPatternFiltering", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr:    string(format.FormatTable),
			StackPattern: "vpc*",
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "vpc/v1")
		assert.NotContains(t, output, "eks/cluster")
		assert.NotContains(t, output, "ecs/cluster")
	})

	// Test multiple stack patterns
	t.Run("MultipleStackPatterns", func(t *testing.T) {
		options := &FilterOptions{
			FormatStr:    string(format.FormatTable),
			StackPattern: "vpc*,ecs*",
		}

		output, err := FilterAndListVendor(&atmosConfig, options)
		assert.NoError(t, err)
		assert.Contains(t, output, "vpc/v1")
		assert.NotContains(t, output, "eks/cluster")
		assert.Contains(t, output, "ecs/cluster")
	})

	// Test error when vendor.base_path not set
	t.Run("ErrorVendorBasepathNotSet", func(t *testing.T) {
		invalidConfig := atmosConfig
		invalidConfig.Vendor.BasePath = ""

		options := &FilterOptions{
			FormatStr: string(format.FormatTable),
		}

		_, err := FilterAndListVendor(&invalidConfig, options)
		assert.Error(t, err)
		assert.Equal(t, ErrVendorBasepathNotSet, err)
	})
}

// Helper function to create nested directories and optionally a file at the deepest level.
func createNestedDirsWithFile(t *testing.T, basePath string, depth int, filename string) string {
	t.Helper()
	currentPath := basePath
	for i := 0; i < depth; i++ {
		currentPath = filepath.Join(currentPath, fmt.Sprintf("d%d", i+1))
	}
	err := os.MkdirAll(currentPath, 0o755)
	require.NoError(t, err, "Failed to create nested directories")

	finalPath := currentPath
	if filename != "" {
		filePath := filepath.Join(currentPath, filename)
		writeErr := os.WriteFile(filePath, []byte("metadata:\n  name: test-component"), 0o644)
		require.NoError(t, writeErr, "Failed to create file in nested directory")
		finalPath = filePath
	}
	return finalPath
}

// Helper function to create a dummy manifest file.
func createDummyManifest(t *testing.T, path string) {
	t.Helper()
	content := "metadata:\n  name: test-component\nvars:\n  region: us-east-1"
	err := os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err, "Failed to write dummy manifest file")
}

// Helper function to create a test file with specific content and permissions.
func createTestFile(t *testing.T, path string, content string, perms fs.FileMode) {
	t.Helper()
	err := os.WriteFile(path, []byte(content), perms)
	require.NoError(t, err, "Failed to create test file")
	// Ensure permissions are set correctly (WriteFile might be affected by umask).
	err = os.Chmod(path, perms)
	require.NoError(t, err, "Failed to set file permissions")
}

// TestBuildVendorDataMap covers all key logic and capitalizeKeys branches.
func TestBuildVendorDataMap(t *testing.T) {
	t.Run("Component key present, capitalizeKeys true", func(t *testing.T) {
		rows := []map[string]interface{}{
			{ColumnNameComponent: "foo", "X": 1},
		}
		result := buildVendorDataMap(rows, true)
		assert.Contains(t, result, "foo")
		m, ok := result["foo"].(map[string]interface{})
		assert.True(t, ok)
		assert.Contains(t, m, ColumnNameComponent)
		assert.Contains(t, m, "X")
	})
	t.Run("Component key present, capitalizeKeys false", func(t *testing.T) {
		rows := []map[string]interface{}{
			{ColumnNameComponent: "FOO", "Y": 2},
		}
		result := buildVendorDataMap(rows, false)
		assert.Contains(t, result, "FOO")
		m, ok := result["FOO"].(map[string]interface{})
		assert.True(t, ok)
		assert.Contains(t, m, strings.ToLower(ColumnNameComponent))
		assert.Contains(t, m, "y")
	})
	t.Run("Component key missing", func(t *testing.T) {
		rows := []map[string]interface{}{
			{"A": 1},
		}
		result := buildVendorDataMap(rows, true)
		assert.Contains(t, result, "vendor_0")
	})
	t.Run("Component key empty string", func(t *testing.T) {
		rows := []map[string]interface{}{
			{ColumnNameComponent: "", "B": 2},
		}
		result := buildVendorDataMap(rows, true)
		assert.Contains(t, result, "vendor_0")
	})
}

// TestBuildVendorCSVTSV covers header/value logic, delimiters, and empty/missing fields.
func TestBuildVendorCSVTSV(t *testing.T) {
	headers := []string{"A", "B"}
	rows := []map[string]interface{}{
		{"A": "foo", "B": "bar"},
		{"A": "baz"}, // missing B
		{"B": "qux"}, // missing A
	}
	t.Run("CSV output", func(t *testing.T) {
		csv := buildVendorCSVTSV(headers, rows, ",")
		assert.Contains(t, csv, "A,B")
		assert.Contains(t, csv, "foo,bar")
		assert.Contains(t, csv, "baz,")
		assert.Contains(t, csv, ",qux")
	})
	t.Run("TSV output", func(t *testing.T) {
		tsv := buildVendorCSVTSV(headers, rows, "\t")
		assert.Contains(t, tsv, "A\tB")
		assert.Contains(t, tsv, "foo\tbar")
		assert.Contains(t, tsv, "baz\t")
		assert.Contains(t, tsv, "\tqux")
	})
	t.Run("Empty rows", func(t *testing.T) {
		empty := buildVendorCSVTSV(headers, nil, ",")
		assert.Contains(t, empty, "A,B\n")
	})
}

// TestRenderVendorTableOutput covers empty/filled rows and missing fields.
func TestRenderVendorTableOutput(t *testing.T) {
	headers := []string{"A", "B"}
	rows := []map[string]interface{}{
		{"A": "foo", "B": "bar"},
		{"A": "baz"}, // missing B
		{"B": "qux"}, // missing A
	}
	output := renderVendorTableOutput(headers, rows)
	assert.Contains(t, output, "A")
	assert.Contains(t, output, "B")
	assert.Contains(t, output, "foo")
	assert.Contains(t, output, "bar")
	assert.Contains(t, output, "baz")
	assert.Contains(t, output, "qux")

	t.Run("Empty rows", func(t *testing.T) {
		empty := renderVendorTableOutput(headers, nil)
		assert.Contains(t, empty, "A")
		assert.Contains(t, empty, "B")
	})
}

// TestFindComponentManifestInComponent tests the recursive search logic.
func TestFindComponentManifestInComponent(t *testing.T) {
	maxDepth := 10 // Must match the value in findComponentManifestInComponent

	testCases := []struct {
		name          string
		setup         func(t *testing.T, tempDir string) string // Returns expected path or empty
		componentPath func(t *testing.T, tempDir string) string // Returns path to search
		expectError   error
	}{
		{
			name: "ManifestAtRoot",
			setup: func(t *testing.T, tempDir string) string {
				expectedPath := filepath.Join(tempDir, "component.yaml")
				createDummyManifest(t, expectedPath)
				return expectedPath
			},
			componentPath: func(t *testing.T, tempDir string) string { return tempDir },
			expectError:   nil,
		},
		{
			name: "ManifestOneLevelDeep",
			setup: func(t *testing.T, tempDir string) string {
				expectedPath := createNestedDirsWithFile(t, tempDir, 1, "component.yaml")
				return expectedPath
			},
			componentPath: func(t *testing.T, tempDir string) string { return tempDir },
			expectError:   nil,
		},
		{
			name: "ManifestFiveLevelsDeep",
			setup: func(t *testing.T, tempDir string) string {
				expectedPath := createNestedDirsWithFile(t, tempDir, 5, "component.yaml")
				return expectedPath
			},
			componentPath: func(t *testing.T, tempDir string) string { return tempDir },
			expectError:   nil,
		},
		{
			name: "ManifestAtMaxDepth",
			setup: func(t *testing.T, tempDir string) string {
				expectedPath := createNestedDirsWithFile(t, tempDir, maxDepth-1, "component.yaml")
				return expectedPath
			},
			componentPath: func(t *testing.T, tempDir string) string { return tempDir },
			expectError:   nil,
		},
		{
			name: "ManifestDeeperThanMaxDepth",
			setup: func(t *testing.T, tempDir string) string {
				_ = createNestedDirsWithFile(t, tempDir, maxDepth+1, "component.yaml")
				return "" // Expected path is empty string
			},
			componentPath: func(t *testing.T, tempDir string) string { return tempDir },
			expectError:   ErrComponentManifestNotFound,
		},
		{
			name: "NoManifestFound",
			setup: func(t *testing.T, tempDir string) string {
				_ = createNestedDirsWithFile(t, tempDir, 2, "someotherfile.txt")
				return ""
			},
			componentPath: func(t *testing.T, tempDir string) string { return tempDir },
			expectError:   ErrComponentManifestNotFound,
		},
		{
			name:          "NonExistentComponentPath",
			setup:         func(t *testing.T, tempDir string) string { return "" },
			componentPath: func(t *testing.T, tempDir string) string { return filepath.Join(tempDir, "does_not_exist") },
			// Expect a specific type of error, typically fs.ErrNotExist wrapped
			expectError: fs.ErrNotExist,
		},
		{
			name: "ManifestIsDirectory",
			setup: func(t *testing.T, tempDir string) string {
				err := os.Mkdir(filepath.Join(tempDir, "component.yaml"), 0o755)
				require.NoError(t, err)
				return ""
			},
			componentPath: func(t *testing.T, tempDir string) string { return tempDir },
			expectError:   ErrComponentManifestNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			expectedPath := tc.setup(t, tempDir)
			componentPath := tc.componentPath(t, tempDir)

			// Ensure paths are absolute for consistency
			if expectedPath != "" {
				absExpectedPath, err := filepath.Abs(expectedPath)
				require.NoError(t, err)
				expectedPath = absExpectedPath
			}
			absComponentPath, err := filepath.Abs(componentPath)
			// For the non-existent path case, Abs will return an error, skip it
			if err == nil {
				componentPath = absComponentPath
			}

			actualPath, err := findComponentManifestInComponent(componentPath)

			if tc.expectError != nil {
				assert.Error(t, err)
				// Use errors.Is for checking wrapped standard errors like fs.ErrNotExist
				if errors.Is(tc.expectError, fs.ErrNotExist) {
					assert.True(t, errors.Is(err, fs.ErrNotExist), "Expected fs.ErrNotExist, got: %v", err)
				} else {
					assert.Equal(t, tc.expectError, err)
				}
				assert.Empty(t, actualPath)
			} else {
				assert.NoError(t, err)
				// Normalize paths for comparison (e.g., clean, make absolute)
				assert.Equal(t, expectedPath, actualPath)
			}
		})
	}
}

// TestReadComponentManifest tests reading and parsing component.yaml.
func TestReadComponentManifest(t *testing.T) {
	testCases := []struct {
		name        string
		setup       func(t *testing.T, tempDir string) string // returns path to file
		content     string
		perms       fs.FileMode
		expectError bool
		errorType   error // Specific error type to check if expectError is true
		expectData  *schema.ComponentManifest
	}{
		{
			name: "ValidManifest",
			content: `
kind: Component
metadata:
  name: test-comp
  description: A description
vars:
  key1: value1
  key2: 123
`,
			perms:       0o644,
			expectError: false,
			expectData: &schema.ComponentManifest{
				Kind:     "Component",
				Metadata: map[string]any{"name": "test-comp", "description": "A description"},
				Vars:     map[string]any{"key1": "value1", "key2": 123},
			},
		},
		{
			name:        "InvalidYAML",
			content:     "metadata: { name: test",
			perms:       0o644,
			expectError: true,
			errorType:   errors.New("invalid component manifest: unexpected format"), // Check for error message
		},
		{
			name: "FileNotFound",
			setup: func(t *testing.T, tempDir string) string {
				return filepath.Join(tempDir, "nonexistent.yaml") // Don't create it
			},
			perms:       0o644,
			expectError: true,
			errorType:   os.ErrNotExist,
		},
		{
			name:        "EmptyFile",
			content:     "",
			perms:       0o644,
			expectError: true,
			errorType:   errors.New("unexpected format in component manifest"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			var filePath string
			if tc.setup != nil {
				filePath = tc.setup(t, tempDir)
			} else {
				filePath = filepath.Join(tempDir, "test_component.yaml")
				createTestFile(t, filePath, tc.content, tc.perms)
			}

			data, err := readComponentManifest(filePath)

			if tc.expectError {
				assertExpectedError(t, err, tc.errorType)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectData, data)
			}
		})
	}
}

// assertExpectedError validates that an error matches the expected error type or message.
func assertExpectedError(t *testing.T, err, expectedError error) {
	assert.Error(t, err)
	if expectedError == nil {
		return
	}

	if errors.Is(expectedError, os.ErrNotExist) || errors.Is(expectedError, os.ErrPermission) {
		assert.True(t, errors.Is(err, expectedError), "Expected error type %T, got: %v", expectedError, err)
		return
	}

	// For other error messages, just check that the error message contains the expected text
	assert.Contains(t, err.Error(), expectedError.Error(), "Expected error message containing '%s', got: %v", expectedError.Error(), err)
}

// TestFormatTargetFolder tests placeholder replacement.
func TestFormatTargetFolder(t *testing.T) {
	testCases := []struct {
		name      string
		target    string
		component string
		version   string
		expected  string
	}{
		{"ReplaceBoth", "path/{{ .Component }}/{{ .Version }}", "comp", "v1", "path/comp/v1"},
		{"ReplaceBothSpaceless", "path/{{.Component}}/{{.Version}}", "comp", "v1", "path/comp/v1"},
		{"ReplaceComponentOnly", "path/{{ .Component }}/fixed", "comp", "v1", "path/comp/fixed"},
		{"ReplaceVersionOnly", "path/fixed/{{ .Version }}", "comp", "v1", "path/fixed/v1"},
		{"VersionEmpty", "path/{{ .Component }}/{{ .Version }}", "comp", "", "path/comp/{{ .Version }}"},
		{"VersionEmptySpaceless", "path/{{.Component}}/{{.Version}}", "comp", "", "path/comp/{{.Version}}"},
		{"NoPlaceholders", "path/fixed/fixed", "comp", "v1", "path/fixed/fixed"},
		{"EmptyTarget", "", "comp", "v1", ""},
		{"OnlyComponentPlaceholder", "{{ .Component }}", "comp", "v1", "comp"},
		{"OnlyVersionPlaceholder", "{{ .Version }}", "comp", "v1", "v1"},
		{"OnlyVersionPlaceholderEmptyVersion", "{{ .Version }}", "comp", "", "{{ .Version }}"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := formatTargetFolder(tc.target, tc.component, tc.version)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

// TestApplyVendorFilters tests the filtering logic.
func TestApplyVendorFilters(t *testing.T) {
	initialInfos := []VendorInfo{
		{Component: "vpc", Type: "terraform", Folder: "components/terraform/vpc"},
		{Component: "eks", Type: "helmfile", Folder: "components/helmfile/eks"},
		{Component: "rds", Type: "terraform", Folder: "components/terraform/rds"},
		{Component: "app", Type: "helmfile", Folder: "components/helmfile/app"},
		{Component: "ecs", Type: "terraform", Folder: "components/terraform/ecs"},
	}

	testCases := []struct {
		name     string
		options  FilterOptions
		input    []VendorInfo
		expected []VendorInfo
	}{
		{
			name:     "NoFilters",
			options:  FilterOptions{},
			input:    initialInfos,
			expected: initialInfos,
		},
		{
			name:     "FilterComponentExactMatch",
			options:  FilterOptions{StackPattern: "vpc"},
			input:    initialInfos,
			expected: []VendorInfo{initialInfos[0]},
		},
		{
			name:     "FilterComponentNoMatch",
			options:  FilterOptions{StackPattern: "nomatch"},
			input:    initialInfos,
			expected: []VendorInfo{},
		},
		{
			name:     "FilterMultiplePatterns",
			options:  FilterOptions{StackPattern: "vpc,eks"},
			input:    initialInfos,
			expected: []VendorInfo{initialInfos[0], initialInfos[1]},
		},
		{
			name:     "FilterSpecialCaseEcs",
			options:  FilterOptions{StackPattern: "ecs"},
			input:    initialInfos,
			expected: []VendorInfo{initialInfos[4]},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := applyVendorFilters(tc.input, tc.options.StackPattern)

			// For the empty slice case, check length instead of direct equality
			if len(tc.expected) == 0 {
				assert.Empty(t, actual, "Expected empty result")
			} else {
				assert.Equal(t, tc.expected, actual)
			}
		})
	}
}
