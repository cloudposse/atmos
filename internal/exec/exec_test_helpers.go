package exec

import (
	"bytes"
	"os"
	"strings"
	"testing"
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
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stdout = w

	// Ensure stdout restoration happens even if commandFunc panics.
	defer func() {
		os.Stdout = oldStdout
	}()

	// Execute the command.
	err = commandFunc()
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	// Close the writer before reading to send EOF to the reader.
	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close pipe writer: %v", err)
	}

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
