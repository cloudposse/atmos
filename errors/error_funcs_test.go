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

// TestCheckErrorPrintAndExitWithExitCodeError tests that ExitCodeError preserves the custom exit code.
func TestCheckErrorPrintAndExitWithExitCodeError(t *testing.T) {
	tests := []struct {
		name         string
		exitCode     int
		envVar       string
		expectedCode int
	}{
		{
			name:         "exit code 2",
			exitCode:     2,
			envVar:       "TEST_EXIT_CODE_2",
			expectedCode: 2,
		},
		{
			name:         "exit code 42",
			exitCode:     42,
			envVar:       "TEST_EXIT_CODE_42",
			expectedCode: 42,
		},
		{
			name:         "exit code 127",
			exitCode:     127,
			envVar:       "TEST_EXIT_CODE_127",
			expectedCode: 127,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if os.Getenv(tt.envVar) == "1" {
				// Create an ExitCodeError and call CheckErrorPrintAndExit.
				err := ExitCodeError{Code: tt.exitCode}
				CheckErrorPrintAndExit(err, "Test Error", "")
				return
			}

			// Run test in subprocess to capture exit code.
			execPath, err := exec.LookPath(os.Args[0])
			assert.NoError(t, err)

			cmd := exec.Command(execPath, "-test.run=TestCheckErrorPrintAndExitWithExitCodeError/"+tt.name)
			cmd.Env = append(os.Environ(), tt.envVar+"=1")
			err = cmd.Run()

			// Verify the exit code matches the ExitCodeError.Code.
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				assert.Equal(t, tt.expectedCode, exitError.ExitCode(), "Exit code should match ExitCodeError.Code")
			} else {
				assert.Fail(t, "Expected an exit error with code %d", tt.expectedCode)
			}
		})
	}
}

// TestCheckErrorPrintAndExitWithExecExitError tests that exec.ExitError preserves the command exit code.
func TestCheckErrorPrintAndExitWithExecExitError(t *testing.T) {
	if os.Getenv("TEST_EXEC_EXIT_ERROR") == "1" {
		// Create a command that exits with code 5.
		cmd := exec.Command("sh", "-c", "exit 5")
		err := cmd.Run()
		CheckErrorPrintAndExit(err, "Command Failed", "")
		return
	}

	// Run test in subprocess.
	execPath, err := exec.LookPath(os.Args[0])
	assert.NoError(t, err)

	cmd := exec.Command(execPath, "-test.run=TestCheckErrorPrintAndExitWithExecExitError")
	cmd.Env = append(os.Environ(), "TEST_EXEC_EXIT_ERROR=1")
	err = cmd.Run()

	// Verify the exit code is preserved from the shell command.
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		assert.Equal(t, 5, exitError.ExitCode(), "Exit code should be preserved from exec.ExitError")
	} else {
		assert.Fail(t, "Expected an exit error with code 5")
	}
}
