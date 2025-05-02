package tests

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/cloudposse/atmos/cmd"
	"github.com/stretchr/testify/assert"
)

func TestSupportCmd(t *testing.T) {
	// Capture stdout since utils.PrintfMarkdown writes directly to os.Stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
	}()
	// Execute the command
	os.Args = []string{"atmos", "support"}
	err := cmd.Execute()
	assert.NoError(t, err, "'atmos support' command should execute without error")

	// Close the writer and restore stdout
	err = w.Close()
	assert.NoError(t, err, "'atmos support' command should execute without error")

	os.Stdout = oldStdout

	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'atmos support' command should execute without error")

	// Check if output contains expected markdown content
	assert.Contains(t, output.String(), "Connect with active users in the", "'atmos support' output should contain information about Cloud Posse Atmos support")
}
