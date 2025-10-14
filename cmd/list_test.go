package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/list/errors"
)

// setupTestCommand creates a test command with the necessary flags.
func setupTestCommand(use string) *cobra.Command {
	cmd := &cobra.Command{
		Use: use,
	}
	cmd.PersistentFlags().String("format", "", "Output format")
	cmd.PersistentFlags().String("delimiter", "", "Delimiter for CSV/TSV output")
	cmd.PersistentFlags().String("stack", "", "Stack pattern")
	cmd.PersistentFlags().String("query", "", "JQ query")
	cmd.PersistentFlags().Int("max-columns", 0, "Maximum columns")
	cmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing")
	cmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing")
	return cmd
}

// TestComponentDefinitionNotFoundError tests that the ComponentDefinitionNotFoundError.
func TestComponentDefinitionNotFoundError(t *testing.T) {
	testCases := []struct {
		name           string
		componentName  string
		expectedOutput string
		runFunc        func(cmd *cobra.Command, args []string) (string, error)
	}{
		{
			name:           "list values - component not found",
			componentName:  "nonexistent-component",
			expectedOutput: "component 'nonexistent-component' does not exist",
			runFunc: func(cmd *cobra.Command, args []string) (string, error) {
				return "", &errors.ComponentDefinitionNotFoundError{Component: args[0]}
			},
		},
		{
			name:           "list settings - component not found",
			componentName:  "nonexistent-component",
			expectedOutput: "component 'nonexistent-component' does not exist",
			runFunc: func(cmd *cobra.Command, args []string) (string, error) {
				return "", &errors.ComponentDefinitionNotFoundError{Component: args[0]}
			},
		},
		{
			name:           "list metadata - component not found",
			componentName:  "nonexistent-component",
			expectedOutput: "component 'nonexistent-component' does not exist",
			runFunc: func(cmd *cobra.Command, args []string) (string, error) {
				return "", &errors.ComponentDefinitionNotFoundError{Component: args[0]}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test command
			cmd := setupTestCommand(tc.name)
			args := []string{tc.componentName}

			// Mock the listValues/listSettings/listMetadata function
			mockRunFunc := tc.runFunc

			// Run the command with the mocked function
			output, err := mockRunFunc(cmd, args)
			assert.Equal(t, "", output)
			assert.Error(t, err)

			// Check that the error is of the expected type
			var componentNotFoundErr *errors.ComponentDefinitionNotFoundError
			assert.ErrorAs(t, err, &componentNotFoundErr)
			assert.Equal(t, tc.componentName, componentNotFoundErr.Component)
			assert.Contains(t, componentNotFoundErr.Error(), tc.expectedOutput)

			// Verify that the error would be properly returned by the RunE function
			// This simulates what would happen in the actual command execution
			// where errors are returned to main() instead of being logged
			assert.NotNil(t, err, "Error should be returned to be handled by main()")
		})
	}
}

// TestNoValuesFoundError tests that the NoValuesFoundError is properly handled.
func TestNoValuesFoundError(t *testing.T) {
	testCases := []struct {
		name            string
		componentName   string
		query           string
		expectedOutput  string
		runFunc         func(cmd *cobra.Command, args []string) (string, error)
		shouldReturnNil bool
	}{
		{
			name:           "list values - no values found",
			componentName:  "test-component",
			expectedOutput: "No values found for component 'test-component'",
			runFunc: func(cmd *cobra.Command, args []string) (string, error) {
				return "", &errors.NoValuesFoundError{Component: args[0]}
			},
			shouldReturnNil: false,
		},
		{
			name:           "list vars - no vars found",
			componentName:  "test-component",
			query:          ".vars",
			expectedOutput: "No vars found for component 'test-component'",
			runFunc: func(cmd *cobra.Command, args []string) (string, error) {
				cmd.Flags().Set("query", ".vars")
				return "", &errors.NoValuesFoundError{Component: args[0]}
			},
			shouldReturnNil: true,
		},
		{
			name:           "list settings - no settings found with component",
			componentName:  "test-component",
			expectedOutput: "No settings found for component 'test-component'",
			runFunc: func(cmd *cobra.Command, args []string) (string, error) {
				return "", &errors.NoValuesFoundError{Component: args[0]}
			},
			shouldReturnNil: false,
		},
		{
			name:           "list settings - no settings found without component",
			componentName:  "",
			expectedOutput: "No settings found",
			runFunc: func(cmd *cobra.Command, args []string) (string, error) {
				return "", &errors.NoValuesFoundError{}
			},
			shouldReturnNil: false,
		},
		{
			name:           "list metadata - no metadata found with component",
			componentName:  "test-component",
			expectedOutput: "No metadata found for component 'test-component'",
			runFunc: func(cmd *cobra.Command, args []string) (string, error) {
				return "", &errors.NoValuesFoundError{Component: args[0]}
			},
			shouldReturnNil: false,
		},
		{
			name:           "list metadata - no metadata found without component",
			componentName:  "",
			expectedOutput: "No metadata found",
			runFunc: func(cmd *cobra.Command, args []string) (string, error) {
				return "", &errors.NoValuesFoundError{}
			},
			shouldReturnNil: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := setupTestCommand(tc.name)
			args := []string{}
			if tc.componentName != "" {
				args = append(args, tc.componentName)
			}

			if tc.query != "" {
				cmd.Flags().Set("query", tc.query)
			}

			mockRunFunc := tc.runFunc

			output, err := mockRunFunc(cmd, args)
			assert.Equal(t, "", output)
			assert.Error(t, err)

			var noValuesErr *errors.NoValuesFoundError
			assert.ErrorAs(t, err, &noValuesErr)
		})
	}
}

func TestListCmds_Error(t *testing.T) {
	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Unset ENV variables after testing
	defer func() {
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		os.Unsetenv("ATMOS_BASE_PATH")
	}()

	err := listComponentsCmd.RunE(listComponentsCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "list components command should return an error when called with invalid flags")

	err = listMetadataCmd.RunE(listMetadataCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "list metadata command should return an error when called with invalid flags")

	err = listSettingsCmd.RunE(listSettingsCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "list settings command should return an error when called with invalid flags")

	err = listValuesCmd.RunE(listValuesCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "list values command should return an error when called with invalid flags")

	err = listVendorCmd.RunE(listVendorCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "list vendor command should return an error when called with invalid flags")

	err = listWorkflowsCmd.RunE(listWorkflowsCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "list workflows command should return an error when called with invalid flags")
}
