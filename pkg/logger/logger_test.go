package logger

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewLogger(t *testing.T) {
	logger, err := NewLogger(log.DebugLevel, "/dev/stdout")
	assert.NoError(t, err)
	assert.NotNil(t, logger)
	assert.Equal(t, log.DebugLevel, logger.GetLevel())
}

func TestNewLoggerFromCliConfig(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
			File:  "/dev/stdout",
		},
	}

	logger, err := NewLoggerFromCliConfig(&atmosConfig)
	assert.NoError(t, err)
	assert.NotNil(t, logger)
	assert.Equal(t, log.InfoLevel, logger.GetLevel())
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    log.Level
		expectError bool
	}{
		{"Empty string returns Info", "", log.InfoLevel, false},
		{"Valid Trace level", "Trace", AtmosTraceLevel, false},
		{"Valid Debug level", "Debug", log.DebugLevel, false},
		{"Valid Info level", "Info", log.InfoLevel, false},
		{"Valid Warning level", "Warning", log.WarnLevel, false},
		{"Valid Off level", "Off", log.FatalLevel + 1, false},
		{"Invalid lowercase level", "trace", 0, true},
		{"Invalid mixed case level", "TrAcE", 0, true},
		{"Invalid level", "InvalidLevel", 0, true},
		{"Invalid empty spaces", "  ", 0, true},
		{"Invalid special characters", "Debug!", 0, true},
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
	var buf bytes.Buffer
	atmosLogger := log.New(&buf)
	atmosLogger.SetLevel(AtmosTraceLevel)

	testLogger := &AtmosLogger{
		Logger: atmosLogger,
	}

	// Generate trace output using our custom AtmosTraceLevel
	testLogger.Trace("Trace message")

	// Get the output and check it contains our message
	output := buf.String()
	t.Logf("Captured output: %q", output)

	// Verify the output contains our trace message
	assert.Contains(t, output, "Trace message")
}

func TestLogger_Debug(t *testing.T) {
	// Create a logger with Debug level
	logger, err := NewLogger(log.DebugLevel, "/dev/stdout")
	assert.NoError(t, err)
	assert.Equal(t, log.DebugLevel, logger.GetLevel())
	assert.NotNil(t, logger.Logger)
	loggerTrace, err := NewLogger(AtmosTraceLevel, "/dev/stdout")
	assert.NoError(t, err)
	assert.Equal(t, AtmosTraceLevel, loggerTrace.GetLevel())
	logger.Debug("Debug message")
	loggerTrace.Debug("Trace level logger should show debug messages")
}

func TestLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	atmosLogger := NewAtmosLogger(&buf)
	atmosLogger.SetLevel(log.InfoLevel)

	logger := &AtmosLogger{
		Logger: atmosLogger,
	}

	logger.Info("Info message")
	output := buf.String()
	assert.Contains(t, output, "Info message")
}

func TestLogger_Warn(t *testing.T) {
	// Test styled logger warn with stderr writer
	var buf bytes.Buffer
	atmosLogger := NewAtmosLogger(&buf)
	atmosLogger.SetLevel(log.WarnLevel)

	logger := &AtmosLogger{
		Logger: atmosLogger,
	}

	logger.Warn("Warning message")
	output := buf.String()
	assert.Contains(t, output, "Warning message")
}

func TestLogger_Error(t *testing.T) {
	// Test styled logger error with stderr writer
	var buf bytes.Buffer
	atmosLogger := NewAtmosLogger(&buf)
	atmosLogger.SetLevel(log.WarnLevel)

	logger := &AtmosLogger{
		Logger: atmosLogger,
	}

	err := fmt.Errorf("This is an error")
	logger.Error(err)
	output := buf.String()
	assert.Contains(t, output, "This is an error")

	// Test styled logger with file path
	var buf2 bytes.Buffer
	fileAtmosLogger := NewAtmosLogger(&buf2)
	fileAtmosLogger.SetLevel(log.WarnLevel)

	fileLogger := &AtmosLogger{
		Logger: fileAtmosLogger,
	}

	err2 := fmt.Errorf("This is a file error")
	fileLogger.Error(err2)
	fileOutput := buf2.String()
	assert.Contains(t, fileOutput, "This is a file error")
}

