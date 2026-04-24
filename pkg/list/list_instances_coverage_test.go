//nolint:dupl // Test structure similarity is intentional for comprehensive coverage
package list

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/list/column"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/tests"
)

// TestUploadInstances tests the uploadInstances() wrapper function.
func TestUploadInstances(t *testing.T) {
	// This tests the production wrapper that uses default implementations.
	// It requires a git repository to function.
	tests.RequireGitRepository(t)

	instances := []schema.Instance{
		{Component: "vpc", Stack: "dev"},
	}

	// Call the wrapper function.
	// This may error if Pro is not configured, but should not panic.
	err := uploadInstances(instances)

	// We expect an error because Pro API is likely not configured in test environment.
	// The important thing is that the function executes without panic.
	// The underlying uploadInstancesWithDeps() is already tested at 100% with mocks.
	_ = err
}

// TestExecuteListInstancesCmd tests the main command entry point with real fixtures.
func TestExecuteListInstancesCmd(t *testing.T) {
	// Initialize I/O and UI contexts for testing.
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("failed to initialize I/O context: %v", err)
	}
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	// Use actual test fixture for integration test.
	fixturePath := "../../tests/fixtures/scenarios/complete"
	tests.RequireFilePath(t, fixturePath, "test fixture directory")

	// Create command with flags.
	cmd := &cobra.Command{}
	cmd.Flags().Bool("upload", false, "Upload instances to Atmos Pro")
	cmd.Flags().String("format", "table", "Output format")

	info := &schema.ConfigAndStacksInfo{
		BasePath: fixturePath,
	}

	// Execute command - should successfully list instances.
	err = ExecuteListInstancesCmd(&InstancesCommandOptions{
		Info:        info,
		Cmd:         cmd,
		Args:        []string{},
		ShowImports: false,
		ColumnsFlag: []string{},
		FilterSpec:  "",
		SortSpec:    "",
	})

	// Should succeed with valid fixture.
	assert.NoError(t, err)
}

// TestExecuteListInstancesCmd_InvalidConfig tests error handling for invalid config.
func TestExecuteListInstancesCmd_InvalidConfig(t *testing.T) {
	// Create command with flags.
	cmd := &cobra.Command{}
	cmd.Flags().Bool("upload", false, "Upload instances to Atmos Pro")
	cmd.Flags().String("format", "table", "Output format")

	// Use invalid config to trigger error path.
	info := &schema.ConfigAndStacksInfo{
		BasePath: "/nonexistent/path",
	}

	// Execute command - will error but won't panic.
	err := ExecuteListInstancesCmd(&InstancesCommandOptions{
		Info:        info,
		Cmd:         cmd,
		Args:        []string{},
		ShowImports: false,
		ColumnsFlag: []string{},
		FilterSpec:  "",
		SortSpec:    "",
	})

	// Error is expected with invalid config.
	assert.Error(t, err)
}

// TestExecuteListInstancesCmd_UploadPath tests the upload branch.
func TestExecuteListInstancesCmd_UploadPath(t *testing.T) {
	// Test that upload flag parsing works.
	cmd := &cobra.Command{}
	cmd.Flags().Bool("upload", true, "Upload instances to Atmos Pro")
	cmd.Flags().String("format", "table", "Output format")

	info := &schema.ConfigAndStacksInfo{
		BasePath: "/nonexistent/path",
	}

	// Execute with upload enabled - will error in config loading before upload.
	err := ExecuteListInstancesCmd(&InstancesCommandOptions{
		Info:        info,
		Cmd:         cmd,
		Args:        []string{},
		ShowImports: false,
		ColumnsFlag: []string{},
		FilterSpec:  "",
		SortSpec:    "",
	})

	// Error is expected (config load will fail).
	assert.Error(t, err)
}

