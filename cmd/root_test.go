package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
)

func TestNoColorLog(t *testing.T) {
	// Set the environment variable to disable color
	// t.Setenv("NO_COLOR", "1")
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")
	t.Setenv("NO_COLOR", "1")
	// Create a buffer to capture the output
	var buf bytes.Buffer
	log.SetOutput(&buf)

	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
	}()
	// Set the arguments for the command
	os.Args = []string{"atmos", "about"}
	// Execute the command
	if err := Execute(); err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
	// Check if the output is without color
	output := buf.String()
	if strings.Contains(output, "\033[") {
		t.Errorf("Expected no color in output, but got: %s", output)
	}
	t.Log(output, "output")
}

func TestInitFunction(t *testing.T) {
	// Save the original state
	originalArgs := os.Args
	originalEnvVars := make(map[string]string)
	defer func() {
		// Restore original state
		os.Args = originalArgs
		for k, v := range originalEnvVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// Test cases
	tests := []struct {
		name           string
		setup          func()
		validate       func(t *testing.T)
		expectedErrMsg string
	}{
		{
			name: "verify command setup",
			setup: func() {
				// No special setup needed
			},
			validate: func(t *testing.T) {
				// Verify subcommands are properly registered
				assert.NotNil(t, RootCmd.Commands())
				// Add specific command checks if needed
			},
		},
		{
			name: "verify version command setup",
			setup: func() {
				// No special setup needed
			},
			validate: func(t *testing.T) {
				// Verify the version command is properly configured
				versionCmd, _, _ := RootCmd.Find([]string{"version"})
				assert.NotNil(t, versionCmd, "version command should be registered")
				// Add more specific version command checks
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.setup != nil {
				tt.setup()
			}

			// The init() function runs automatically when the package is imported
			// We can verify its effects through the RootCmd and other package-level variables

			// Validate
			if tt.validate != nil {
				tt.validate(t)
			}
		})
	}
}
