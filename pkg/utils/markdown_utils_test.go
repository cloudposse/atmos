package utils

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

func TestPrintErrorMarkdown(t *testing.T) {
	render, _ = markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})
	title := "Test Error"
	err := errors.New("this is a test error")
	suggestion := "Try checking your configuration."

	// Redirect stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	PrintErrorMarkdown(title, err, suggestion)

	// Restore stderr
	err = w.Close()
	assert.Nil(t, err)

	os.Stderr = oldStderr

	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'TestPrintErrorMarkdown' should execute without error")

	// Check if output contains the expected content
	expectedOutput := "Test Error"
	assert.Contains(t, output.String(), expectedOutput, "'TestPrintErrorMarkdown' output should contain information about the error")
}

func TestPrintfErrorMarkdown(t *testing.T) {
	render, _ = markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})

	// Redirect stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	PrintfErrorMarkdown("An error occurred: %s", "something went wrong")

	// Restore stderr
	err := w.Close()
	assert.Nil(t, err)

	os.Stderr = oldStderr

	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'TestPrintfErrorMarkdown' should execute without error")

	// Check if output contains the expected content
	expectedOutput := "An error occurred"
	assert.Contains(t, output.String(), expectedOutput, "'TestPrintfErrorMarkdown' output should contain information about the error")
}

func TestPrintErrorMarkdownAndExit(t *testing.T) {
	if os.Getenv("TEST_EXIT") == "1" {
		PrintErrorMarkdownAndExit("Fatal Error", errors.New("critical failure"), "Check logs.")
		return
	}
	execPath, err := exec.LookPath(os.Args[0])
	assert.Nil(t, err)
	cmd := exec.Command(execPath, "-test.run=TestPrintErrorMarkdownAndExit")
	cmd.Env = append(os.Environ(), "TEST_EXIT=1")
	err = cmd.Run()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		assert.Equal(t, 1, exitError.ExitCode())
	} else {
		assert.Fail(t, "Expected an exit error with code 1")
	}
}

func TestPrintInvalidUsageErrorAndExit(t *testing.T) {
	if os.Getenv("TEST_EXIT") == "1" {
		PrintInvalidUsageErrorAndExit(errors.New("invalid command"), "Use --help for usage information.")
		return
	}
	execPath, err := exec.LookPath(os.Args[0])
	assert.Nil(t, err)
	cmd := exec.Command(execPath, "-test.run=TestPrintErrorMarkdownAndExit")
	cmd.Env = append(os.Environ(), "TEST_EXIT=1")
	err = cmd.Run()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		assert.Equal(t, 1, exitError.ExitCode())
	} else {
		assert.Fail(t, "Expected an exit error with code 1")
	}
}

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
