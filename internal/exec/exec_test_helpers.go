package exec

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// testCaptureCommandOutput executes a command in a specific directory and captures its stdout output.
// It handles directory changes, stdout capture, and output verification.
func testCaptureCommandOutput(t *testing.T, workDir string, commandFunc func() error, expectedOutput string) {
	t.Helper()

	// Capture the starting working directory.
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test.
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the work directory and change to it.
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	// Create a pipe to capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute the command.
	err = commandFunc()
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	// Restore stdout.
	err = w.Close()
	assert.NoError(t, err)
	os.Stdout = oldStdout

	// Read the captured output.
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}
	output := buf.String()

	if !strings.Contains(output, expectedOutput) {
		t.Errorf("%s not found in the output", expectedOutput)
	}
}
