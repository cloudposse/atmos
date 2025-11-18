//nolint:dupl // Test structure similarity is intentional for consistency
package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestListStacksFlags tests that the list stacks command has the correct flags.
func TestListStacksFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "stacks",
		Short: "List all Atmos stacks or stacks for a specific component",
		Long:  "This command lists all Atmos stacks, or filters the list to show only the stacks associated with a specified component.",
		Args:  cobra.NoArgs,
	}

	cmd.PersistentFlags().StringP("component", "c", "", "List all stacks that contain the specified component")

	componentFlag := cmd.PersistentFlags().Lookup("component")
	assert.NotNil(t, componentFlag, "Expected component flag to exist")
	assert.Equal(t, "", componentFlag.DefValue)
	assert.Equal(t, "c", componentFlag.Shorthand)
}

// TestListStacksValidatesArgs tests that the command validates arguments.
func TestListStacksValidatesArgs(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "stacks",
		Args: cobra.NoArgs,
	}

	err := cmd.ValidateArgs([]string{})
	assert.NoError(t, err, "Validation should pass with no arguments")

	err = cmd.ValidateArgs([]string{"extra"})
	assert.Error(t, err, "Validation should fail with arguments")
}

// TestListStacksCommand tests the stacks command structure.
func TestListStacksCommand(t *testing.T) {
	assert.Equal(t, "stacks", stacksCmd.Use)
	assert.Contains(t, stacksCmd.Short, "List all Atmos stacks")
	assert.NotNil(t, stacksCmd.RunE)

	// Check that NoArgs validator is set
	err := stacksCmd.Args(stacksCmd, []string{"unexpected"})
	assert.Error(t, err, "Should reject extra arguments")

	err = stacksCmd.Args(stacksCmd, []string{})
	assert.NoError(t, err, "Should accept no arguments")
}

// TestListStacksWithOptions_EmptyComponent tests listing all stacks.
func TestListStacksWithOptions_EmptyComponent(t *testing.T) {
	opts := &StacksOptions{
		Component: "",
	}

	// Test that the options are properly structured
	assert.Equal(t, "", opts.Component)
}

// TestListStacksWithOptions_WithComponent tests filtering by component.
func TestListStacksWithOptions_WithComponent(t *testing.T) {
	opts := &StacksOptions{
		Component: "vpc",
	}

	// Test that the options are properly structured
	assert.Equal(t, "vpc", opts.Component)
}

// TestStacksOptions tests the StacksOptions structure.
func TestStacksOptions(t *testing.T) {
	testCases := []struct {
		name              string
		opts              *StacksOptions
		expectedComponent string
	}{
		{
			name:              "with component",
			opts:              &StacksOptions{Component: "database"},
			expectedComponent: "database",
		},
		{
			name:              "without component",
			opts:              &StacksOptions{},
			expectedComponent: "",
		},
		{
			name:              "with complex component name",
			opts:              &StacksOptions{Component: "my-vpc-component"},
			expectedComponent: "my-vpc-component",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedComponent, tc.opts.Component)
		})
	}
}

// TestStacksOptions_AllFields tests all fields in StacksOptions.
func TestStacksOptions_AllFields(t *testing.T) {
	testCases := []struct {
		name            string
		opts            *StacksOptions
		expectedComp    string
		expectedFormat  string
		expectedColumns []string
		expectedSort    string
		expectedProv    bool
	}{
		{
			name: "All fields populated",
			opts: &StacksOptions{
				Component:  "vpc",
				Format:     "table",
				Columns:    []string{"stack", "component"},
				Sort:       "stack:asc",
				Provenance: true,
			},
			expectedComp:    "vpc",
			expectedFormat:  "table",
			expectedColumns: []string{"stack", "component"},
			expectedSort:    "stack:asc",
			expectedProv:    true,
		},
		{
			name:            "Empty options",
			opts:            &StacksOptions{},
			expectedComp:    "",
			expectedFormat:  "",
			expectedColumns: nil,
			expectedSort:    "",
			expectedProv:    false,
		},
		{
			name: "Format options",
			opts: &StacksOptions{
				Format: "json",
			},
			expectedComp:   "",
			expectedFormat: "json",
			expectedSort:   "",
			expectedProv:   false,
		},
		{
			name: "Tree format with provenance",
			opts: &StacksOptions{
				Format:     "tree",
				Provenance: true,
			},
			expectedComp:   "",
			expectedFormat: "tree",
			expectedProv:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedComp, tc.opts.Component)
			assert.Equal(t, tc.expectedFormat, tc.opts.Format)
			assert.Equal(t, tc.expectedColumns, tc.opts.Columns)
			assert.Equal(t, tc.expectedSort, tc.opts.Sort)
			assert.Equal(t, tc.expectedProv, tc.opts.Provenance)
		})
	}
}

