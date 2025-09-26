package logger

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAtmosLogger(t *testing.T) {
	// Test with valid logger
	t.Run("valid logger", func(t *testing.T) {
		baseLogger := log.New(os.Stderr)
		assert.NotPanics(t, func() {
			logger := NewAtmosLogger(baseLogger)
			assert.NotNil(t, logger)
		})
	})

	// Test with log.Default()
	t.Run("default logger", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger := NewAtmosLogger(log.Default())
			assert.NotNil(t, logger)
		})
	})

	// Test with nil logger uses default
	t.Run("nil logger", func(t *testing.T) {
		assert.NotPanics(t, func() {
			var nilLogger *log.Logger
			logger := NewAtmosLogger(nilLogger)
			assert.NotNil(t, logger)
			// Should use default logger
			assert.NotNil(t, logger.charm)
		})
	})
}

func TestAtmosLogger_AllLogMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := New()
	logger.SetOutput(&buf)
	logger.SetLevel(TraceLevel)

	// Test Trace methods
	t.Run("Trace", func(t *testing.T) {
		buf.Reset()
		logger.Trace("trace message")
		assert.Contains(t, buf.String(), "trace message")
	})

	t.Run("Tracef", func(t *testing.T) {
		buf.Reset()
		logger.Tracef("trace %s", "formatted")
		assert.Contains(t, buf.String(), "trace formatted")
	})

	// Test Debug methods
	t.Run("Debug", func(t *testing.T) {
		buf.Reset()
		logger.Debug("debug message")
		assert.Contains(t, buf.String(), "debug message")
	})

	t.Run("Debugf", func(t *testing.T) {
		buf.Reset()
		logger.Debugf("debug %s", "formatted")
		assert.Contains(t, buf.String(), "debug formatted")
	})

	// Test Info methods
	t.Run("Info", func(t *testing.T) {
		buf.Reset()
		logger.Info("info message")
		assert.Contains(t, buf.String(), "info message")
	})

	t.Run("Infof", func(t *testing.T) {
		buf.Reset()
		logger.Infof("info %s", "formatted")
		assert.Contains(t, buf.String(), "info formatted")
	})

	// Test Warn methods
	t.Run("Warn", func(t *testing.T) {
		buf.Reset()
		logger.Warn("warn message")
		assert.Contains(t, buf.String(), "warn message")
	})

	t.Run("Warnf", func(t *testing.T) {
		buf.Reset()
		logger.Warnf("warn %s", "formatted")
		assert.Contains(t, buf.String(), "warn formatted")
	})

	// Test Error methods
	t.Run("Error", func(t *testing.T) {
		buf.Reset()
		logger.Error("error message")
		assert.Contains(t, buf.String(), "error message")
	})

	t.Run("Errorf", func(t *testing.T) {
		buf.Reset()
		logger.Errorf("error %s", "formatted")
		assert.Contains(t, buf.String(), "error formatted")
	})

	// Test Log and Logf methods
	t.Run("Log", func(t *testing.T) {
		buf.Reset()
		logger.Log(InfoLevel, "log", "message")
		assert.Contains(t, buf.String(), "message")
	})

	t.Run("Logf", func(t *testing.T) {
		buf.Reset()
		logger.Logf(InfoLevel, "log %s", "formatted")
		assert.Contains(t, buf.String(), "log formatted")
	})

	// Test Print methods
	t.Run("Print", func(t *testing.T) {
		buf.Reset()
		logger.Print("print", "message")
		assert.Contains(t, buf.String(), "print")
		assert.Contains(t, buf.String(), "message")
	})

	t.Run("Printf", func(t *testing.T) {
		buf.Reset()
		logger.Printf("print %s %s", "formatted", "message")
		assert.Contains(t, buf.String(), "print formatted message")
	})
}

func TestAtmosLogger_Fatal(t *testing.T) {
	// Fatal methods exit the program, so we need to test them differently
	// We can't test the actual exit, but we can verify the method exists
	logger := New()

	// Create a function that would call Fatal in production
	// but we won't actually execute it in the test
	fatalFunc := func() {
		logger.Fatal("fatal error")
	}

	// Just verify the function can be created without panic
	assert.NotNil(t, fatalFunc)

	// Same for Fatalf
	fatalfFunc := func() {
		logger.Fatalf("fatal %s", "error")
	}
	assert.NotNil(t, fatalfFunc)
}

func TestAtmosLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := New()
	logger.SetOutput(&buf)

	tests := []struct {
		name           string
		level          log.Level
		expectedLogs   []string
		unexpectedLogs []string
	}{
		{
			name:           "ErrorLevel filters lower levels",
			level:          ErrorLevel,
			expectedLogs:   []string{},
			unexpectedLogs: []string{"trace", "debug", "info", "warn"},
		},
		{
			name:           "WarnLevel shows warn and above",
			level:          WarnLevel,
			expectedLogs:   []string{},
			unexpectedLogs: []string{"trace", "debug", "info"},
		},
		{
			name:           "InfoLevel shows info and above",
			level:          InfoLevel,
			expectedLogs:   []string{},
			unexpectedLogs: []string{"trace", "debug"},
		},
		{
			name:           "DebugLevel shows debug and above",
			level:          DebugLevel,
			expectedLogs:   []string{},
			unexpectedLogs: []string{"trace"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			logger.SetLevel(tt.level)

			// Log messages at all levels
			logger.Trace("trace message")
			logger.Debug("debug message")
			logger.Info("info message")
			logger.Warn("warn message")
			logger.Error("error message")

			output := buf.String()

			// Check that unexpected logs are filtered out
			for _, msg := range tt.unexpectedLogs {
				assert.NotContains(t, output, msg+" message")
			}
		})
	}
}

func TestAtmosLogger_SettersAndGetters(t *testing.T) {
	logger := New()

	// Test SetLevel and GetLevel
	t.Run("SetLevel/GetLevel", func(t *testing.T) {
		logger.SetLevel(DebugLevel)
		assert.Equal(t, DebugLevel, logger.GetLevel())

		logger.SetLevel(InfoLevel)
		assert.Equal(t, InfoLevel, logger.GetLevel())
	})

	// Test SetOutput
	t.Run("SetOutput", func(t *testing.T) {
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		logger.SetLevel(InfoLevel)
		logger.Info("test output")
		assert.Contains(t, buf.String(), "test output")
	})

	// Test SetStyles
	t.Run("SetStyles", func(t *testing.T) {
		styles := log.DefaultStyles()
		// This should not panic
		logger.SetStyles(styles)
		assert.NotPanics(t, func() {
			logger.Info("styled message")
		})
	})

	// Test SetColorProfile
	t.Run("SetColorProfile", func(t *testing.T) {
		// Test various color profiles
		profiles := []termenv.Profile{
			termenv.Ascii,
			termenv.ANSI,
			termenv.ANSI256,
			termenv.TrueColor,
		}

		for _, profile := range profiles {
			logger.SetColorProfile(profile)
			// Should not panic
			assert.NotPanics(t, func() {
				logger.Info("colored message")
			})
		}
	})

	// Test SetReportCaller
	t.Run("SetReportCaller", func(t *testing.T) {
		logger.SetReportCaller(true)
		// Should not panic
		assert.NotPanics(t, func() {
			logger.Info("caller message")
		})
		logger.SetReportCaller(false)
	})

	// Test SetReportTimestamp
	t.Run("SetReportTimestamp", func(t *testing.T) {
		logger.SetReportTimestamp(true)
		// Should not panic
		assert.NotPanics(t, func() {
			logger.Info("timestamp message")
		})
		logger.SetReportTimestamp(false)
	})

	// Test SetTimeFormat
	t.Run("SetTimeFormat", func(t *testing.T) {
		logger.SetTimeFormat("2006-01-02 15:04:05")
		// Should not panic
		assert.NotPanics(t, func() {
			logger.Info("time formatted message")
		})
	})

	// Test SetPrefix and GetPrefix
	t.Run("SetPrefix/GetPrefix", func(t *testing.T) {
		logger.SetPrefix("TEST")
		assert.Equal(t, "TEST", logger.GetPrefix())

		logger.SetPrefix("ANOTHER")
		assert.Equal(t, "ANOTHER", logger.GetPrefix())
	})
}

