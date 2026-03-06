//nolint:dupl // Test structure similarity is intentional for comprehensive coverage
package list

import (
	"bytes"
	goio "io"
	"os"
	"path/filepath"
	"strings"
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

func TestParseMetadataColumnsFlag(t *testing.T) {
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
			expected:    defaultMetadataColumns,
			expectErr:   false,
		},
		{
			name:        "nil flag returns defaults",
			columnsFlag: nil,
			expected:    defaultMetadataColumns,
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
			name:        "trims whitespace from name and value",
			columnsFlag: []string{"  Stack  =  {{ .stack }}  "},
			expected: []column.Config{
				{Name: "Stack", Value: "{{ .stack }}"},
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
			name:        "whitespace-only name",
			columnsFlag: []string{"   ={{ .stack }}"},
			expectErr:   true,
			errContains: "has empty name",
		},
		{
			name:        "empty template",
			columnsFlag: []string{"Stack="},
			expectErr:   true,
			errContains: "has empty template",
		},
		{
			name:        "whitespace-only template",
			columnsFlag: []string{"Stack=   "},
			expectErr:   true,
			errContains: "has empty template",
		},
		{
			name:        "error includes column number",
			columnsFlag: []string{"Valid={{ .stack }}", "Invalid"},
			expectErr:   true,
			errContains: "column spec 2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseMetadataColumnsFlag(tc.columnsFlag)

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

func TestGetMetadataColumns(t *testing.T) {
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
			expected:    defaultMetadataColumns,
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
			expected:    defaultMetadataColumns,
			expectErr:   false,
		},
		{
			name:        "invalid flag returns error",
			atmosConfig: &schema.AtmosConfiguration{},
			columnsFlag: []string{"InvalidSpec"},
			expectErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := getMetadataColumns(tc.atmosConfig, tc.columnsFlag)

			if tc.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestBuildMetadataSorters(t *testing.T) {
	tests := []struct {
		name        string
		sortSpec    string
		expected    []*listSort.Sorter
		expectErr   bool
		errContains string
	}{
		{
			name:     "empty spec returns default sorters",
			sortSpec: "",
			expected: []*listSort.Sorter{
				listSort.NewSorter("Stack", listSort.Ascending),
				listSort.NewSorter("Component", listSort.Ascending),
			},
			expectErr: false,
		},
		{
			name:     "single column ascending",
			sortSpec: "Stack:asc",
			expected: []*listSort.Sorter{
				listSort.NewSorter("Stack", listSort.Ascending),
			},
			expectErr: false,
		},
		{
			name:     "single column descending",
			sortSpec: "Stack:desc",
			expected: []*listSort.Sorter{
				listSort.NewSorter("Stack", listSort.Descending),
			},
			expectErr: false,
		},
		{
			name:     "multiple columns",
			sortSpec: "Stack:asc,Component:desc",
			expected: []*listSort.Sorter{
				listSort.NewSorter("Stack", listSort.Ascending),
				listSort.NewSorter("Component", listSort.Descending),
			},
			expectErr: false,
		},
		{
			name:        "invalid format missing colon",
			sortSpec:    "Stack",
			expectErr:   true,
			errContains: "expected format 'column:order'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := buildMetadataSorters(tc.sortSpec)

			if tc.expectErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.Len(t, result, len(tc.expected))
			for i, s := range result {
				assert.Equal(t, tc.expected[i].Column, s.Column)
				assert.Equal(t, tc.expected[i].Order, s.Order)
			}
		})
	}
}

func TestBuildMetadataFilters(t *testing.T) {
	// Currently buildMetadataFilters is a placeholder that returns nil.
	// Test that it behaves as expected.
	tests := []struct {
		name       string
		filterSpec string
	}{
		{
			name:       "empty filter spec",
			filterSpec: "",
		},
		{
			name:       "non-empty filter spec (currently ignored)",
			filterSpec: "stack=dev*",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := buildMetadataFilters(tc.filterSpec)
			require.NoError(t, err)
			assert.Nil(t, result)
		})
	}
}

func TestDefaultMetadataColumns(t *testing.T) {
	// Verify default columns are properly configured.
	assert.Len(t, defaultMetadataColumns, 8)

	expectedNames := []string{
		"Stack",
		"Component",
		"Type",
		"Enabled",
		"Locked",
		"Component (base)",
		"Inherits",
		"Description",
	}

	for i, col := range defaultMetadataColumns {
		assert.Equal(t, expectedNames[i], col.Name, "column %d name mismatch", i)
		assert.NotEmpty(t, col.Value, "column %d should have a template", i)
		assert.Contains(t, col.Value, "{{", "column %d template should be a Go template", i)
	}
}

func TestMetadataOptionsStruct(t *testing.T) {
	// Test that MetadataOptions struct can be properly constructed.
	opts := MetadataOptions{
		Format:    "json",
		Columns:   []string{"Stack={{ .stack }}"},
		Sort:      "-Stack",
		Filter:    "stack=dev*",
		Stack:     "dev",
		Delimiter: ",",
	}

	assert.Equal(t, "json", opts.Format)
	assert.Equal(t, []string{"Stack={{ .stack }}"}, opts.Columns)
	assert.Equal(t, "-Stack", opts.Sort)
	assert.Equal(t, "stack=dev*", opts.Filter)
	assert.Equal(t, "dev", opts.Stack)
	assert.Equal(t, ",", opts.Delimiter)
}

// TestExecuteListMetadataCmd tests the main metadata command entry point with real fixtures.
func TestExecuteListMetadataCmd(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	fixturePath, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "complete"))
	require.NoError(t, err)
	tests.RequireFilePath(t, fixturePath, "test fixture directory")

	cmd := &cobra.Command{}
	cmd.Flags().String("format", "table", "Output format")

	info := &schema.ConfigAndStacksInfo{
		BasePath: fixturePath,
	}

	err = ExecuteListMetadataCmd(info, cmd, []string{}, &MetadataOptions{})
	assert.NoError(t, err)
}

// TestExecuteListMetadataCmd_WithStackPattern tests that --stack filters output to the target stack.
func TestExecuteListMetadataCmd_WithStackPattern(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	fixturePath, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "complete"))
	require.NoError(t, err)
	tests.RequireFilePath(t, fixturePath, "test fixture directory")

	cmd := &cobra.Command{}
	cmd.Flags().String("format", "table", "Output format")

	info := &schema.ConfigAndStacksInfo{
		BasePath: fixturePath,
	}

	// Capture stdout to assert filtering behavior.
	oldStdout := os.Stdout

	r, w, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	defer func() { _ = r.Close() }()
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	err = ExecuteListMetadataCmd(info, cmd, []string{}, &MetadataOptions{
		Stack: "tenant1-ue2-dev",
	})

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, copyErr := goio.Copy(&buf, r)
	require.NoError(t, copyErr)
	os.Stdout = oldStdout

	require.NoError(t, err)
	output := buf.String()

	// Every data row must belong to the requested stack.
	assert.NotEmpty(t, output, "expected non-empty output for tenant1-ue2-dev")
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		assert.Contains(t, line, "tenant1-ue2-dev", "unexpected stack in output line: %q", line)
	}
}

// TestExecuteListMetadataCmd_InvalidConfig tests error handling for invalid config.
func TestExecuteListMetadataCmd_InvalidConfig(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("format", "table", "Output format")

	info := &schema.ConfigAndStacksInfo{
		BasePath: filepath.Join(t.TempDir(), "nonexistent", "path"),
	}

	err := ExecuteListMetadataCmd(info, cmd, []string{}, &MetadataOptions{})
	assert.Error(t, err)
}
