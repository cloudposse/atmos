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
		name          string
		level         string
		expectedLevel LogLevel
		expectError   bool
	}{
		{
			name:          "valid trace level",
			level:         "Trace",
			expectedLevel: LogLevelTrace,
			expectError:   false,
		},
		{
			name:          "valid debug level",
			level:         "Debug",
			expectedLevel: LogLevelDebug,
			expectError:   false,
		},
		{
			name:          "valid info level",
			level:         "Info",
			expectedLevel: LogLevelInfo,
			expectError:   false,
		},
		{
			name:          "valid warning level",
			level:         "Warning",
			expectedLevel: LogLevelWarning,
			expectError:   false,
		},
		{
			name:          "valid off level",
			level:         "Off",
			expectedLevel: LogLevelOff,
			expectError:   false,
		},
		{
			name:        "invalid lowercase level",
			level:       "debug",
			expectError: true,
		},
		{
			name:        "invalid mixed case level",
			level:       "DeBuG",
			expectError: true,
		},
		{
			name:        "empty level",
			level:       "",
			expectError: true,
		},
		{
			name:        "invalid level",
			level:       "invalid",
			expectError: true,
		},
		{
			name:        "special characters",
			level:       "Debug!",
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level, err := ParseLogLevel(test.level)
			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedLevel, level)
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
		name            string
		loggerLevel     string
		messageLevel    string
		message         string
		shouldBePrinted bool
	}{
		{
			name:            "trace logs when trace enabled",
			loggerLevel:     "Trace",
			messageLevel:    "TRACE",
			message:         "trace message",
			shouldBePrinted: true,
		},
		{
			name:            "debug logs when trace enabled",
			loggerLevel:     "Trace",
			messageLevel:    "DEBUG",
			message:         "debug message",
			shouldBePrinted: true,
		},
		{
			name:            "info logs when trace enabled",
			loggerLevel:     "Trace",
			messageLevel:    "INFO",
			message:         "info message",
			shouldBePrinted: true,
		},
		{
			name:            "warning logs when trace enabled",
			loggerLevel:     "Trace",
			messageLevel:    "WARNING",
			message:         "warning message",
			shouldBePrinted: true,
		},
		{
			name:            "trace not logged when debug enabled",
			loggerLevel:     "Debug",
			messageLevel:    "TRACE",
			message:         "trace message",
			shouldBePrinted: false,
		},
		{
			name:            "debug logged when debug enabled",
			loggerLevel:     "Debug",
			messageLevel:    "DEBUG",
			message:         "debug message",
			shouldBePrinted: true,
		},
		{
			name:            "info logged when info enabled",
			loggerLevel:     "Info",
			messageLevel:    "INFO",
			message:         "info message",
			shouldBePrinted: true,
		},
		{
			name:            "debug not logged when info enabled",
			loggerLevel:     "Info",
			messageLevel:    "DEBUG",
			message:         "debug message",
			shouldBePrinted: false,
		},
		{
			name:            "warning logged when warning enabled",
			loggerLevel:     "Warning",
			messageLevel:    "WARNING",
			message:         "warning message",
			shouldBePrinted: true,
		},
		{
			name:            "info not logged when warning enabled",
			loggerLevel:     "Warning",
			messageLevel:    "INFO",
			message:         "info message",
			shouldBePrinted: false,
		},
		{
			name:            "nothing logged when off",
			loggerLevel:     "Off",
			messageLevel:    "WARNING",
			message:         "warning message",
			shouldBePrinted: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a pipe to capture output
			r, w, err := os.Pipe()
			assert.NoError(t, err)

			// Save original stdout and replace it with our pipe
			originalStdout := os.Stdout
			os.Stdout = w

			// Create logger with test level
			config := schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: test.loggerLevel,
					File:  "/dev/stdout",
				},
			}
			logger, err := NewLoggerFromCliConfig(config)
			assert.NoError(t, err)

			// Log message based on level
			switch test.messageLevel {
			case "TRACE":
				logger.Trace(test.message)
			case "DEBUG":
				logger.Debug(test.message)
			case "INFO":
				logger.Info(test.message)
			case "WARNING":
				logger.Warning(test.message)
			}

			// Close the write end of the pipe to flush it
			w.Close()

			// Read the output
			output := make([]byte, 1024)
			n, _ := r.Read(output)
			outputStr := string(output[:n])

			// Restore original stdout
			os.Stdout = originalStdout

			if test.shouldBePrinted {
				assert.Contains(t, outputStr, test.message)
				assert.Contains(t, outputStr, "["+test.messageLevel+"]")
			} else {
				assert.Empty(t, outputStr)
			}
		})
	}
}

func TestNewLoggerFromCliConfig_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		config      schema.AtmosConfiguration
		envVars     map[string]string
		expectError bool
		checkLevel  bool
		wantLevel   LogLevel
	}{
		{
			name: "valid config with debug level",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Debug",
					File:  "/dev/stdout",
				},
			},
			expectError: false,
			checkLevel:  true,
			wantLevel:   LogLevelDebug,
		},
		{
			name: "valid config with trace level",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Trace",
					File:  "/dev/stdout",
				},
			},
			expectError: false,
			checkLevel:  true,
			wantLevel:   LogLevelTrace,
		},
		{
			name: "invalid log level in config",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Invalid",
					File:  "/dev/stdout",
				},
			},
			expectError: true,
		},
		{
			name: "environment variable overrides config",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Info",
					File:  "/dev/stdout",
				},
			},
			envVars: map[string]string{
				"ATMOS_LOGS_LEVEL": "Debug",
			},
			expectError: false,
			checkLevel:  true,
			wantLevel:   LogLevelDebug,
		},
		{
			name: "invalid environment variable",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Info",
					File:  "/dev/stdout",
				},
			},
			envVars: map[string]string{
				"ATMOS_LOGS_LEVEL": "Invalid",
			},
			expectError: true,
		},
		{
			name: "empty log level defaults to Info",
			config: schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "",
					File:  "/dev/stdout",
				},
			},
			expectError: false,
			checkLevel:  true,
			wantLevel:   LogLevelInfo,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range test.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			logger, err := NewLoggerFromCliConfig(test.config)
			if test.expectError {
				assert.Error(t, err)
				assert.Nil(t, logger)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, logger)
				if test.checkLevel {
					assert.Equal(t, test.wantLevel, logger.LogLevel)
				}
				assert.Equal(t, test.config.Logs.File, logger.File)
			}
		})
	}
}