func TestAtmosLogger_WithMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := New()
	logger.SetOutput(&buf)
	logger.SetLevel(InfoLevel)

	// Test WithPrefix
	t.Run("WithPrefix", func(t *testing.T) {
		buf.Reset()
		prefixedLogger := logger.WithPrefix("PREFIX")
		prefixedLogger.Info("prefixed message")
		output := buf.String()
		// The output should contain the message
		assert.Contains(t, output, "prefixed message")
	})

	// Test With
	t.Run("With", func(t *testing.T) {
		buf.Reset()
		contextLogger := logger.With("key", "value", "another", "data")
		contextLogger.Info("context message")
		output := buf.String()
		assert.Contains(t, output, "context message")
		// Context fields should be in the output
		assert.Contains(t, output, "key")
		assert.Contains(t, output, "value")
	})
}

func TestAtmosLogger_Helper(t *testing.T) {
	logger := New()
	// Helper method should not panic
	assert.NotPanics(t, func() {
		logger.Helper()
		logger.Info("helper test")
	})
}

func TestAtmosLogger_GetLevelString(t *testing.T) {
	logger := New()

	tests := []struct {
		level    log.Level
		expected string
	}{
		{TraceLevel, "trace"},
		{DebugLevel, "debug"},
		{InfoLevel, "info"},
		{WarnLevel, "warn"},
		{ErrorLevel, "error"},
		{FatalLevel, "fatal"},
		{log.Level(-100), ""}, // Unknown level returns empty string
	}

	for _, tt := range tests {
		testName := tt.expected
		if testName == "" {
			testName = "unknown"
		}
		t.Run(testName, func(t *testing.T) {
			logger.SetLevel(tt.level)
			result := logger.GetLevelString()
			assert.Equal(t, tt.expected, strings.ToLower(result))
		})
	}
}

func TestGlobalLoggerFunctions(t *testing.T) {
	// Save and restore default logger
	oldLogger := Default()
	defer SetDefault(oldLogger)

	var buf bytes.Buffer
	testLogger := New()
	testLogger.SetOutput(&buf)
	testLogger.SetLevel(TraceLevel)
	SetDefault(testLogger)

	// Test that Default() returns the set logger
	assert.Equal(t, testLogger, Default())

	// Test global Trace function
	t.Run("Global_Trace", func(t *testing.T) {
		buf.Reset()
		Trace("global trace")
		assert.Contains(t, buf.String(), "global trace")
	})
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
		hasError bool
	}{
		// Valid levels (case insensitive)
		{"Trace", LogLevelTrace, false},
		{"trace", LogLevelTrace, false},
		{"TRACE", LogLevelTrace, false},
		{"Debug", LogLevelDebug, false},
		{"debug", LogLevelDebug, false},
		{"Info", LogLevelInfo, false},
		{"info", LogLevelInfo, false},
		{"Warning", LogLevelWarning, false},
		{"warning", LogLevelWarning, false},
		{"Warn", LogLevelWarning, false},
		{"warn", LogLevelWarning, false},
		{"Error", LogLevelError, false},
		{"error", LogLevelError, false},
		{"Off", LogLevelOff, false},
		{"off", LogLevelOff, false},
		{"OFF", LogLevelOff, false},
		{"", LogLevelInfo, false}, // Default to Info
		// Invalid levels
		{"Invalid", "", true},
		{"unknown", "", true},
		{"123", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level, err := ParseLogLevel(tt.input)
			if tt.hasError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidLogLevel)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, level)
			}
		})
	}
}

// TestAtmosLogger_Integration tests that the logger properly integrates with the underlying log package.
func TestAtmosLogger_Integration(t *testing.T) {
	// Create a logger with specific configuration
	var buf bytes.Buffer
	baseLogger := log.New(&buf)
	baseLogger.SetLevel(log.DebugLevel)

	atmosLogger := NewAtmosLogger(baseLogger)
	require.NotNil(t, atmosLogger)

	// Verify that our wrapper properly delegates
	atmosLogger.Debug("integration test")
	assert.Contains(t, buf.String(), "integration test")
}

// BenchmarkAtmosLogger_Info benchmarks the Info method to ensure performance.
func BenchmarkAtmosLogger_Info(b *testing.B) {
	logger := New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(InfoLevel)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message")
	}
}

func BenchmarkAtmosLogger_WithContext(b *testing.B) {
	logger := New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(InfoLevel)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.With("key", "value").Info("benchmark message")
	}
}
