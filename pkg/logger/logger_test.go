package logger

import (
	"bytes"
	"errors"
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

func TestInitializeLogger(t *testing.T) {
	logger, err := InitializeLogger(LogLevelDebug, "/dev/stdout")
	assert.NoError(t, err)
	assert.NotNil(t, logger)
	assert.Equal(t, LogLevelDebug, logger.LogLevel)
	assert.Equal(t, "/dev/stdout", logger.File)

	assert.NotNil(t, logger.charm, "Charmbracelet logger should be initialized")
}

func TestInitializeLoggerFromCliConfig(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
			File:  "/dev/stdout",
		},
	}

	logger, err := InitializeLoggerFromCliConfig(&atmosConfig)
	assert.NoError(t, err)
	assert.NotNil(t, logger)
	assert.Equal(t, LogLevelInfo, logger.LogLevel)
	assert.Equal(t, "/dev/stdout", logger.File)
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    LogLevel
		expectError bool
	}{
		{"Empty string returns Info", "", LogLevelInfo, false},
		{"Valid Trace level", "Trace", LogLevelTrace, false},
		{"Valid Debug level", "Debug", LogLevelDebug, false},
		{"Valid Info level", "Info", LogLevelInfo, false},
		{"Valid Warning level", "Warning", LogLevelWarning, false},
		{"Valid Off level", "Off", LogLevelOff, false},
		{"Invalid lowercase level", "trace", "", true},
		{"Invalid mixed case level", "TrAcE", "", true},
		{"Invalid level", "InvalidLevel", "", true},
		{"Invalid empty spaces", "  ", "", true},
		{"Invalid special characters", "Debug!", "", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level, err := ParseLogLevel(test.input)
			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expected, level)
			}
		})
	}
}

func TestLogger_Trace(t *testing.T) {
	logger, _ := InitializeLogger(LogLevelTrace, "/dev/stdout")

	output := captureOutput(func() {
		logger.Trace("Trace message")
	})

	assert.Contains(t, output, "Trace message")
}

func TestLogger_Debug(t *testing.T) {
	logger, _ := InitializeLogger(LogLevelDebug, "/dev/stdout")

	output := captureOutput(func() {
		logger.Debug("Debug message")
	})

	assert.Contains(t, output, "Debug message")

	logger, _ = InitializeLogger(LogLevelTrace, "/dev/stdout")

	output = captureOutput(func() {
		logger.Debug("Trace message should appear")
	})

	assert.Contains(t, output, "Trace message should appear")
}

func TestLogger_Info(t *testing.T) {
	logger, _ := InitializeLogger(LogLevelInfo, "/dev/stdout")

	output := captureOutput(func() {
		logger.Info("Info message")
	})

	assert.Contains(t, output, "Info message")
}

func TestLogger_Warning(t *testing.T) {
	logger, _ := InitializeLogger(LogLevelWarning, "/dev/stdout")

	output := captureOutput(func() {
		logger.Warning("Warning message")
	})

	assert.Contains(t, output, "Warning message")
}

// ErrTest is a static test error.
var ErrTest = errors.New("This is an error")

func TestLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	color.Error = &buf
	logger, _ := InitializeLogger(LogLevelWarning, "/dev/stderr")

	logger.Error(ErrTest)
	assert.Contains(t, buf.String(), "This is an error")
}

func TestLogger_FileLogging(t *testing.T) {
	tempDir := os.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	defer os.Remove(logFile)

	logger, _ := InitializeLogger(LogLevelInfo, logFile)
	logger.Info("File logging test")

	data, err := os.ReadFile(logFile)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "File logging test")
}

func TestLogger_SetLogLevel(t *testing.T) {
	logger, _ := InitializeLogger(LogLevelInfo, "/dev/stdout")

	err := logger.SetLogLevel(LogLevelDebug)
	assert.NoError(t, err)
	assert.Equal(t, LogLevelDebug, logger.LogLevel)
}

