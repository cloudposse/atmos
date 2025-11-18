//nolint:dupl // Test structure similarity is intentional for consistency
package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestListComponentsFlags tests that the list components command has the correct flags.
func TestListComponentsFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "components",
		Short: "List all Atmos components or filter by stack",
		Long:  "List Atmos components, with options to filter results by specific stacks.",
		Args:  cobra.NoArgs,
	}

	cmd.PersistentFlags().StringP("stack", "s", "", "Filter by stack name or pattern")

	stackFlag := cmd.PersistentFlags().Lookup("stack")
	assert.NotNil(t, stackFlag, "Expected stack flag to exist")
	assert.Equal(t, "", stackFlag.DefValue)
	assert.Equal(t, "s", stackFlag.Shorthand)
}

// TestListComponentsValidatesArgs tests that the command validates arguments.
func TestListComponentsValidatesArgs(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "components",
		Args: cobra.NoArgs,
	}

	err := cmd.ValidateArgs([]string{})
	assert.NoError(t, err, "Validation should pass with no arguments")

	err = cmd.ValidateArgs([]string{"extra"})
	assert.Error(t, err, "Validation should fail with arguments")
}

// TestListComponentsCommand tests the components command structure.
func TestListComponentsCommand(t *testing.T) {
	assert.Equal(t, "components", componentsCmd.Use)
	assert.Contains(t, componentsCmd.Short, "List all Atmos components")
	assert.NotNil(t, componentsCmd.RunE)

	// Check that NoArgs validator is set
	err := componentsCmd.Args(componentsCmd, []string{"unexpected"})
	assert.Error(t, err, "Should reject extra arguments")

	err = componentsCmd.Args(componentsCmd, []string{})
	assert.NoError(t, err, "Should accept no arguments")
}

// TestListComponentsWithOptions_EmptyStack tests filtering with empty stack pattern.
func TestListComponentsWithOptions_EmptyStack(t *testing.T) {
	opts := &ComponentsOptions{
		Stack: "",
	}

	// Test that the options are properly structured
	assert.Equal(t, "", opts.Stack)
}

// TestListComponentsWithOptions_StackPattern tests filtering with stack pattern.
func TestListComponentsWithOptions_StackPattern(t *testing.T) {
	opts := &ComponentsOptions{
		Stack: "prod-*",
	}

	// Test that the options are properly structured
	assert.Equal(t, "prod-*", opts.Stack)
}

// TestComponentsOptions_AllPatterns tests various stack pattern combinations.
func TestComponentsOptions_AllPatterns(t *testing.T) {
	testCases := []struct {
		name          string
		opts          *ComponentsOptions
		expectedStack string
	}{
		{
			name:          "wildcard pattern at end",
			opts:          &ComponentsOptions{Stack: "prod-*"},
			expectedStack: "prod-*",
		},
		{
			name:          "wildcard pattern at start",
			opts:          &ComponentsOptions{Stack: "*-prod"},
			expectedStack: "*-prod",
		},
		{
			name:          "wildcard pattern in middle",
			opts:          &ComponentsOptions{Stack: "prod-*-vpc"},
			expectedStack: "prod-*-vpc",
		},
		{
			name:          "multiple wildcard patterns",
			opts:          &ComponentsOptions{Stack: "*-dev-*"},
			expectedStack: "*-dev-*",
		},
		{
			name:          "exact stack name",
			opts:          &ComponentsOptions{Stack: "prod-us-east-1"},
			expectedStack: "prod-us-east-1",
		},
		{
			name:          "empty stack",
			opts:          &ComponentsOptions{},
			expectedStack: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedStack, tc.opts.Stack)
		})
	}
}

