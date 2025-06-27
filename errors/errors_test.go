package errors

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

func Test_checkErrorAndExit(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{
			name:    "nil error should not exit",
			err:     nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("CheckErrorAndExit() should have panicked with error")
					}
				}()
			}
			CheckErrorPrintMarkdownAndExit(tt.err, "", "")
		})
	}
}

func TestPrintErrorMarkdown(t *testing.T) {
	render, _ = markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})
	title := "Test Error"
	err := errors.New("this is a test error")
	suggestion := "Try checking your configuration."

	// Redirect stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	CheckErrorAndPrintMarkdown(err, title, suggestion)

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

func TestPrintErrorMarkdownAndExit(t *testing.T) {
	if os.Getenv("TEST_EXIT") == "1" {
		er := errors.New("critical failure")
		CheckErrorPrintMarkdownAndExit(er, "Fatal Error", "Check logs.")
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
		er := errors.New("invalid command")
		CheckErrorPrintMarkdownAndExit(er, "", "Use --help for usage information.")
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
