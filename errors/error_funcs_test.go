package errors

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

func Test_CheckErrorPrintAndExit(t *testing.T) {
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
			CheckErrorPrintAndExit(tt.err, "", "")
		})
	}
}

func TestPrintErrorMarkdownAndExit(t *testing.T) {
	if os.Getenv("TEST_EXIT") == "1" {
		er := errors.New("critical failure")
		CheckErrorPrintAndExit(er, "Fatal Error", "Check logs.")
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
		CheckErrorPrintAndExit(er, "", "Use --help for usage information.")
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

func TestCheckErrorPrintAndExit_ExitCodeError(t *testing.T) {
	if os.Getenv("TEST_EXIT_CODE_2") == "1" {
		err := ExitCodeError{Code: 2}
		CheckErrorPrintAndExit(err, "Exit Code Error", "")
		return
	}
	if os.Getenv("TEST_EXIT_CODE_42") == "1" {
		err := ExitCodeError{Code: 42}
		CheckErrorPrintAndExit(err, "Exit Code Error", "")
		return
	}

	// Test exit code 2
	execPath, err := exec.LookPath(os.Args[0])
	assert.Nil(t, err)
	cmd := exec.Command(execPath, "-test.run=TestCheckErrorPrintAndExit_ExitCodeError")
	cmd.Env = append(os.Environ(), "TEST_EXIT_CODE_2=1")
	err = cmd.Run()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		assert.Equal(t, 2, exitError.ExitCode(), "Should exit with code 2")
	} else {
		assert.Fail(t, "Expected an exit error with code 2")
	}

	// Test exit code 42
	cmd = exec.Command(execPath, "-test.run=TestCheckErrorPrintAndExit_ExitCodeError")
	cmd.Env = append(os.Environ(), "TEST_EXIT_CODE_42=1")
	err = cmd.Run()
	if errors.As(err, &exitError) {
		assert.Equal(t, 42, exitError.ExitCode(), "Should exit with code 42")
	} else {
		assert.Fail(t, "Expected an exit error with code 42")
	}
}

func TestCheckErrorPrintAndExit_ExecExitError(t *testing.T) {
	if os.Getenv("TEST_EXEC_EXIT") == "1" {
		// Create an exec.ExitError
		cmd := exec.Command("sh", "-c", "exit 3")
		err := cmd.Run()
		CheckErrorPrintAndExit(err, "Exec Error", "")
		return
	}

	execPath, err := exec.LookPath(os.Args[0])
	assert.Nil(t, err)
	cmd := exec.Command(execPath, "-test.run=TestCheckErrorPrintAndExit_ExecExitError")
	cmd.Env = append(os.Environ(), "TEST_EXEC_EXIT=1")
	err = cmd.Run()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		assert.Equal(t, 3, exitError.ExitCode(), "Should exit with code 3 from exec.ExitError")
	} else {
		assert.Fail(t, "Expected an exit error with code 3")
	}
}

func TestCheckErrorAndPrint(t *testing.T) {
	// Save original logger
	originalLogger := log.Default()
	defer log.SetDefault(originalLogger)

	// Create test logger to capture log output
	var logBuf bytes.Buffer
	testLogger := log.New()
	testLogger.SetOutput(&logBuf)
	testLogger.SetLevel(log.TraceLevel)
	log.SetDefault(testLogger)

	render, _ = markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})
	title := "Test Error"
	err := errors.New("this is a test error")
	suggestion := "Try checking your configuration."

	// Redirect stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	CheckErrorAndPrint(err, title, suggestion)

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

	// Test with nil render to improve coverage
	t.Run("nil render", func(t *testing.T) {
		// Save and nil render
		originalRender := render
		render = nil
		defer func() {
			render = originalRender
		}()

		logBuf.Reset()
		testErr := errors.New("error with nil render")
		CheckErrorAndPrint(testErr, "Test", "Suggestion")

		// Should log error directly
		assert.Contains(t, logBuf.String(), "error with nil render")
	})

	// Test with empty title (defaults to "Error")
	t.Run("empty title", func(t *testing.T) {
		render, _ = markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})

		r, w, _ := os.Pipe()
		os.Stderr = w

		testErr := errors.New("test error with empty title")
		CheckErrorAndPrint(testErr, "", "")

		w.Close()
		os.Stderr = oldStderr

		var output bytes.Buffer
		io.Copy(&output, r)

		// Should default to "Error" as title
		assert.Contains(t, output.String(), "Error")
	})
}

func TestInitializeMarkdown(t *testing.T) {
	// Save original logger
	originalLogger := log.Default()
	defer log.SetDefault(originalLogger)

	// Create test logger to capture log output
	var logBuf bytes.Buffer
	testLogger := log.New()
	testLogger.SetOutput(&logBuf)
	testLogger.SetLevel(log.TraceLevel)
	log.SetDefault(testLogger)

	// Test with valid configuration
	t.Run("valid configuration", func(t *testing.T) {
		logBuf.Reset()
		atmosConfig := schema.AtmosConfiguration{}
		InitializeMarkdown(atmosConfig)

		// Should initialize without error
		assert.NotContains(t, logBuf.String(), "failed to initialize Markdown renderer")
	})
}