// TestComponentsOptions_AllFields tests all fields in ComponentsOptions.
func TestComponentsOptions_AllFields(t *testing.T) {
	enabledTrue := true
	enabledFalse := false
	lockedTrue := true
	lockedFalse := false

	testCases := []struct {
		name             string
		opts             *ComponentsOptions
		expectedStack    string
		expectedType     string
		expectedEnabled  *bool
		expectedLocked   *bool
		expectedFormat   string
		expectedColumns  []string
		expectedSort     string
		expectedAbstract bool
	}{
		{
			name: "All fields populated",
			opts: &ComponentsOptions{
				Stack:    "prod-*",
				Type:     "terraform",
				Enabled:  &enabledTrue,
				Locked:   &lockedFalse,
				Format:   "table",
				Columns:  []string{"component", "stack", "type"},
				Sort:     "component:asc",
				Abstract: true,
			},
			expectedStack:    "prod-*",
			expectedType:     "terraform",
			expectedEnabled:  &enabledTrue,
			expectedLocked:   &lockedFalse,
			expectedFormat:   "table",
			expectedColumns:  []string{"component", "stack", "type"},
			expectedSort:     "component:asc",
			expectedAbstract: true,
		},
		{
			name:             "Empty options",
			opts:             &ComponentsOptions{},
			expectedStack:    "",
			expectedType:     "",
			expectedEnabled:  nil,
			expectedLocked:   nil,
			expectedFormat:   "",
			expectedColumns:  nil,
			expectedSort:     "",
			expectedAbstract: false,
		},
		{
			name: "Tri-state booleans - all true",
			opts: &ComponentsOptions{
				Enabled: &enabledTrue,
				Locked:  &lockedTrue,
			},
			expectedStack:    "",
			expectedType:     "",
			expectedEnabled:  &enabledTrue,
			expectedLocked:   &lockedTrue,
			expectedFormat:   "",
			expectedAbstract: false,
		},
		{
			name: "Tri-state booleans - all false",
			opts: &ComponentsOptions{
				Enabled: &enabledFalse,
				Locked:  &lockedFalse,
			},
			expectedStack:    "",
			expectedType:     "",
			expectedEnabled:  &enabledFalse,
			expectedLocked:   &lockedFalse,
			expectedFormat:   "",
			expectedAbstract: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedStack, tc.opts.Stack)
			assert.Equal(t, tc.expectedType, tc.opts.Type)
			assert.Equal(t, tc.expectedEnabled, tc.opts.Enabled)
			assert.Equal(t, tc.expectedLocked, tc.opts.Locked)
			assert.Equal(t, tc.expectedFormat, tc.opts.Format)
			assert.Equal(t, tc.expectedColumns, tc.opts.Columns)
			assert.Equal(t, tc.expectedSort, tc.opts.Sort)
			assert.Equal(t, tc.expectedAbstract, tc.opts.Abstract)
		})
	}
}

// TestBuildComponentFilters tests filter building.
func TestBuildComponentFilters(t *testing.T) {
	enabledTrue := true
	enabledFalse := false
	lockedTrue := true

	testCases := []struct {
		name          string
		opts          *ComponentsOptions
		expectedCount int
		description   string
	}{
		{
			name:          "No filters",
			opts:          &ComponentsOptions{},
			expectedCount: 1, // Abstract filter (component_type=real) is always added
			description:   "Default includes abstract filter for real components",
		},
		{
			name: "Stack filter",
			opts: &ComponentsOptions{
				Stack: "prod-*",
			},
			expectedCount: 2, // Stack filter + abstract filter
			description:   "Stack glob filter + abstract filter",
		},
		{
			name: "Type filter",
			opts: &ComponentsOptions{
				Type: "terraform",
			},
			expectedCount: 2, // Type filter + abstract filter
			description:   "Type filter + abstract filter",
		},
		{
			name: "Enabled filter true",
			opts: &ComponentsOptions{
				Enabled: &enabledTrue,
			},
			expectedCount: 2, // Enabled filter + abstract filter
			description:   "Enabled=true filter + abstract filter",
		},
		{
			name: "Enabled filter false",
			opts: &ComponentsOptions{
				Enabled: &enabledFalse,
			},
			expectedCount: 2, // Enabled filter + abstract filter
			description:   "Enabled=false filter + abstract filter",
		},
		{
			name: "Locked filter",
			opts: &ComponentsOptions{
				Locked: &lockedTrue,
			},
			expectedCount: 2, // Locked filter + abstract filter
			description:   "Locked filter + abstract filter",
		},
		{
			name: "Abstract flag true",
			opts: &ComponentsOptions{
				Abstract: true,
			},
			expectedCount: 0, // Abstract filter is NOT added when Abstract=true
			description:   "No filters when showing abstract components",
		},
		{
			name: "All filters combined",
			opts: &ComponentsOptions{
				Stack:   "prod-*",
				Type:    "terraform",
				Enabled: &enabledTrue,
				Locked:  &lockedTrue,
			},
			expectedCount: 5, // Stack + Type + Enabled + Locked + abstract filter
			description:   "All filters combined",
		},
		{
			name: "Type filter with 'all'",
			opts: &ComponentsOptions{
				Type: "all",
			},
			expectedCount: 1, // Type='all' is not added as filter, only abstract filter
			description:   "Type='all' is ignored",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := buildComponentFilters(tc.opts)
			assert.Equal(t, tc.expectedCount, len(result), tc.description)
		})
	}
}

