package errors

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
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
	// Use os.Args[0] to get path to test binary for subprocess execution.
	// This is the correct Go testing pattern for testing os.Exit behavior.
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
	// Use os.Args[0] to get path to test binary for subprocess execution.
	// This is the correct Go testing pattern for testing os.Exit behavior.
	execPath, err := exec.LookPath(os.Args[0])
	assert.Nil(t, err)
	cmd := exec.Command(execPath, "-test.run=TestPrintInvalidUsageErrorAndExit")
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

	// Use os.Args[0] to get path to test binary for subprocess execution.
	// This is the correct Go testing pattern for testing os.Exit behavior.
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
		// Create an exec.ExitError using platform-appropriate command.
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", "exit 3")
		} else {
			cmd = exec.Command("sh", "-c", "exit 3")
		}
		err := cmd.Run()
		CheckErrorPrintAndExit(err, "Exec Error", "")
		return
	}

	// Use os.Args[0] to get path to test binary for subprocess execution.
	// This is the correct Go testing pattern for testing os.Exit behavior.
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

func TestCheckErrorPrintAndExit_ExitCodeZero(t *testing.T) {
	if os.Getenv("TEST_EXIT_CODE_ZERO") == "1" {
		// Test that ExitCodeError{Code: 0} exits with code 0 without printing error
		err := ExitCodeError{Code: 0}
		CheckErrorPrintAndExit(err, "This should not print", "")
		return
	}

	// Use os.Args[0] to get path to test binary for subprocess execution.
	// This is the correct Go testing pattern for testing os.Exit behavior.
	execPath, err := exec.LookPath(os.Args[0])
	assert.Nil(t, err)
	cmd := exec.Command(execPath, "-test.run=TestCheckErrorPrintAndExit_ExitCodeZero")
	cmd.Env = append(os.Environ(), "TEST_EXIT_CODE_ZERO=1")

	// Capture stderr to verify no error is printed
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Run()
	// ExitCodeError{Code: 0} should exit with code 0 (success)
	if err != nil {
		t.Errorf("Expected successful exit (code 0), got error: %v", err)
	}

	// Verify no error message was printed to stderr
	stderrOutput := stderr.String()
	if stderrOutput != "" {
		t.Errorf("Expected no error output, got: %s", stderrOutput)
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
	r, w, pipeErr := os.Pipe()
	assert.NoError(t, pipeErr, "failed to create pipe")
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

		r, w, pipeErr := os.Pipe()
		assert.NoError(t, pipeErr, "failed to create pipe")
		originalStderr := os.Stderr
		os.Stderr = w
		defer func() {
			os.Stderr = originalStderr
		}()

		testErr := errors.New("error with nil render")
		CheckErrorAndPrint(testErr, "Test", "Suggestion")

		err := w.Close()
		assert.NoError(t, err, "failed to close pipe writer")

		var buf bytes.Buffer
		_, err = buf.ReadFrom(r)
		assert.NoError(t, err, "failed to read from pipe")
		output := buf.String()

		// Should output plain text error to stderr
		assert.Contains(t, output, "error with nil render")
		assert.Contains(t, output, "Test")
		assert.Contains(t, output, "Suggestion")
	})

	// Test with empty title (defaults to "Error")
	t.Run("empty title", func(t *testing.T) {
		render, _ = markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})

		r, w, pipeErr := os.Pipe()
		assert.NoError(t, pipeErr, "failed to create pipe")
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
		InitializeMarkdown(&atmosConfig)

		// Should initialize without error
		assert.NotContains(t, logBuf.String(), "failed to initialize Markdown renderer")
	})
}

func TestPrintStructuredPlainError(t *testing.T) {
	// Save original stderr.
	oldStderr := os.Stderr

	t.Run("error with ErrorBuilder enrichments", func(t *testing.T) {
		// Create a rich error using ErrorBuilder.
		err := Build(ErrProfileNotFound).
			WithExplanationf("Profile `%s` not found in any configured location", "test-profile").
			WithExplanation("Searched in: `/path/.atmos/profiles/test-profile`").
			WithHint("Run `atmos profile list` to see all available profiles").
			WithHint("Create the profile directory in one of the search locations").
			WithContext("profile", "test-profile").
			WithContext("searched_paths", "/path/.atmos/profiles/test-profile").
			WithExitCode(2).
			Err()

		// Capture stderr.
		r, w, pipeErr := os.Pipe()
		assert.NoError(t, pipeErr, "failed to create pipe")
		os.Stderr = w

		printStructuredPlainError(err, "", "")

		// Restore stderr.
		w.Close()
		os.Stderr = oldStderr

		var output bytes.Buffer
		_, readErr := io.Copy(&output, r)
		assert.NoError(t, readErr)

		out := output.String()

		// Should contain the error message.
		assert.Contains(t, out, "profile not found", "output should contain base error")

		// Should contain explanations.
		assert.Contains(t, out, "Explanation:", "output should have explanation section")
		assert.Contains(t, out, "Profile `test-profile` not found", "output should contain profile name in explanation")
		assert.Contains(t, out, "Searched in:", "output should show searched paths")

		// Should contain hints.
		assert.Contains(t, out, "Hints:", "output should have hints section")
		assert.Contains(t, out, "atmos profile list", "output should contain profile list hint")
		assert.Contains(t, out, "Create the profile directory", "output should contain creation hint")

		// Should contain context.
		assert.Contains(t, out, "Context:", "output should have context section")
		assert.Contains(t, out, "profile:", "output should show profile context")
	})

	t.Run("simple error without enrichments", func(t *testing.T) {
		// Create a simple error without ErrorBuilder.
		err := errors.New("simple error message")

		// Capture stderr.
		r, w, pipeErr := os.Pipe()
		assert.NoError(t, pipeErr, "failed to create pipe")
		os.Stderr = w

		printStructuredPlainError(err, "Custom Title", "legacy suggestion")

		// Restore stderr.
		w.Close()
		os.Stderr = oldStderr

		var output bytes.Buffer
		_, readErr := io.Copy(&output, r)
		assert.NoError(t, readErr)

		out := output.String()

		// Should contain custom title.
		assert.Contains(t, out, "Custom Title:", "output should contain custom title")

		// Should contain error message.
		assert.Contains(t, out, "simple error message", "output should contain error message")

		// Should contain legacy suggestion.
		assert.Contains(t, out, "legacy suggestion", "output should contain legacy suggestion")

		// Should NOT contain section headers since no enrichments.
		assert.NotContains(t, out, "Explanation:", "output should not have explanation section for simple error")
		assert.NotContains(t, out, "Hints:", "output should not have hints section for simple error")
		assert.NotContains(t, out, "Context:", "output should not have context section for simple error")
	})

	t.Run("default title when empty", func(t *testing.T) {
		err := errors.New("test error")

		// Capture stderr.
		r, w, pipeErr := os.Pipe()
		assert.NoError(t, pipeErr, "failed to create pipe")
		os.Stderr = w

		printStructuredPlainError(err, "", "")

		// Restore stderr.
		w.Close()
		os.Stderr = oldStderr

		var output bytes.Buffer
		_, readErr := io.Copy(&output, r)
		assert.NoError(t, readErr)

		// Should default to "Error" when title is empty.
		assert.Contains(t, output.String(), "Error:", "output should default to 'Error' title")
	})
}
