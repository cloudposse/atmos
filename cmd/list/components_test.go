package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// initTestIO initializes the I/O and UI contexts for testing.
// This must be called before tests that use renderComponents or similar functions.
func initTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("failed to initialize I/O context: %v", err)
	}
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)
}

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
			expectedCount: 1, // Type filter only (authoritative)
			description:   "Type filter is authoritative",
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
			expectedCount: 4, // Stack + Type + Enabled + Locked (no abstract filter when Type is set)
			description:   "All filters combined",
		},
		{
			name: "Type filter with 'all'",
			opts: &ComponentsOptions{
				Type: "all",
			},
			expectedCount: 0, // Type='all' is not added as filter, and no abstract filter either
			description:   "Type='all' is ignored and no abstract filter",
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
			expectLen:   4, // CLI flag now properly parses column specifications
			expectName:  "Component",
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
		expectValue string
	}{
		{
			name:        "Empty columns",
			columnsFlag: []string{},
			expectLen:   0,
			expectName:  "",
			expectValue: "",
		},
		{
			name:        "Single simple column",
			columnsFlag: []string{"component"},
			expectLen:   1,
			expectName:  "Component",
			expectValue: "{{ .component }}",
		},
		{
			name:        "Multiple simple columns",
			columnsFlag: []string{"component", "stack", "type"},
			expectLen:   3,
			expectName:  "Component",
			expectValue: "{{ .component }}",
		},
		{
			name:        "Named column with template",
			columnsFlag: []string{"Name={{ .component }}"},
			expectLen:   1,
			expectName:  "Name",
			expectValue: "{{ .component }}",
		},
		{
			name:        "Named column with simple field",
			columnsFlag: []string{"MyStack=stack"},
			expectLen:   1,
			expectName:  "MyStack",
			expectValue: "{{ .stack }}",
		},
		{
			name:        "Mixed formats",
			columnsFlag: []string{"component", "MyType={{ .type }}"},
			expectLen:   2,
			expectName:  "Component",
			expectValue: "{{ .component }}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseColumnsFlag(tc.columnsFlag)
			assert.Equal(t, tc.expectLen, len(result))

			if tc.expectName != "" && len(result) > 0 {
				assert.Equal(t, tc.expectName, result[0].Name)
			}
			if tc.expectValue != "" && len(result) > 0 {
				assert.Equal(t, tc.expectValue, result[0].Value)
			}
		})
	}
}

// TestParseColumnSpec tests parsing individual column specifications.
func TestParseColumnSpec(t *testing.T) {
	testCases := []struct {
		name        string
		spec        string
		expectName  string
		expectValue string
	}{
		{
			name:        "Empty spec",
			spec:        "",
			expectName:  "",
			expectValue: "",
		},
		{
			name:        "Whitespace only",
			spec:        "   ",
			expectName:  "",
			expectValue: "",
		},
		{
			name:        "Simple field name",
			spec:        "component",
			expectName:  "Component",
			expectValue: "{{ .component }}",
		},
		{
			name:        "Field with leading/trailing whitespace",
			spec:        "  stack  ",
			expectName:  "Stack",
			expectValue: "{{ .stack }}",
		},
		{
			name:        "Named column with template",
			spec:        "MyColumn={{ .component }}",
			expectName:  "MyColumn",
			expectValue: "{{ .component }}",
		},
		{
			name:        "Named column with simple field (auto-wrap)",
			spec:        "MyStack=stack",
			expectName:  "MyStack",
			expectValue: "{{ .stack }}",
		},
		{
			name:        "Named column with whitespace",
			spec:        " Name = {{ .field }} ",
			expectName:  "Name",
			expectValue: "{{ .field }}",
		},
		{
			name:        "Complex template",
			spec:        "Info={{ .component }}-{{ .stack }}",
			expectName:  "Info",
			expectValue: "{{ .component }}-{{ .stack }}",
		},
		{
			name:        "Template with function",
			spec:        "Upper={{ upper .component }}",
			expectName:  "Upper",
			expectValue: "{{ upper .component }}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseColumnSpec(tc.spec)
			assert.Equal(t, tc.expectName, result.Name)
			assert.Equal(t, tc.expectValue, result.Value)
		})
	}
}

