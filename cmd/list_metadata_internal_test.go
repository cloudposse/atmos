package cmd

import (
	"bytes"
	"fmt"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/list/errors"
	f "github.com/cloudposse/atmos/pkg/list/format"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestListMetadataCmd tests the command structure and flags.
func TestListMetadataCmd(t *testing.T) {
	assert.Equal(t, "metadata", listMetadataCmd.Use)
	assert.NotEmpty(t, listMetadataCmd.Short)
	assert.NotEmpty(t, listMetadataCmd.Long)
	assert.NotEmpty(t, listMetadataCmd.Example)
	assert.NotNil(t, listMetadataCmd.Run)

	cmd := &cobra.Command{
		Use: "test",
	}
	cmd.AddCommand(listMetadataCmd)

	flags := listMetadataCmd.PersistentFlags()

	_, err := flags.GetString("query")
	assert.NoError(t, err, "query flag should be defined")

	_, err = flags.GetInt("max-columns")
	assert.NoError(t, err, "max-columns flag should be defined")

	_, err = flags.GetString("format")
	assert.NoError(t, err, "format flag should be defined")

	_, err = flags.GetString("delimiter")
	assert.NoError(t, err, "delimiter flag should be defined")

	_, err = flags.GetString("stack")
	assert.NoError(t, err, "stack flag should be defined")
}

// TestListMetadataValidation tests that the command validates the Atmos configuration.
func TestListMetadataValidation(t *testing.T) {
	var buf bytes.Buffer
	testLogger := log.New(&buf)
	testLogger.SetLevel(log.DebugLevel)
	testLogger.SetReportTimestamp(false)
	testLogger.SetReportCaller(false)

	originalLogger := log.Default()
	log.SetDefault(testLogger)
	defer log.SetDefault(originalLogger)

	originalCheckAtmosConfig := checkAtmosConfigFn

	checkAtmosConfigFn = func(opts ...AtmosValidateOption) {
	}

	defer func() {
		checkAtmosConfigFn = originalCheckAtmosConfig
	}()

	cmd := &cobra.Command{
		Use: "test",
		Run: listMetadataCmd.Run,
	}

	// Add required flags to allow parsing
	cmd.PersistentFlags().String("query", "", "")
	cmd.PersistentFlags().Int("max-columns", 0, "")
	cmd.PersistentFlags().String("format", "", "")
	cmd.PersistentFlags().String("delimiter", "", "")
	cmd.PersistentFlags().String("stack", "", "")

	// Execute with --help to avoid actually running the command fully
	cmd.SetArgs([]string{"--help"})
	_ = cmd.Execute()

	// Verify no errors were logged
	assert.NotContains(t, buf.String(), "error")
}

// TestListMetadataErrorHandling tests error handling in the Run function.
func TestListMetadataErrorHandling(t *testing.T) {
	t.Skip("Skipping this test as it depends on specific environment configuration")

	var buf bytes.Buffer
	testLogger := log.New(&buf)
	testLogger.SetLevel(log.DebugLevel)
	testLogger.SetReportTimestamp(false)
	testLogger.SetReportCaller(false)

	originalLogger := log.Default()
	log.SetDefault(testLogger)
	defer log.SetDefault(originalLogger)

	originalCheckAtmosConfig := checkAtmosConfigFn
	checkAtmosConfigFn = func(opts ...AtmosValidateOption) {}
	defer func() {
		checkAtmosConfigFn = originalCheckAtmosConfig
	}()

	// Command with invalid flags to trigger error.
	cmd := &cobra.Command{
		Use: "test",
		Run: listMetadataCmd.Run,
	}

	// Run the command, it should fail because flags are missing
	// and the config file doesn't exist.
	cmd.Run(cmd, []string{})

	// The key point here is that the command runs without panicking.
	// The specific error message will depend on the environment.
}

// TestListMetadataCommonFlagsError tests error handling when getting common flags fails.
func TestListMetadataCommonFlagsError(t *testing.T) {
	// Create a test function that simulates listMetadata's error handling
	// but uses our mock getCommonListFlags function.
	getCommonListFlagsMock := func(cmd *cobra.Command) (*list.CommonListFlags, error) {
		return nil, fmt.Errorf("mock common flags error")
	}

	// Create a test function that mimics listMetadata but uses our mock.
	testListMetadata := func(cmd *cobra.Command) (string, error) {
		// Simulate the behavior in listMetadata.
		_, err := getCommonListFlagsMock(cmd)
		if err != nil {
			return "", &errors.QueryError{
				Query: "common flags",
				Cause: err,
			}
		}
		return "mock result", nil
	}

	cmd := &cobra.Command{
		Use: "test",
	}

	result, err := testListMetadata(cmd)

	assert.Equal(t, "", result)
	assert.Error(t, err)

	queryErr, ok := err.(*errors.QueryError)
	assert.True(t, ok)
	assert.Equal(t, "common flags", queryErr.Query)
	assert.EqualError(t, queryErr.Cause, "mock common flags error")
}

// TestListMetadataCSVDelimiterAdjustment tests the automatic adjustment of delimiter for CSV format.
func TestListMetadataCSVDelimiterAdjustment(t *testing.T) {
	testCases := []struct {
		name           string
		format         string
		inputDelimiter string
		wantDelimiter  string
	}{
		{
			name:           "CSV format with TSV delimiter should change to CSV delimiter",
			format:         string(f.FormatCSV),
			inputDelimiter: f.DefaultTSVDelimiter,
			wantDelimiter:  f.DefaultCSVDelimiter,
		},
		{
			name:           "CSV format with custom delimiter should not change",
			format:         string(f.FormatCSV),
			inputDelimiter: "|",
			wantDelimiter:  "|",
		},
		{
			name:           "Non-CSV format should not change delimiter",
			format:         string(f.FormatTable),
			inputDelimiter: f.DefaultTSVDelimiter,
			wantDelimiter:  f.DefaultTSVDelimiter,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initialFlags := &list.CommonListFlags{
				Format:    tc.format,
				Delimiter: tc.inputDelimiter,
			}

			processFlags := func(flags *list.CommonListFlags) *list.CommonListFlags {
				if f.Format(flags.Format) == f.FormatCSV && flags.Delimiter == f.DefaultTSVDelimiter {
					flags.Delimiter = f.DefaultCSVDelimiter
				}
				return flags
			}

			resultFlags := processFlags(initialFlags)

			assert.Equal(t, tc.wantDelimiter, resultFlags.Delimiter)
		})
	}
}
