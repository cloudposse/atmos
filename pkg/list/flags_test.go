package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestAddCommonListFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
		Run: func(cmd *cobra.Command, args []string) {},
	}

	AddCommonListFlags(cmd)

	flags := cmd.PersistentFlags()

	query, err := flags.GetString("query")
	assert.NoError(t, err)
	assert.Equal(t, "", query, "Default query should be empty")

	// Check max-columns flag.
	maxColumns, err := flags.GetInt("max-columns")
	assert.NoError(t, err)
	assert.Equal(t, DefaultMaxColumns, maxColumns, "Default max-columns should match DefaultMaxColumns")

	// Check format flag.
	format, err := flags.GetString("format")
	assert.NoError(t, err)
	assert.Equal(t, "", format, "Default format should be empty")

	// Check delimiter flag.
	delimiter, err := flags.GetString("delimiter")
	assert.NoError(t, err)
	assert.Equal(t, "\t", delimiter, "Default delimiter should be tab")

	// Check stack flag.
	stack, err := flags.GetString("stack")
	assert.NoError(t, err)
	assert.Equal(t, "", stack, "Default stack should be empty")
}

// TestSuccessfulFlagParsingWithLocalFlags tests the normal operation of GetCommonListFlags
// using local flags and only valid inputs.
func TestSuccessfulFlagParsingWithLocalFlags(t *testing.T) {
	// Create a test command that will provide all the necessary flags.
	cmd := &cobra.Command{
		Use: "test",
		Run: func(cmd *cobra.Command, args []string) {},
	}

	// Add local flags (not persistent).
	cmd.Flags().String("query", "test-query", "Test query flag")
	cmd.Flags().Int("max-columns", 5, "Test max-columns flag")
	cmd.Flags().String("format", "json", "Test format flag")
	cmd.Flags().String("delimiter", ",", "Test delimiter flag")
	cmd.Flags().String("stack", "test-stack", "Test stack flag")

	// Get the flags - since we're providing valid values, this should succeed.
	result, err := GetCommonListFlags(cmd)

	// Verify results.
	assert.NoError(t, err, "GetCommonListFlags should succeed with valid flags")
	assert.NotNil(t, result, "Result should not be nil")

	// Check all values.
	assert.Equal(t, "test-query", result.Query)
	assert.Equal(t, 5, result.MaxColumns)
	assert.Equal(t, "json", result.Format)
	assert.Equal(t, ",", result.Delimiter)
	assert.Equal(t, "test-stack", result.Stack)
}

// TestGetCommonListFlagsErrors tests the error handling in GetCommonListFlags.
func TestGetCommonListFlagsErrors(t *testing.T) {
	// Test when flags are completely missing.
	t.Run("Missing flags", func(t *testing.T) {
		cmd := &cobra.Command{
			Use: "test",
			Run: func(cmd *cobra.Command, args []string) {},
		}

		result, err := GetCommonListFlags(cmd)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	// Test common error cases for the flags.
	flagTests := []struct {
		name       string
		setupFlags func(*cobra.Command)
	}{
		{
			name: "Missing query flag",
			setupFlags: func(cmd *cobra.Command) {
				// Add all flags except query.
				cmd.Flags().Int("max-columns", DefaultMaxColumns, "")
				cmd.Flags().String("format", "json", "") // Using a valid format.
				cmd.Flags().String("delimiter", "\t", "")
				cmd.Flags().String("stack", "", "")
			},
		},
		{
			name: "Missing max-columns flag",
			setupFlags: func(cmd *cobra.Command) {
				// Add all flags except max-columns.
				cmd.Flags().String("query", "", "")
				cmd.Flags().String("format", "json", "") // Using a valid format.
				cmd.Flags().String("delimiter", "\t", "")
				cmd.Flags().String("stack", "", "")
			},
		},
		{
			name: "Missing format flag",
			setupFlags: func(cmd *cobra.Command) {
				// Add all flags except format.
				cmd.Flags().String("query", "", "")
				cmd.Flags().Int("max-columns", DefaultMaxColumns, "")
				cmd.Flags().String("delimiter", "\t", "")
				cmd.Flags().String("stack", "", "")
			},
		},
		{
			name: "Missing delimiter flag",
			setupFlags: func(cmd *cobra.Command) {
				// Add all flags except delimiter.
				cmd.Flags().String("query", "", "")
				cmd.Flags().Int("max-columns", DefaultMaxColumns, "")
				cmd.Flags().String("format", "json", "") // Using a valid format.
				cmd.Flags().String("stack", "", "")
			},
		},
		{
			name: "Missing stack flag",
			setupFlags: func(cmd *cobra.Command) {
				// Add all flags except stack.
				cmd.Flags().String("query", "", "")
				cmd.Flags().Int("max-columns", DefaultMaxColumns, "")
				cmd.Flags().String("format", "json", "") // Using a valid format.
				cmd.Flags().String("delimiter", "\t", "")
			},
		},
		{
			name: "Invalid format",
			setupFlags: func(cmd *cobra.Command) {
				// Add all flags but provide an invalid format.
				cmd.Flags().String("query", "", "")
				cmd.Flags().Int("max-columns", DefaultMaxColumns, "")
				cmd.Flags().String("format", "invalid-format", "")
				cmd.Flags().String("delimiter", "\t", "")
				cmd.Flags().String("stack", "", "")
			},
		},
	}

	for _, tt := range flagTests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a command with the test flags.
			cmd := &cobra.Command{
				Use: "test",
				Run: func(cmd *cobra.Command, args []string) {},
			}

			// Set up the flags for this specific test.
			tt.setupFlags(cmd)

			// Try to get the flags.
			result, err := GetCommonListFlags(cmd)

			// Verify that we got an error and no result.
			assert.Error(t, err, "Should return an error")
			assert.Nil(t, result, "Result should be nil")
		})
	}
}
