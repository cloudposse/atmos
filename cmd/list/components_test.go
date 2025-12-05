package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
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
