package cmd

import (
	"bytes"
	"errors"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

var (
	errMockListStacks = errors.New("mock listStacks error")
	errInitCliConfig  = errors.New("init cli config error")
	errDescribeStacks = errors.New("describe stacks error")
)

// TestRunListStacksHandlesListStacksError tests that the Run function properly handles errors from listStacks.
func TestRunListStacksHandlesListStacksError(t *testing.T) {
	originalListStacks := listStacksFn
	originalCheckAtmosConfig := checkAtmosConfigFn

	listStacksFn = func(cmd *cobra.Command) ([]string, error) {
		return nil, errMockListStacks
	}

	checkAtmosConfigFn = func(opts ...AtmosValidateOption) {}

	defer func() {
		listStacksFn = originalListStacks
		checkAtmosConfigFn = originalCheckAtmosConfig
	}()

	var buf bytes.Buffer
	testLogger := log.New(&buf)
	testLogger.SetLevel(log.DebugLevel)
	testLogger.SetReportTimestamp(false)
	testLogger.SetReportCaller(false)

	originalLogger := log.Default()
	log.SetDefault(testLogger)
	defer log.SetDefault(originalLogger)

	cmd := &cobra.Command{
		Use: "teststacks",
		Run: listStacksCmd.Run,
	}

	cmd.Run(cmd, []string{})

	assert.Contains(t, buf.String(), "error filtering stacks")
	assert.Contains(t, buf.String(), "mock listStacks error")
}

// testLogError is a helper function to test error logging with different error scenarios
func testLogError(t *testing.T, errorMsg string, err error, expectedOutput []string) {
	var buf bytes.Buffer
	testLogger := log.New(&buf)
	testLogger.SetLevel(log.DebugLevel)
	testLogger.SetReportTimestamp(false)
	testLogger.SetReportCaller(false)

	originalLogger := log.Default()
	log.SetDefault(testLogger)
	defer log.SetDefault(originalLogger)

	originalListStacks := listStacksFn

	listStacksFn = func(cmd *cobra.Command) ([]string, error) {
		log.Error(errorMsg, "error", err)
		return nil, err
	}

	defer func() {
		listStacksFn = originalListStacks
	}()

	originalCheckAtmosConfig := checkAtmosConfigFn
	checkAtmosConfigFn = func(opts ...AtmosValidateOption) {}
	defer func() {
		checkAtmosConfigFn = originalCheckAtmosConfig
	}()

	cmd := &cobra.Command{
		Use: "teststacks",
		Run: listStacksCmd.Run,
	}

	cmd.Run(cmd, []string{})

	for _, expected := range expectedOutput {
		assert.Contains(t, buf.String(), expected)
	}
}

// TestLogErrorCliConfigInitialize tests the error logging for CLI config initialization errors.
func TestLogErrorCliConfigInitialize(t *testing.T) {
	testLogError(t,
		"failed to initialize CLI config",
		errInitCliConfig,
		[]string{"failed to initialize CLI config", "init cli config error"})
}

// TestLogErrorDescribeStacks tests the error logging for ExecuteDescribeStacks errors.
func TestLogErrorDescribeStacks(t *testing.T) {
	testLogError(t,
		"failed to describe stacks",
		errDescribeStacks,
		[]string{"failed to describe stacks", "describe stacks error"})
}
