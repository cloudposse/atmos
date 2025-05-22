package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	log "github.com/charmbracelet/log"
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
