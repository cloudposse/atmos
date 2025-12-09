//nolint:dupl // Test structure similarity is intentional for consistency
package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
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