func TestLogger_FileLogging(t *testing.T) {
	tempDir := os.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	defer os.Remove(logFile)

	logger, _ := NewLogger(log.InfoLevel, logFile)
	logger.Info("File logging test")

	data, err := os.ReadFile(logFile)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "File logging test")
}

func TestLogger_SetLogLevel(t *testing.T) {
	logger, _ := NewLogger(log.InfoLevel, "/dev/stdout")

	err := logger.SetLogLevel(log.DebugLevel)
	assert.NoError(t, err)
	assert.Equal(t, log.DebugLevel, logger.GetLevel())
}

func TestLogger_GetLevel(t *testing.T) {
	tests := []struct {
		name          string
		level         log.Level
		expectedLevel log.Level
	}{
		{"Trace level is preserved", AtmosTraceLevel, AtmosTraceLevel},
		{"Debug level is preserved", log.DebugLevel, log.DebugLevel},
		{"Info level is preserved", log.InfoLevel, log.InfoLevel},
		{"Warn level is preserved", log.WarnLevel, log.WarnLevel},
		{"Error level is preserved", log.ErrorLevel, log.ErrorLevel},
		{"Fatal+1 level is preserved", log.FatalLevel + 1, log.FatalLevel + 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a logger with a buffer to avoid actual output
			var buf bytes.Buffer
			atmosLogger := log.New(&buf)
			atmosLogger.SetLevel(test.level)

			logger := &AtmosLogger{Logger: atmosLogger}
			gotLevel := logger.GetLevel()
			assert.Equal(t, test.expectedLevel, gotLevel)
		})
	}
}

func TestLogger_LogMethods(t *testing.T) {
	tests := []struct {
		name         string
		loggerLevel  log.Level
		message      string
		expectOutput bool
		logFunc      func(*AtmosLogger, string)
	}{
		{"Trace logs when level is Trace", AtmosTraceLevel, "trace message", true, (*AtmosLogger).Trace},
		{"Trace doesn't log when level is Debug", log.DebugLevel, "trace message", false, (*AtmosLogger).Trace},
		{"Debug logs when level is Trace", AtmosTraceLevel, "debug message", true, (*AtmosLogger).Debug},
		{"Debug logs when level is Debug", log.DebugLevel, "debug message", true, (*AtmosLogger).Debug},
		{"Debug doesn't log when level is Info", log.InfoLevel, "debug message", false, (*AtmosLogger).Debug},
		{"Info logs when level is Trace", AtmosTraceLevel, "info message", true, (*AtmosLogger).Info},
		{"Info logs when level is Debug", log.DebugLevel, "info message", true, (*AtmosLogger).Info},
		{"Info logs when level is Info", log.InfoLevel, "info message", true, (*AtmosLogger).Info},
		{"Info doesn't log when level is Warning", log.WarnLevel, "info message", false, (*AtmosLogger).Info},
		{"Warn logs when level is Trace", AtmosTraceLevel, "warning message", true, (*AtmosLogger).Warn},
		{"Warn logs when level is Warning", log.WarnLevel, "warning message", true, (*AtmosLogger).Warn},
		{"Nothing logs when level is Off", log.FatalLevel + 1, "any message", false, (*AtmosLogger).Info},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a buffer to capture output
			var buf bytes.Buffer

			// Create a logger with the buffer
			atmosLogger := log.New(&buf)
			atmosLogger.SetLevel(test.loggerLevel)

			logger := &AtmosLogger{
				Logger: atmosLogger,
			}

			// Call the log function
			test.logFunc(logger, test.message)

			// Read the output
			output := buf.String()

			if test.expectOutput {
				assert.Contains(t, output, test.message)
			} else {
				assert.Empty(t, output)
			}
		})
	}
}

