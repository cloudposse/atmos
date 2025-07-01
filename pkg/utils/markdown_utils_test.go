package utils

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

func TestPrintfMarkdown(t *testing.T) {
	render, _ = markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintfMarkdown("Atmos: %s", "Manage Environments Easily in Terraform")

	err := w.Close()
	assert.Nil(t, err)

	os.Stdout = oldStdout

	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'TestPrintfMarkdown' should execute without error")

	// Check if output contains the expected content
	expectedOutput := "Atmos: Manage Environments Easily in Terraform"
	assert.Contains(t, output.String(), expectedOutput, "'TestPrintfMarkdown' output should contain information about Atmos")
}

func TestInitializeMarkdown(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	InitializeMarkdown(atmosConfig)
	assert.NotNil(t, render)
}