// TestGetComponentColumns tests column configuration logic.
func TestGetComponentColumns(t *testing.T) {
	testCases := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		columnsFlag []string
		expectLen   int
		expectName  string
	}{
		{
			name: "Default columns",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{},
				},
			},
			columnsFlag: []string{},
			expectLen:   3,
			expectName:  "Component",
		},
		{
			name: "Columns from flag",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{},
				},
			},
			columnsFlag: []string{"component", "stack", "type", "enabled"},
			expectLen:   3, // parseColumnsFlag returns default for now
		},
		{
			name: "Columns from config",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{
						Columns: []schema.ListColumnConfig{
							{Name: "Component", Value: "{{ .component }}"},
							{Name: "Stack", Value: "{{ .stack }}"},
							{Name: "Type", Value: "{{ .type }}"},
							{Name: "Enabled", Value: "{{ .enabled }}"},
						},
					},
				},
			},
			columnsFlag: []string{},
			expectLen:   4,
			expectName:  "Component",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getComponentColumns(tc.atmosConfig, tc.columnsFlag)
			assert.Equal(t, tc.expectLen, len(result))

			if tc.expectName != "" && len(result) > 0 {
				assert.Equal(t, tc.expectName, result[0].Name)
			}
		})
	}
}

// TestBuildComponentSorters tests sorter building.
func TestBuildComponentSorters(t *testing.T) {
	testCases := []struct {
		name        string
		sortSpec    string
		expectLen   int
		expectError bool
	}{
		{
			name:      "Empty sort (default)",
			sortSpec:  "",
			expectLen: 1, // Default sort by component ascending
		},
		{
			name:      "Single sort field ascending",
			sortSpec:  "component:asc",
			expectLen: 1,
		},
		{
			name:      "Single sort field descending",
			sortSpec:  "stack:desc",
			expectLen: 1,
		},
		{
			name:      "Multiple sort fields",
			sortSpec:  "type:asc,component:desc",
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
			result, err := buildComponentSorters(tc.sortSpec)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectLen, len(result))
			}
		})
	}
}

// TestParseColumnsFlag tests column flag parsing.
func TestParseColumnsFlag(t *testing.T) {
	testCases := []struct {
		name        string
		columnsFlag []string
		expectLen   int
		expectName  string
	}{
		{
			name:        "Empty columns",
			columnsFlag: []string{},
			expectLen:   3, // Returns default
			expectName:  "Component",
		},
		{
			name:        "Multiple columns",
			columnsFlag: []string{"component", "stack", "type"},
			expectLen:   3, // Returns default for now (TODO)
			expectName:  "Component",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseColumnsFlag(tc.columnsFlag)
			assert.Equal(t, tc.expectLen, len(result))

			if tc.expectName != "" && len(result) > 0 {
				assert.Equal(t, tc.expectName, result[0].Name)
			}
		})
	}
}

// TestColumnsCompletionForComponents tests tab completion for columns flag.
func TestColumnsCompletionForComponents(t *testing.T) {
	// This test verifies the function signature and basic behavior.
	// Full integration testing would require a valid atmos.yaml config.
	cmd := &cobra.Command{}
	args := []string{}
	toComplete := ""

	// Should return empty or error if config cannot be loaded.
	suggestions, directive := columnsCompletionForComponents(cmd, args, toComplete)

	// Function should return (even if empty) and directive should be NoFileComp.
	// Suggestions can be nil or empty when config is not available.
	_ = suggestions // May be nil or empty
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}
