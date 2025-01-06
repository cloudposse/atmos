package logger

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func captureOutput(f func()) string {
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w

	outC := make(chan string)
	// Copy the output in a separate goroutine so printing can't block indefinitely
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()

	// Call the function which will use stdout
	f()

	// Close the writer and restore the original stdout
	w.Close()
	os.Stdout = stdout

	// Read the output string
	out := <-outC

	return out
}

func TestNewLogger(t *testing.T) {
	logger, err := NewLogger(LogLevelDebug, "/dev/stdout")
	assert.NoError(t, err)
	assert.NotNil(t, logger)
	assert.Equal(t, LogLevelDebug, logger.LogLevel)
	assert.Equal(t, "/dev/stdout", logger.File)
}

func TestNewLoggerFromCliConfig(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
			File:  "/dev/stdout",
		},
	}

	logger, err := NewLoggerFromCliConfig(atmosConfig)
	assert.NoError(t, err)
	assert.NotNil(t, logger)
	assert.Equal(t, LogLevelInfo, logger.LogLevel)
	assert.Equal(t, "/dev/stdout", logger.File)
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
		err      bool
	}{
		{"Trace", LogLevelTrace, false},
		{"Debug", LogLevelDebug, false},
		{"Info", LogLevelInfo, false},
		{"Warning", LogLevelWarning, false},
		{"Invalid", LogLevelInfo, true},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("input=%s", test.input), func(t *testing.T) {
			logLevel, err := ParseLogLevel(test.input)
			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expected, logLevel)
			}
		})
	}
}

func TestLogger_Trace(t *testing.T) {
	logger, _ := NewLogger(LogLevelTrace, "/dev/stdout")

	output := captureOutput(func() {
		logger.Trace("Trace message")
	})

	assert.Contains(t, output, "Trace message")
}

func TestLogger_Debug(t *testing.T) {
	logger, _ := NewLogger(LogLevelDebug, "/dev/stdout")

	output := captureOutput(func() {
		logger.Debug("Debug message")
	})

	assert.Contains(t, output, "Debug message")

	logger, _ = NewLogger(LogLevelTrace, "/dev/stdout")

	output = captureOutput(func() {
		logger.Debug("Trace message should appear")
	})

	assert.Contains(t, output, "Trace message should appear")
}

func TestLogger_Info(t *testing.T) {
	logger, _ := NewLogger(LogLevelInfo, "/dev/stdout")

	output := captureOutput(func() {
		logger.Info("Info message")
	})

	assert.Contains(t, output, "Info message")
}

func TestLogger_Warning(t *testing.T) {
	logger, _ := NewLogger(LogLevelWarning, "/dev/stdout")

	output := captureOutput(func() {
		logger.Warning("Warning message")
	})

	assert.Contains(t, output, "Warning message")
}

func TestLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	color.Error = &buf
	logger, _ := NewLogger(LogLevelWarning, "/dev/stderr")

	err := fmt.Errorf("This is an error")
	logger.Error(err)
	assert.Contains(t, buf.String(), "This is an error")
}

func TestLogger_FileLogging(t *testing.T) {
	tempDir := os.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	defer os.Remove(logFile)

	logger, _ := NewLogger(LogLevelInfo, logFile)
	logger.Info("File logging test")

	data, err := os.ReadFile(logFile)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "File logging test")
}

func TestLogger_SetLogLevel(t *testing.T) {
	logger, _ := NewLogger(LogLevelInfo, "/dev/stdout")

	err := logger.SetLogLevel(LogLevelDebug)
	assert.NoError(t, err)
	assert.Equal(t, LogLevelDebug, logger.LogLevel)
}
