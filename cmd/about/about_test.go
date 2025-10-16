package about

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAboutCmd(t *testing.T) {
	// Capture stdout since utils.PrintfMarkdown writes directly to os.Stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute the command.
	err := aboutCmd.RunE(aboutCmd, []string{})
	assert.NoError(t, err, "'atmos about' command should execute without error")

	// Close the writer and restore stdout.
	err = w.Close()
	assert.NoError(t, err, "'atmos about' command should execute without error")

	os.Stdout = oldStdout

	// Read captured output.
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'atmos about' command should execute without error")

	// Check if the output contains expected markdown content.
	assert.Contains(t, output.String(), aboutMarkdown, "'atmos about' output should contain information about Atmos")
}

func TestAboutCommandProvider(t *testing.T) {
	provider := &AboutCommandProvider{}

	// Test GetCommand.
	cmd := provider.GetCommand()
	assert.NotNil(t, cmd)
	assert.Equal(t, "about", cmd.Use)

	// Test GetName.
	assert.Equal(t, "about", provider.GetName())

	// Test GetGroup.
	assert.Equal(t, "Other Commands", provider.GetGroup())
}
