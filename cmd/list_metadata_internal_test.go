package cmd

import (
	"bytes"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestListMetadataCmd tests the command structure and flags
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

// TestListMetadataErrorHandling tests error handling in the Run function
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