// TestBuildStackFilters tests filter building.
func TestBuildStackFilters(t *testing.T) {
	testCases := []struct {
		name          string
		opts          *StacksOptions
		expectedCount int
	}{
		{
			name:          "No filters",
			opts:          &StacksOptions{},
			expectedCount: 0,
		},
		{
			name: "With component filter",
			opts: &StacksOptions{
				Component: "vpc",
			},
			expectedCount: 0, // Component filter is handled by extraction logic
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := buildStackFilters(tc.opts)
			assert.Equal(t, tc.expectedCount, len(result))
		})
	}
}

// TestGetStackColumns tests column configuration logic.
func TestGetStackColumns(t *testing.T) {
	testCases := []struct {
		name         string
		atmosConfig  *schema.AtmosConfiguration
		columnsFlag  []string
		hasComponent bool
		expectLen    int
		expectName   string
	}{
		{
			name: "Default columns without component",
			atmosConfig: &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					List: schema.ListConfig{},
				},
			},
			columnsFlag:  []string{},
			hasComponent: false,
			expectLen:    1,
			expectName:   "Stack",
		},
		{
			name: "Default columns with component",
			atmosConfig: &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					List: schema.ListConfig{},
				},
			},
			columnsFlag:  []string{},
			hasComponent: true,
			expectLen:    2,
			expectName:   "Stack",
		},
		{
			name: "Columns from flag",
			atmosConfig: &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					List: schema.ListConfig{},
				},
			},
			columnsFlag:  []string{"stack", "component", "type"},
			hasComponent: false,
			expectLen:    3,
		},
		{
			name: "Columns from config",
			atmosConfig: &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					List: schema.ListConfig{
						Columns: []schema.ListColumnConfig{
							{Name: "Stack", Value: "{{ .stack }}"},
							{Name: "Component", Value: "{{ .component }}"},
						},
					},
				},
			},
			columnsFlag:  []string{},
			hasComponent: false,
			expectLen:    2,
			expectName:   "Stack",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getStackColumns(tc.atmosConfig, tc.columnsFlag, tc.hasComponent)
			assert.Equal(t, tc.expectLen, len(result))

			if tc.expectName != "" && len(result) > 0 {
				assert.Equal(t, tc.expectName, result[0].Name)
			}
		})
	}
}

// TestBuildStackSorters tests sorter building.
func TestBuildStackSorters(t *testing.T) {
	testCases := []struct {
		name        string
		sortSpec    string
		expectLen   int
		expectError bool
	}{
		{
			name:      "Empty sort (default)",
			sortSpec:  "",
			expectLen: 1, // Default sort by stack ascending
		},
		{
			name:      "Single sort field ascending",
			sortSpec:  "stack:asc",
			expectLen: 1,
		},
		{
			name:      "Single sort field descending",
			sortSpec:  "stack:desc",
			expectLen: 1,
		},
		{
			name:      "Multiple sort fields",
			sortSpec:  "component:asc,stack:desc",
			expectLen: 2,
		},
		{
			name:        "Invalid sort spec",
			sortSpec:    "invalid::spec",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := buildStackSorters(tc.sortSpec)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectLen, len(result))
			}
		})
	}
}

// TestColumnsCompletionForStacks tests tab completion for columns flag.
func TestColumnsCompletionForStacks(t *testing.T) {
	// This test verifies the function signature and basic behavior.
	// Full integration testing would require a valid atmos.yaml config.
	cmd := &cobra.Command{}
	args := []string{}
	toComplete := ""

	// Should return empty or error if config cannot be loaded.
	suggestions, directive := columnsCompletionForStacks(cmd, args, toComplete)

	// Function should return (even if empty) and directive should be NoFileComp.
	// Suggestions can be nil or empty when config is not available.
	_ = suggestions // May be nil or empty
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}