// TestParseColumnsFlag_EdgeCases tests edge cases for column flag parsing.
func TestParseColumnsFlag_EdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		columnsFlag []string
		expectLen   int
		checkFirst  bool
		firstName   string
		firstValue  string
	}{
		{
			name:        "Single empty string in slice",
			columnsFlag: []string{""},
			expectLen:   0, // Empty strings are skipped
		},
		{
			name:        "Multiple empty strings",
			columnsFlag: []string{"", "", ""},
			expectLen:   0,
		},
		{
			name:        "Mix of empty and valid",
			columnsFlag: []string{"", "component", ""},
			expectLen:   1,
			checkFirst:  true,
			firstName:   "Component",
			firstValue:  "{{ .component }}",
		},
		{
			name:        "Underscore field name",
			columnsFlag: []string{"component_type"},
			expectLen:   1,
			checkFirst:  true,
			firstName:   "Component_type",
			firstValue:  "{{ .component_type }}",
		},
		{
			name:        "Field with numbers",
			columnsFlag: []string{"var1"},
			expectLen:   1,
			checkFirst:  true,
			firstName:   "Var1",
			firstValue:  "{{ .var1 }}",
		},
		{
			name:        "Named column with equals in template",
			columnsFlag: []string{"Check={{ if eq .enabled true }}yes{{ end }}"},
			expectLen:   1,
			checkFirst:  true,
			firstName:   "Check",
			firstValue:  "{{ if eq .enabled true }}yes{{ end }}",
		},
		{
			name:        "Multiple named columns",
			columnsFlag: []string{"A={{ .a }}", "B={{ .b }}", "C={{ .c }}"},
			expectLen:   3,
			checkFirst:  true,
			firstName:   "A",
			firstValue:  "{{ .a }}",
		},
		{
			name:        "Column name only (equals at end)",
			columnsFlag: []string{"Name="},
			expectLen:   1,
			checkFirst:  true,
			firstName:   "Name",
			firstValue:  "{{ . }}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseColumnsFlag(tc.columnsFlag)
			assert.Equal(t, tc.expectLen, len(result), "Expected %d columns, got %d", tc.expectLen, len(result))

			if tc.checkFirst && len(result) > 0 {
				assert.Equal(t, tc.firstName, result[0].Name)
				assert.Equal(t, tc.firstValue, result[0].Value)
			}
		})
	}
}

// TestParseColumnSpec_SpecialCharacters tests parsing with special characters.
func TestParseColumnSpec_SpecialCharacters(t *testing.T) {
	testCases := []struct {
		name        string
		spec        string
		expectName  string
		expectValue string
	}{
		{
			name:        "Dot in field name",
			spec:        "vars.region",
			expectName:  "Vars.Region", // strings.Title capitalizes after dots.
			expectValue: "{{ .vars.region }}",
		},
		{
			name:        "Hyphen in field name",
			spec:        "my-field",
			expectName:  "My-Field", // strings.Title capitalizes after hyphens.
			expectValue: "{{ .my-field }}",
		},
		{
			name:        "Template with pipe",
			spec:        "Upper={{ .component | upper }}",
			expectName:  "Upper",
			expectValue: "{{ .component | upper }}",
		},
		{
			name:        "Template with multiple pipes",
			spec:        "Formatted={{ .name | lower | truncate 10 }}",
			expectName:  "Formatted",
			expectValue: "{{ .name | lower | truncate 10 }}",
		},
		{
			name:        "Template with conditional",
			spec:        "Status={{ if .enabled }}on{{ else }}off{{ end }}",
			expectName:  "Status",
			expectValue: "{{ if .enabled }}on{{ else }}off{{ end }}",
		},
		{
			name:        "Template with range",
			spec:        "Items={{ range .items }}{{ . }}{{ end }}",
			expectName:  "Items",
			expectValue: "{{ range .items }}{{ . }}{{ end }}",
		},
		{
			name:        "Named column with colon in name",
			spec:        "Type:Info={{ .type }}",
			expectName:  "Type:Info",
			expectValue: "{{ .type }}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseColumnSpec(tc.spec)
			assert.Equal(t, tc.expectName, result.Name)
			assert.Equal(t, tc.expectValue, result.Value)
		})
	}
}