func TestLogger_isLevelEnabled(t *testing.T) {
	tests := []struct {
		name          string
		currentLevel  LogLevel
		checkLevel    LogLevel
		expectEnabled bool
	}{
		{"Trace enables all levels", LogLevelTrace, LogLevelTrace, true},
		{"Trace enables Debug", LogLevelTrace, LogLevelDebug, true},
		{"Trace enables Info", LogLevelTrace, LogLevelInfo, true},
		{"Trace enables Warning", LogLevelTrace, LogLevelWarning, true},
		{"Debug disables Trace", LogLevelDebug, LogLevelTrace, false},
		{"Debug enables Debug", LogLevelDebug, LogLevelDebug, true},
		{"Debug enables Info", LogLevelDebug, LogLevelInfo, true},
		{"Debug enables Warning", LogLevelDebug, LogLevelWarning, true},
		{"Info disables Trace", LogLevelInfo, LogLevelTrace, false},
		{"Info disables Debug", LogLevelInfo, LogLevelDebug, false},
		{"Info enables Info", LogLevelInfo, LogLevelInfo, true},
		{"Info enables Warning", LogLevelInfo, LogLevelWarning, true},
		{"Warning disables Trace", LogLevelWarning, LogLevelTrace, false},
		{"Warning disables Debug", LogLevelWarning, LogLevelDebug, false},
		{"Warning disables Info", LogLevelWarning, LogLevelInfo, false},
		{"Warning enables Warning", LogLevelWarning, LogLevelWarning, true},
		{"Off disables all levels", LogLevelOff, LogLevelTrace, false},
		{"Off disables Debug", LogLevelOff, LogLevelDebug, false},
		{"Off disables Info", LogLevelOff, LogLevelInfo, false},
		{"Off disables Warning", LogLevelOff, LogLevelWarning, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := &Logger{LogLevel: test.currentLevel}
			enabled := logger.isLevelEnabled(test.checkLevel)
			assert.Equal(t, test.expectEnabled, enabled)
		})
	}
}

func TestLogger_LogMethods(t *testing.T) {
	tests := []struct {
		name         string
		loggerLevel  LogLevel
		message      string
		expectOutput bool
		logFunc      func(*Logger, string)
	}{
		{"Trace logs when level is Trace", LogLevelTrace, "trace message", true, (*Logger).Trace},
		{"Trace doesn't log when level is Debug", LogLevelDebug, "trace message", false, (*Logger).Trace},
		{"Debug logs when level is Trace", LogLevelTrace, "debug message", true, (*Logger).Debug},
		{"Debug logs when level is Debug", LogLevelDebug, "debug message", true, (*Logger).Debug},
		{"Debug doesn't log when level is Info", LogLevelInfo, "debug message", false, (*Logger).Debug},
		{"Info logs when level is Trace", LogLevelTrace, "info message", true, (*Logger).Info},
		{"Info logs when level is Debug", LogLevelDebug, "info message", true, (*Logger).Info},
		{"Info logs when level is Info", LogLevelInfo, "info message", true, (*Logger).Info},
		{"Info doesn't log when level is Warning", LogLevelWarning, "info message", false, (*Logger).Info},
		{"Warning logs when level is Trace", LogLevelTrace, "warning message", true, (*Logger).Warning},
		{"Warning logs when level is Warning", LogLevelWarning, "warning message", true, (*Logger).Warning},
		{"Nothing logs when level is Off", LogLevelOff, "any message", false, (*Logger).Info},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Pipe to capture output
			r, w, _ := os.Pipe()
			oldStdout := os.Stdout
			os.Stdout = w

			// Channel to capture output
			outC := make(chan string)
			go func() {
				var buf bytes.Buffer
				io.Copy(&buf, r)
				outC <- buf.String()
			}()

			logger, _ := InitializeLogger(test.loggerLevel, "/dev/stdout")
			test.logFunc(logger, test.message)

			// Close the writer and restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read the output
			output := <-outC

			if test.expectOutput {
				assert.Contains(t, output, test.message)
			} else {
				assert.Empty(t, output)
			}
		})
	}
}

func TestLoggerFromCliConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      schema.AtmosConfiguration
		expectError bool
	}{
		{
			name: "Valid config with Info level",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Info",
					File:  "/dev/stdout",
				},
			},
			expectError: false,
		},
		{
			name: "Valid config with Trace level",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Trace",
					File:  "/dev/stdout",
				},
			},
			expectError: false,
		},
		{
			name: "Invalid log level",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Invalid",
					File:  "/dev/stdout",
				},
			},
			expectError: true,
		},
		{
			name: "Empty log level defaults to Info",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "",
					File:  "/dev/stdout",
				},
			},
			expectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger, err := InitializeLoggerFromCliConfig(&test.config)
			if test.expectError {
				assert.Error(t, err)
				assert.Nil(t, logger)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, logger)
				if test.config.Logs.Level == "" {
					assert.Equal(t, LogLevelInfo, logger.LogLevel)
				} else {
					assert.Equal(t, LogLevel(test.config.Logs.Level), logger.LogLevel)
				}
				assert.Equal(t, test.config.Logs.File, logger.File)
			}
		})
	}
}
