package list

import (
	"bytes"
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// captureStdout captures stdout and returns a function to restore it
func captureStdout() (*bytes.Buffer, func()) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	buf := new(bytes.Buffer)
	go func() {
		buf.ReadFrom(r)
	}()
	return buf, func() {
		w.Close()
		os.Stdout = old
	}
}

// createTestCommand creates a cobra.Command with the necessary flags for testing
func createTestCommand() *cobra.Command {
	cmd := &cobra.Command{}

	// Add common flags
	cmd.Flags().String("config", "", "Config file")
	cmd.Flags().String("base-path", "", "Base path for atmos")
	cmd.Flags().String("config-dir", "", "Config directory")
	cmd.Flags().String("workflow-dir", "", "Workflow directory")
	cmd.Flags().String("components-dir", "", "Components directory")
	cmd.Flags().String("stacks-dir", "", "Stacks directory")
	cmd.Flags().String("templates-dir", "", "Templates directory")
	cmd.Flags().String("process-templates", "", "Process templates")
	cmd.Flags().String("process-functions", "", "Process functions")
	cmd.Flags().String("skip", "", "Skip")
	cmd.Flags().String("query", "", "Query")
	cmd.Flags().String("format", "", "Format")
	cmd.Flags().String("output-file", "", "Output file")
	cmd.Flags().String("logs-level", "", "Logs level")
	cmd.Flags().String("logs-file", "", "Logs file")

	// Add deployment-specific flags
	cmd.Flags().StringP("stack", "s", "", "Filter deployments by stack")
	cmd.Flags().Bool("drift-enabled", false, "Filter deployments with drift detection enabled")
	cmd.Flags().Bool("upload", false, "Upload deployments to pro API")

	return cmd
}

func TestExecuteListDeploymentsCmd(t *testing.T) {
	testCases := []struct {
		name        string
		args        []string
		info        schema.ConfigAndStacksInfo
		setupMocks  func() // placeholder for future dependency injection
		expectError bool
	}{
		{
			name:        "success - no args",
			args:        []string{},
			info:        schema.ConfigAndStacksInfo{},
			setupMocks:  func() {},
			expectError: false,
		},
		{
			name:        "error from ProcessCommandLineArgs",
			args:        []string{"--bad-flag"},
			info:        schema.ConfigAndStacksInfo{},
			setupMocks:  func() {},
			expectError: true,
		},
		{
			name:        "drift detection filter",
			args:        []string{"--drift-enabled"},
			info:        schema.ConfigAndStacksInfo{},
			setupMocks:  func() {},
			expectError: false,
		},
		{
			name:        "upload flag",
			args:        []string{"--upload"},
			info:        schema.ConfigAndStacksInfo{},
			setupMocks:  func() {},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks if/when refactored for DI
			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			// Create test command
			cmd := createTestCommand()
			cmd.ParseFlags(tc.args)

			// Capture stdout for output validation
			buf, restore := captureStdout()
			defer restore()

			err := ExecuteListDeploymentsCmd(tc.info, cmd, tc.args)
			if tc.expectError {
				assert.Error(t, err, "expected error but got nil")
			} else {
				assert.NoError(t, err, "expected no error but got: %v", err)
			}

			// Optionally validate output
			output := buf.String()
			t.Logf("Output: %s", output)
		})
	}
}