// TestParseColumnsFlag tests parsing column specifications from CLI flags.
func TestParseColumnsFlag(t *testing.T) {
	tests := []struct {
		name        string
		columnsFlag []string
		expected    []column.Config
		expectErr   bool
		errContains string
	}{
		{
			name:        "empty flag returns defaults",
			columnsFlag: []string{},
			expected:    defaultInstanceColumns,
			expectErr:   false,
		},
		{
			name:        "nil flag returns defaults",
			columnsFlag: nil,
			expected:    defaultInstanceColumns,
			expectErr:   false,
		},
		{
			name:        "valid single column",
			columnsFlag: []string{"Stack={{ .stack }}"},
			expected: []column.Config{
				{Name: "Stack", Value: "{{ .stack }}"},
			},
			expectErr: false,
		},
		{
			name:        "valid multiple columns",
			columnsFlag: []string{"Stack={{ .stack }}", "Component={{ .component }}"},
			expected: []column.Config{
				{Name: "Stack", Value: "{{ .stack }}"},
				{Name: "Component", Value: "{{ .component }}"},
			},
			expectErr: false,
		},
		{
			name:        "column with multiple equals signs in template",
			columnsFlag: []string{"Check={{ if eq .enabled true }}yes{{ end }}"},
			expected: []column.Config{
				{Name: "Check", Value: "{{ if eq .enabled true }}yes{{ end }}"},
			},
			expectErr: false,
		},
		{
			name:        "missing equals sign",
			columnsFlag: []string{"InvalidSpec"},
			expectErr:   true,
			errContains: "must be in format 'Name=Template'",
		},
		{
			name:        "empty name",
			columnsFlag: []string{"={{ .stack }}"},
			expectErr:   true,
			errContains: "has empty name",
		},
		{
			name:        "empty template",
			columnsFlag: []string{"Stack="},
			expectErr:   true,
			errContains: "has empty template",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseColumnsFlag(tc.columnsFlag)

			if tc.expectErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestGetInstanceColumns tests column configuration resolution.
func TestGetInstanceColumns(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		columnsFlag []string
		expected    []column.Config
		expectErr   bool
	}{
		{
			name: "CLI flag takes precedence over config",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{
						Columns: []schema.ListColumnConfig{
							{Name: "ConfigColumn", Value: "{{ .config }}"},
						},
					},
				},
			},
			columnsFlag: []string{"FlagColumn={{ .flag }}"},
			expected: []column.Config{
				{Name: "FlagColumn", Value: "{{ .flag }}"},
			},
			expectErr: false,
		},
		{
			name: "config columns used when no flag provided",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{
						Columns: []schema.ListColumnConfig{
							{Name: "ConfigStack", Value: "{{ .stack }}"},
							{Name: "ConfigComponent", Value: "{{ .component }}"},
						},
					},
				},
			},
			columnsFlag: nil,
			expected: []column.Config{
				{Name: "ConfigStack", Value: "{{ .stack }}"},
				{Name: "ConfigComponent", Value: "{{ .component }}"},
			},
			expectErr: false,
		},
		{
			name:        "defaults used when no flag and no config",
			atmosConfig: &schema.AtmosConfiguration{},
			columnsFlag: nil,
			expected:    defaultInstanceColumns,
			expectErr:   false,
		},
		{
			name: "defaults used when config has empty columns",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{
						Columns: []schema.ListColumnConfig{},
					},
				},
			},
			columnsFlag: nil,
			expected:    defaultInstanceColumns,
			expectErr:   false,
		},
		{
			name:        "invalid flag returns error",
			atmosConfig: &schema.AtmosConfiguration{},
			columnsFlag: []string{"InvalidSpec"},
			expectErr:   true,
		},
		{
			name: "new list.instances.columns config takes precedence over deprecated components.list.columns",
			atmosConfig: &schema.AtmosConfiguration{
				List: schema.TopLevelListConfig{
					Instances: schema.ListConfig{
						Columns: []schema.ListColumnConfig{
							{Name: "NewConfigStack", Value: "{{ .stack }}", Width: 20},
							{Name: "NewConfigComponent", Value: "{{ .component }}", Width: 30},
						},
					},
				},
				Components: schema.Components{
					List: schema.ListConfig{
						Columns: []schema.ListColumnConfig{
							{Name: "OldConfigColumn", Value: "{{ .old }}"},
						},
					},
				},
			},
			columnsFlag: nil,
			expected: []column.Config{
				{Name: "NewConfigStack", Value: "{{ .stack }}", Width: 20},
				{Name: "NewConfigComponent", Value: "{{ .component }}", Width: 30},
			},
			expectErr: false,
		},
		{
			name: "new list.instances.columns config used when no deprecated config",
			atmosConfig: &schema.AtmosConfiguration{
				List: schema.TopLevelListConfig{
					Instances: schema.ListConfig{
						Columns: []schema.ListColumnConfig{
							{Name: "InstanceColumn", Value: "{{ .instance }}"},
						},
					},
				},
			},
			columnsFlag: nil,
			expected: []column.Config{
				{Name: "InstanceColumn", Value: "{{ .instance }}"},
			},
			expectErr: false,
		},
		{
			name: "CLI flag takes precedence over new list.instances.columns config",
			atmosConfig: &schema.AtmosConfiguration{
				List: schema.TopLevelListConfig{
					Instances: schema.ListConfig{
						Columns: []schema.ListColumnConfig{
							{Name: "ConfigColumn", Value: "{{ .config }}"},
						},
					},
				},
			},
			columnsFlag: []string{"FlagColumn={{ .flag }}"},
			expected: []column.Config{
				{Name: "FlagColumn", Value: "{{ .flag }}"},
			},
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := getInstanceColumns(tc.atmosConfig, tc.columnsFlag)

			if tc.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestBuildInstanceSorters tests sorter configuration.
func TestBuildInstanceSorters(t *testing.T) {
	tests := []struct {
		name        string
		sortSpec    string
		columns     []column.Config
		expected    []*listSort.Sorter
		expectErr   bool
		errContains string
	}{
		{
			name:     "empty spec with default columns returns default sorters",
			sortSpec: "",
			columns: []column.Config{
				{Name: "Component", Value: "{{ .component }}"},
				{Name: "Stack", Value: "{{ .stack }}"},
			},
			expected: []*listSort.Sorter{
				listSort.NewSorter("Component", listSort.Ascending),
				listSort.NewSorter("Stack", listSort.Ascending),
			},
			expectErr: false,
		},
		{
			name:      "empty spec with custom columns returns nil",
			sortSpec:  "",
			columns:   []column.Config{{Name: "Custom", Value: "{{ .custom }}"}},
			expected:  nil,
			expectErr: false,
		},
		{
			name:     "explicit sort spec overrides defaults",
			sortSpec: "Stack:asc",
			columns: []column.Config{
				{Name: "Component", Value: "{{ .component }}"},
				{Name: "Stack", Value: "{{ .stack }}"},
			},
			expected: []*listSort.Sorter{
				listSort.NewSorter("Stack", listSort.Ascending),
			},
			expectErr: false,
		},
		{
			name:     "descending sort",
			sortSpec: "Component:desc",
			columns:  []column.Config{{Name: "Component", Value: "{{ .component }}"}},
			expected: []*listSort.Sorter{
				listSort.NewSorter("Component", listSort.Descending),
			},
			expectErr: false,
		},
		{
			name:        "invalid sort spec format",
			sortSpec:    "InvalidFormat",
			columns:     []column.Config{{Name: "Component", Value: "{{ .component }}"}},
			expectErr:   true,
			errContains: "expected format 'column:order'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := buildInstanceSorters(tc.sortSpec, tc.columns)

			if tc.expectErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			if tc.expected == nil {
				assert.Nil(t, result)
				return
			}

			require.Len(t, result, len(tc.expected))
			for i, s := range result {
				assert.Equal(t, tc.expected[i].Column, s.Column)
				assert.Equal(t, tc.expected[i].Order, s.Order)
			}
		})
	}
}

