package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/tests"
)

func TestTerraformGenerateBackendCmd(t *testing.T) {
	_ = NewTestKit(t)

	tests.RequireTerraform(t)

	stacksPath := "../tests/fixtures/scenarios/stack-templates"

	// Use t.Setenv() for automatic cleanup by test framework.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")

	// Reset flag states to prevent pollution from other tests.
	// Only reset flags that were actually changed to avoid issues with complex flag types.
	defer func() {
		RootCmd.PersistentFlags().Visit(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}()

	// Capture stderr without a bounded pipe that can block Debug logging.
	outputStr := captureTerraformGenerateStderr(t, func() {
		// Execute the command.
		RootCmd.SetArgs([]string{"terraform", "generate", "backend", "component-1", "-s", "nonprod"})
		err := Execute()
		assert.NoError(t, err, "'TestTerraformGenerateBackendCmd' should execute without error")
	})

	// Expected output after processing the templates in the component's `backend` section.
	expectedOutput := "nonprod-tfstate-lock"

	// Check if the output contains the expected output.
	assert.Contains(t, outputStr, expectedOutput, "'TestTerraformGenerateBackendCmd' output should contain information about the generated backend")
}

// captureTerraformGenerateStderr captures synchronous command output without pipe backpressure.
func captureTerraformGenerateStderr(t *testing.T, run func()) string {
	t.Helper()

	output, err := os.Create(filepath.Join(t.TempDir(), "stderr.log"))
	require.NoError(t, err)

	oldStderr, oldLogger := os.Stderr, log.Default()
	os.Stderr = output
	// Use an independent logger so Execute cannot leave the previous logger writing to a closed file.
	log.SetDefault(log.New())
	defer func() {
		os.Stderr = oldStderr
		log.SetDefault(oldLogger)
		require.NoError(t, output.Close())
	}()

	run()
	captured, err := os.ReadFile(output.Name())
	require.NoError(t, err)
	return string(captured)
}

func TestCaptureTerraformGenerateStderrLargeDebugOutput(t *testing.T) {
	_ = NewTestKit(t)

	// This record exceeds the pipe buffer and reproduces the original maskedWriter deadlock.
	payload := strings.Repeat("large-debug-record ", 1<<16)
	output := captureTerraformGenerateStderr(t, func() {
		log.SetOutput(iolib.MaskWriter(os.Stderr))
		log.SetLevel(log.DebugLevel)
		log.Debug(payload)
	})
	assert.Contains(t, output, payload)
}

func TestCaptureTerraformGenerateStderrRestoresAfterPanic(t *testing.T) {
	_ = NewTestKit(t)

	oldStderr, oldLogger := os.Stderr, log.Default()
	var capturedFile *os.File
	assert.Panics(t, func() {
		captureTerraformGenerateStderr(t, func() {
			capturedFile = os.Stderr
			panic("command failed")
		})
	})
	assert.Same(t, oldStderr, os.Stderr)
	assert.Same(t, oldLogger, log.Default())
	require.NotNil(t, capturedFile)
	_, err := capturedFile.WriteString("after cleanup")
	assert.ErrorIs(t, err, os.ErrClosed)
}
