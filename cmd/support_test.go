package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupportCmd(t *testing.T) {
	// Capture stdout since utils.PrintfMarkdown writes directly to os.Stdout
	oldStdout := os.Stdout
	t.Log("Step 1")
	r, w, _ := os.Pipe()
	os.Stdout = w
	// Execute the command
	t.Log("Step 2")
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
	}()
	os.Args = []string{"atmos", "support"}
	err := Execute()
	assert.NoError(t, err, "'atmos support' command should execute without error")
	t.Log("Step 3")

	// Close the writer and restore stdout
	err = w.Close()
	assert.NoError(t, err, "'atmos support' command should execute without error")
	t.Log("Step 4")

	os.Stdout = oldStdout
	t.Log("Step 5")
	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'atmos support' command should execute without error")
	t.Log("Step 6")

	// Check if output contains expected markdown content
	assert.Contains(t, output.String(), "Need help? Join the", "'atmos support' output should contain information about Cloud Posse Atmos support")
}