// TestExecuteListInstancesCmd_MatrixFormatRejectsUpload tests that matrix format rejects --upload.
func TestExecuteListInstancesCmd_MatrixFormatRejectsUpload(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("upload", true, "Upload instances to Atmos Pro")
	cmd.Flags().String("format", "matrix", "Output format")

	info := &schema.ConfigAndStacksInfo{
		BasePath: "../../tests/fixtures/scenarios/complete",
	}

	err := ExecuteListInstancesCmd(&InstancesCommandOptions{
		Info: info,
		Cmd:  cmd,
		Args: []string{},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.Contains(t, err.Error(), "--upload is not supported with --format=matrix")
}

// TestExecuteListInstancesCmd_MatrixFormat tests matrix format with valid fixture.
func TestExecuteListInstancesCmd_MatrixFormat(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	fixturePath := "../../tests/fixtures/scenarios/complete"
	tests.RequireFilePath(t, fixturePath, "test fixture directory")

	cmd := &cobra.Command{}
	cmd.Flags().Bool("upload", false, "Upload instances to Atmos Pro")
	cmd.Flags().String("format", "matrix", "Output format")

	info := &schema.ConfigAndStacksInfo{
		BasePath: fixturePath,
	}

	err = ExecuteListInstancesCmd(&InstancesCommandOptions{
		Info: info,
		Cmd:  cmd,
		Args: []string{},
	})

	assert.NoError(t, err)
}

// TestExecuteListInstancesCmd_MatrixFormatWithOutputFile tests matrix format writing to file.
func TestExecuteListInstancesCmd_MatrixFormatWithOutputFile(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	fixturePath := "../../tests/fixtures/scenarios/complete"
	tests.RequireFilePath(t, fixturePath, "test fixture directory")

	outputFile := filepath.Join(t.TempDir(), "github_output")

	cmd := &cobra.Command{}
	cmd.Flags().Bool("upload", false, "Upload instances to Atmos Pro")
	cmd.Flags().String("format", "matrix", "Output format")

	info := &schema.ConfigAndStacksInfo{
		BasePath: fixturePath,
	}

	err = ExecuteListInstancesCmd(&InstancesCommandOptions{
		Info:       info,
		Cmd:        cmd,
		Args:       []string{},
		OutputFile: outputFile,
	})
	require.NoError(t, err)

	// Verify file was written with expected format.
	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "matrix=")
	assert.Contains(t, string(content), "count=")
	assert.Contains(t, string(content), `"include"`)
}

// TestExecuteListInstancesCmd_OutputFileRejectsNonMatrix tests that --output-file is rejected for non-matrix formats.
func TestExecuteListInstancesCmd_OutputFileRejectsNonMatrix(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("upload", false, "Upload instances to Atmos Pro")
	cmd.Flags().String("format", "json", "Output format")

	info := &schema.ConfigAndStacksInfo{
		BasePath: "../../tests/fixtures/scenarios/complete",
	}

	err := ExecuteListInstancesCmd(&InstancesCommandOptions{
		Info:       info,
		Cmd:        cmd,
		Args:       []string{},
		OutputFile: "/tmp/some-file",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.Contains(t, err.Error(), "--output-file is only supported with --format=matrix")
}

// TestBuildInstanceFilters tests the filter builder placeholder.
func TestBuildInstanceFilters(t *testing.T) {
	// Currently buildInstanceFilters is a placeholder that returns nil.
	result, err := buildInstanceFilters("any-spec")
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestExecuteListInstancesCmd_TreeFormat exercises the tree-format branch
// of ExecuteListInstancesCmd — enables provenance tracking, re-processes
// stacks via ResolveImportTreeFromProvenance, and renders via
// format.RenderInstancesTree.
func TestExecuteListInstancesCmd_TreeFormat(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	fixturePath := "../../tests/fixtures/scenarios/complete"
	tests.RequireFilePath(t, fixturePath, "test fixture directory")

	cmd := &cobra.Command{}
	cmd.Flags().Bool("upload", false, "Upload instances to Atmos Pro")
	cmd.Flags().String("format", "tree", "Output format")

	info := &schema.ConfigAndStacksInfo{
		BasePath: fixturePath,
	}

	err = ExecuteListInstancesCmd(&InstancesCommandOptions{
		Info:             info,
		Cmd:              cmd,
		Args:             []string{},
		ProcessTemplates: true,
		ProcessFunctions: false,
	})
	require.NoError(t, err, "complete fixture should render instances tree cleanly")
}

// TestExecuteListInstancesCmd_TreeFormatRejectsUpload verifies the
// tree-format branch rejects `--upload` (mirrors the matrix-format
// rejection test).
func TestExecuteListInstancesCmd_TreeFormatRejectsUpload(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("upload", true, "Upload instances to Atmos Pro")
	cmd.Flags().String("format", "tree", "Output format")

	info := &schema.ConfigAndStacksInfo{
		BasePath: "../../tests/fixtures/scenarios/complete",
	}

	err := ExecuteListInstancesCmd(&InstancesCommandOptions{
		Info: info,
		Cmd:  cmd,
		Args: []string{},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.Contains(t, err.Error(), "--upload is not supported with --format=tree")
}