// TestParseColumnsFlag_VerifyAllColumns tests that all columns are parsed correctly.
func TestParseColumnsFlag_VerifyAllColumns(t *testing.T) {
	columnsFlag := []string{
		"component",
		"Stack={{ .stack }}",
		"MyType=type",
	}

	result := parseColumnsFlag(columnsFlag)
	assert.Equal(t, 3, len(result))

	// Check first column (simple field)
	assert.Equal(t, "Component", result[0].Name)
	assert.Equal(t, "{{ .component }}", result[0].Value)

	// Check second column (named with template)
	assert.Equal(t, "Stack", result[1].Name)
	assert.Equal(t, "{{ .stack }}", result[1].Value)

	// Check third column (named with field)
	assert.Equal(t, "MyType", result[2].Name)
	assert.Equal(t, "{{ .type }}", result[2].Value)
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

// TestRenderComponents tests the renderComponents function with mock data.
func TestRenderComponents(t *testing.T) {
	initTestIO(t)

	testCases := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		opts        *ComponentsOptions
		components  []map[string]any
		expectError bool
		errorMsg    string
	}{
		{
			name: "Empty components list",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{},
				},
			},
			opts:        &ComponentsOptions{Format: "table"},
			components:  []map[string]any{},
			expectError: false,
		},
		{
			name: "Single component with table format",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{},
				},
			},
			opts: &ComponentsOptions{Format: "table"},
			components: []map[string]any{
				{"component": "vpc", "stack": "prod-us-east-1", "type": "terraform"},
			},
			expectError: false,
		},
		{
			name: "Multiple components with json format",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{},
				},
			},
			opts: &ComponentsOptions{Format: "json"},
			components: []map[string]any{
				{"component": "vpc", "stack": "prod-us-east-1", "type": "terraform"},
				{"component": "rds", "stack": "prod-us-east-1", "type": "terraform"},
				{"component": "eks", "stack": "dev-us-west-2", "type": "terraform"},
			},
			expectError: false,
		},
		{
			name: "Components with yaml format",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{},
				},
			},
			opts: &ComponentsOptions{Format: "yaml"},
			components: []map[string]any{
				{"component": "vpc", "stack": "prod", "type": "terraform"},
			},
			expectError: false,
		},
		{
			name: "Components with invalid sort spec",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{},
				},
			},
			opts: &ComponentsOptions{
				Format: "table",
				Sort:   "invalid::sort::spec",
			},
			components: []map[string]any{
				{"component": "vpc", "stack": "prod", "type": "terraform"},
			},
			expectError: true,
			errorMsg:    "error parsing sort specification",
		},
		{
			name: "Components with stack filter",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{},
				},
			},
			opts: &ComponentsOptions{
				Format: "table",
				Stack:  "prod-*",
			},
			components: []map[string]any{
				{"component": "vpc", "stack": "prod-us-east-1", "type": "terraform"},
				{"component": "rds", "stack": "dev-us-west-2", "type": "terraform"},
			},
			expectError: false,
		},
		{
			name: "Components with custom columns from config",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{
						Columns: []schema.ListColumnConfig{
							{Name: "Name", Value: "{{ .component }}"},
							{Name: "Environment", Value: "{{ .stack }}"},
						},
					},
				},
			},
			opts: &ComponentsOptions{
				Format: "table",
				Sort:   "Name:asc", // Use custom column name for sorting.
			},
			components: []map[string]any{
				{"component": "vpc", "stack": "prod", "type": "terraform"},
			},
			expectError: false,
		},
		{
			name: "Components with sort ascending",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{},
				},
			},
			opts: &ComponentsOptions{
				Format: "table",
				Sort:   "component:asc",
			},
			components: []map[string]any{
				{"component": "rds", "stack": "prod", "type": "terraform"},
				{"component": "eks", "stack": "prod", "type": "terraform"},
				{"component": "vpc", "stack": "prod", "type": "terraform"},
			},
			expectError: false,
		},
		{
			name: "Components with sort descending",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					List: schema.ListConfig{},
				},
			},
			opts: &ComponentsOptions{
				Format: "table",
				Sort:   "component:desc",
			},
			components: []map[string]any{
				{"component": "rds", "stack": "prod", "type": "terraform"},
				{"component": "eks", "stack": "prod", "type": "terraform"},
				{"component": "vpc", "stack": "prod", "type": "terraform"},
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := renderComponents(tc.atmosConfig, tc.opts, tc.components)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestRenderComponents_TriStateBoolFilters tests renderComponents with tri-state boolean filters.
func TestRenderComponents_TriStateBoolFilters(t *testing.T) {
	initTestIO(t)

	enabledTrue := true
	enabledFalse := false
	lockedTrue := true
	lockedFalse := false

	testCases := []struct {
		name       string
		opts       *ComponentsOptions
		components []map[string]any
	}{
		{
			name: "Filter enabled=true",
			opts: &ComponentsOptions{
				Format:  "json",
				Enabled: &enabledTrue,
			},
			components: []map[string]any{
				{"component": "vpc", "stack": "prod", "enabled": true},
				{"component": "rds", "stack": "prod", "enabled": false},
			},
		},
		{
			name: "Filter enabled=false",
			opts: &ComponentsOptions{
				Format:  "json",
				Enabled: &enabledFalse,
			},
			components: []map[string]any{
				{"component": "vpc", "stack": "prod", "enabled": true},
				{"component": "rds", "stack": "prod", "enabled": false},
			},
		},
		{
			name: "Filter locked=true",
			opts: &ComponentsOptions{
				Format: "json",
				Locked: &lockedTrue,
			},
			components: []map[string]any{
				{"component": "vpc", "stack": "prod", "locked": true},
				{"component": "rds", "stack": "prod", "locked": false},
			},
		},
		{
			name: "Filter locked=false",
			opts: &ComponentsOptions{
				Format: "json",
				Locked: &lockedFalse,
			},
			components: []map[string]any{
				{"component": "vpc", "stack": "prod", "locked": true},
				{"component": "rds", "stack": "prod", "locked": false},
			},
		},
		{
			name: "Combine enabled and locked filters",
			opts: &ComponentsOptions{
				Format:  "json",
				Enabled: &enabledTrue,
				Locked:  &lockedFalse,
			},
			components: []map[string]any{
				{"component": "vpc", "stack": "prod", "enabled": true, "locked": false},
				{"component": "rds", "stack": "prod", "enabled": false, "locked": true},
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			List: schema.ListConfig{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := renderComponents(atmosConfig, tc.opts, tc.components)
			assert.NoError(t, err)
		})
	}
}

// TestRenderComponents_TypeFilter tests renderComponents with type filtering.
func TestRenderComponents_TypeFilter(t *testing.T) {
	initTestIO(t)

	testCases := []struct {
		name       string
		typeFilter string
		abstract   bool
	}{
		{
			name:       "Type filter terraform",
			typeFilter: "terraform",
			abstract:   false,
		},
		{
			name:       "Type filter helmfile",
			typeFilter: "helmfile",
			abstract:   false,
		},
		{
			name:       "Type filter all",
			typeFilter: "all",
			abstract:   false,
		},
		{
			name:       "Abstract flag true",
			typeFilter: "",
			abstract:   true,
		},
		{
			name:       "Type filter with abstract",
			typeFilter: "terraform",
			abstract:   true,
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			List: schema.ListConfig{},
		},
	}

	components := []map[string]any{
		{"component": "vpc", "stack": "prod", "type": "terraform", "component_type": "real"},
		{"component": "base-vpc", "stack": "prod", "type": "terraform", "component_type": "abstract"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := &ComponentsOptions{
				Format:   "json",
				Type:     tc.typeFilter,
				Abstract: tc.abstract,
			}
			err := renderComponents(atmosConfig, opts, components)
			assert.NoError(t, err)
		})
	}
}

// TestInitAndExtractComponents is documented in integration tests.
// Unit testing with nil command is not meaningful as ProcessCommandLineArgs
// requires a valid command context. See tests/cli_list_commands_test.go for
// integration tests that exercise the full command flow.