func TestDevNullLogging(t *testing.T) {
	// Create a logger with /dev/null to verify log suppression
	logger, err := NewLogger(log.InfoLevel, "/dev/null")
	assert.NoError(t, err)
	assert.NotNil(t, logger)

	// Test that logging to /dev/null doesn't produce any output
	// Capture os.Stdout
	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	// Log a message
	logger.Info("This should be suppressed")

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify no output was generated
	assert.Empty(t, output)
}

func TestDevStdoutWarning(t *testing.T) {
	// Redirect warning logs to capture the warning
	var buf bytes.Buffer
	oldLogger := log.Default()
	newLogger := log.New(&buf)
	newLogger.SetLevel(log.WarnLevel)
	log.SetDefault(newLogger)

	// Restore the original logger after the test
	defer log.SetDefault(oldLogger)

	// Create logger to trigger warning
	_, err := NewLogger(log.InfoLevel, "/dev/stdout")
	assert.NoError(t, err)

	// Get the warning message
	warningMessage := buf.String()

	// Verify warning was generated
	assert.Contains(t, warningMessage, "Sending logs to stdout")
	assert.Contains(t, warningMessage, "break commands")
}

func TestIllegalDeviceFiles(t *testing.T) {
	// Test cases for illegal device files
	illegalDevices := []string{
		"/dev/random",
		"/dev/urandom",
		"/dev/zero",
		"/dev/tty",
	}

	for _, device := range illegalDevices {
		t.Run(fmt.Sprintf("Reject %s", device), func(t *testing.T) {
			logger, err := NewLogger(log.InfoLevel, device)
			assert.Error(t, err)
			assert.Nil(t, logger)
			assert.Contains(t, err.Error(), "unsupported device file")
		})
	}

	// Test that regular files and supported device files work
	validPaths := []string{
		"/dev/stderr",
		"/dev/stdout",
		"/dev/null",
		"test.log",
		os.TempDir() + "/test.log",
	}

	for _, path := range validPaths {
		t.Run(fmt.Sprintf("Accept %s", path), func(t *testing.T) {
			logger, err := NewLogger(log.InfoLevel, path)
			assert.NoError(t, err)
			assert.NotNil(t, logger)
		})
	}
}

func TestLoggerFromCliConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        schema.AtmosConfiguration
		expectError   bool
		expectedLevel log.Level
	}{
		{
			name: "Valid config with Info level",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Info",
					File:  "/dev/stdout",
				},
			},
			expectError:   false,
			expectedLevel: log.InfoLevel,
		},
		{
			name: "Valid config with Trace level",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Trace",
					File:  "/dev/stdout",
				},
			},
			expectError:   false,
			expectedLevel: AtmosTraceLevel,
		},
		{
			name: "Invalid log level",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Invalid",
					File:  "/dev/stdout",
				},
			},
			expectError:   true,
			expectedLevel: 0,
		},
		{
			name: "Empty log level defaults to Info",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "",
					File:  "/dev/stdout",
				},
			},
			expectError:   false,
			expectedLevel: log.InfoLevel,
		},
		{
			name: "/dev/null disables logging",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Info",
					File:  "/dev/null",
				},
			},
			expectError:   false,
			expectedLevel: log.InfoLevel,
		},
		{
			name: "Invalid device file causes error",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Info",
					File:  "/dev/random",
				},
			},
			expectError:   true,
			expectedLevel: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger, err := NewLoggerFromCliConfig(&test.config)
			if test.expectError {
				assert.Error(t, err)
				assert.Nil(t, logger)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, logger)
				assert.Equal(t, test.expectedLevel, logger.GetLevel())
			}
		})
	}
}
