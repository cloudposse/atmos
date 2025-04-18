package utils

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
)

func TestPrintMessage(t *testing.T) {
	// Save stdout
	oldStdout := os.Stdout

	// Create a pipe
	r, w, _ := os.Pipe()
	os.Stdout = w

	message := "test message"
	PrintMessage(message)

	// Close the writer to get all output
	w.Close()

	// Read the output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	// Restore stdout
	os.Stdout = oldStdout

	assert.Contains(t, buf.String(), message)
}

func TestPrintMessageInColor(t *testing.T) {
	// Save stdout
	oldStdout := os.Stdout

	// Create a pipe
	r, w, _ := os.Pipe()
	os.Stdout = w

	message := "colored message"
	testColor := color.New(color.FgBlue)
	PrintMessageInColor(message, testColor)

	// Close the writer to get all output
	w.Close()

	// Read the output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	// Restore stdout
	os.Stdout = oldStdout

	// We can't easily test the color, but we can check the message was printed
	assert.Contains(t, buf.String(), message)
}

func TestPrintErrorInColor(t *testing.T) {
	// Save stderr
	oldStderr := os.Stderr

	// Create a pipe
	r, w, _ := os.Pipe()
	os.Stderr = w

	message := "error message"
	PrintErrorInColor(message)

	// Close the writer to get all output
	w.Close()

	// Read the output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	// Restore stderr
	os.Stderr = oldStderr

	assert.Contains(t, buf.String(), message)
}

func TestLogErrorAndExit(t *testing.T) {
	// Save and restore original OsExit
	originalOsExit := OsExit
	defer func() { OsExit = originalOsExit }()

	// Create a mock for OsExit
	var exitCode int
	OsExit = func(code int) {
		exitCode = code
		// Don't exit the test
	}

	// Set up a logger mock to capture logs
	var logBuffer bytes.Buffer
	oldLogger := log.Default()
	defer func() { log.SetDefault(oldLogger) }()

	customLogger := log.NewWithOptions(
		&logBuffer,
		log.Options{
			Level: log.DebugLevel,
		},
	)
	log.SetDefault(customLogger)

	// Test with a regular error
	simpleError := errors.New("simple error")
	LogErrorAndExit(simpleError)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, logBuffer.String(), "simple error")

	// Test with an exec.ExitError
	exitError := &exec.ExitError{}
	exitError.ProcessState = &os.ProcessState{}
	// We can't easily set the exit code in the mock, but we can test the code path
	LogErrorAndExit(exitError)
}

func TestLogError(t *testing.T) {
	// Set up a logger mock to capture logs
	var logBuffer bytes.Buffer
	oldLogger := log.Default()
	defer func() { log.SetDefault(oldLogger) }()

	customLogger := log.NewWithOptions(
		&logBuffer,
		log.Options{
			Level: log.DebugLevel,
		},
	)
	log.SetDefault(customLogger)

	testError := errors.New("test error")
	LogError(testError)
	assert.Contains(t, logBuffer.String(), "test error")

	// Test with nil error
	logBuffer.Reset()
	LogError(nil)
	assert.Empty(t, logBuffer.String())
}

func TestLogLevels(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(string)
		message  string
		expected string
	}{
		{
			name:     "LogTrace",
			logFunc:  LogTrace,
			message:  "trace message",
			expected: "trace message",
		},
		{
			name:     "LogDebug",
			logFunc:  LogDebug,
			message:  "debug message",
			expected: "debug message",
		},
		{
			name:     "LogInfo",
			logFunc:  LogInfo,
			message:  "info message",
			expected: "info message",
		},
		{
			name:     "LogWarning",
			logFunc:  LogWarning,
			message:  "warning message",
			expected: "warning message",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			oldLogger := log.Default()
			defer func() { log.SetDefault(oldLogger) }()

			customLogger := log.NewWithOptions(
				&logBuffer,
				log.Options{
					Level: log.DebugLevel,
				},
			)
			log.SetDefault(customLogger)

			tc.logFunc(tc.message)
			assert.Contains(t, logBuffer.String(), tc.expected)
		})
	}
}
