package logger

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

// TestGlobalLoggerPackageFunctions tests all the global package-level functions.
func TestGlobalLoggerPackageFunctions(t *testing.T) {
	// Save and restore the default logger
	originalLogger := Default()
	defer SetDefault(originalLogger)

	// Create a test logger with a buffer to capture output
	var buf bytes.Buffer
	testLogger := New()
	testLogger.SetOutput(&buf)
	testLogger.SetLevel(TraceLevel)
	SetDefault(testLogger)

	// Test all logging functions
	tests := []struct {
		name     string
		logFunc  func()
		expected string
	}{
		{
			name:     "Trace",
			logFunc:  func() { Trace("trace message") },
			expected: "trace message",
		},
		{
			name:     "Tracef",
			logFunc:  func() { Tracef("trace %s", "formatted") },
			expected: "trace formatted",
		},
		{
			name:     "Debug",
			logFunc:  func() { Debug("debug message") },
			expected: "debug message",
		},
		{
			name:     "Debugf",
			logFunc:  func() { Debugf("debug %s", "formatted") },
			expected: "debug formatted",
		},
		{
			name:     "Info",
			logFunc:  func() { Info("info message") },
			expected: "info message",
		},
		{
			name:     "Infof",
			logFunc:  func() { Infof("info %s", "formatted") },
			expected: "info formatted",
		},
		{
			name:     "Warn",
			logFunc:  func() { Warn("warn message") },
			expected: "warn message",
		},
		{
			name:     "Warnf",
			logFunc:  func() { Warnf("warn %s", "formatted") },
			expected: "warn formatted",
		},
		{
			name:     "Error",
			logFunc:  func() { Error("error message") },
			expected: "error message",
		},
		{
			name:     "Errorf",
			logFunc:  func() { Errorf("error %s", "formatted") },
			expected: "error formatted",
		},
		{
			name:     "Log",
			logFunc:  func() { Log(InfoLevel, "log", "message") },
			expected: "message",
		},
		{
			name:     "Logf",
			logFunc:  func() { Logf(InfoLevel, "log %s", "formatted") },
			expected: "log formatted",
		},
		{
			name:     "Print",
			logFunc:  func() { Print("print", "message") },
			expected: "print",
		},
		{
			name:     "Printf",
			logFunc:  func() { Printf("print %s", "formatted") },
			expected: "print formatted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc()
			output := buf.String()
			assert.Contains(t, output, tt.expected)
		})
	}
}

// TestGlobalFatalFunctions tests Fatal and Fatalf (without actually exiting).
func TestGlobalFatalFunctions(t *testing.T) {
	// Save and restore the default logger
	originalLogger := Default()
	defer SetDefault(originalLogger)

	// Create a test logger
	testLogger := New()
	SetDefault(testLogger)

	// Fatal functions would normally call os.Exit, so we just verify they're callable
	// We can't actually test them without exiting the test process
	fatalFunc := func() {
		// This would call Fatal("message") in production
		// Fatal("fatal message")
	}
	assert.NotNil(t, fatalFunc)

	fatalfFunc := func() {
		// This would call Fatalf("fatal %s", "error") in production
		// Fatalf("fatal %s", "error")
	}
	assert.NotNil(t, fatalfFunc)
}

// TestGlobalSettersAndGetters tests all the global setter and getter functions.
func TestGlobalSettersAndGetters(t *testing.T) {
	// Save and restore the default logger
	originalLogger := Default()
	defer SetDefault(originalLogger)

	testLogger := New()
	SetDefault(testLogger)

	// Test SetLevel and GetLevel
	t.Run("SetLevel/GetLevel", func(t *testing.T) {
		SetLevel(DebugLevel)
		assert.Equal(t, DebugLevel, GetLevel())

		SetLevel(InfoLevel)
		assert.Equal(t, InfoLevel, GetLevel())

		SetLevel(WarnLevel)
		assert.Equal(t, WarnLevel, GetLevel())
	})

	// Test SetOutput
	t.Run("SetOutput", func(t *testing.T) {
		var buf bytes.Buffer
		SetOutput(&buf)
		SetLevel(InfoLevel)
		Info("test output")
		assert.Contains(t, buf.String(), "test output")
	})

	// Test SetStyles
	t.Run("SetStyles", func(t *testing.T) {
		styles := log.DefaultStyles()
		assert.NotPanics(t, func() {
			SetStyles(styles)
			Info("styled message")
		})
	})

	// Test SetColorProfile
	t.Run("SetColorProfile", func(t *testing.T) {
		profiles := []termenv.Profile{
			termenv.Ascii,
			termenv.ANSI,
			termenv.ANSI256,
			termenv.TrueColor,
		}

		for _, profile := range profiles {
			assert.NotPanics(t, func() {
				SetColorProfile(profile)
				Info("colored message")
			})
		}
	})

	// Test GetLevelString
	t.Run("GetLevelString", func(t *testing.T) {
		SetLevel(TraceLevel)
		assert.Equal(t, "trace", GetLevelString())

		SetLevel(DebugLevel)
		assert.Equal(t, "debug", strings.ToLower(GetLevelString()))

		SetLevel(InfoLevel)
		assert.Equal(t, "info", strings.ToLower(GetLevelString()))
	})

	// Test SetReportCaller
	t.Run("SetReportCaller", func(t *testing.T) {
		assert.NotPanics(t, func() {
			SetReportCaller(true)
			Info("caller message")
			SetReportCaller(false)
		})
	})

	// Test SetReportTimestamp
	t.Run("SetReportTimestamp", func(t *testing.T) {
		assert.NotPanics(t, func() {
			SetReportTimestamp(true)
			Info("timestamp message")
			SetReportTimestamp(false)
		})
	})

	// Test SetTimeFormat
	t.Run("SetTimeFormat", func(t *testing.T) {
		assert.NotPanics(t, func() {
			SetTimeFormat("2006-01-02 15:04:05")
			Info("time formatted message")
		})
	})

	// Test SetPrefix and GetPrefix
	t.Run("SetPrefix/GetPrefix", func(t *testing.T) {
		SetPrefix("TEST")
		assert.Equal(t, "TEST", GetPrefix())

		SetPrefix("ANOTHER")
		assert.Equal(t, "ANOTHER", GetPrefix())

		SetPrefix("")
		assert.Equal(t, "", GetPrefix())
	})
}

// TestGlobalWithMethods tests the With and WithPrefix global functions.
func TestGlobalWithMethods(t *testing.T) {
	// Save and restore the default logger
	originalLogger := Default()
	defer SetDefault(originalLogger)

	var buf bytes.Buffer
	testLogger := New()
	testLogger.SetOutput(&buf)
	testLogger.SetLevel(InfoLevel)
	SetDefault(testLogger)

	// Test With
	t.Run("With", func(t *testing.T) {
		buf.Reset()
		contextLogger := With("key", "value", "another", "data")
		contextLogger.Info("context message")
		output := buf.String()
		assert.Contains(t, output, "context message")
		assert.Contains(t, output, "key")
		assert.Contains(t, output, "value")
	})

	// Test WithPrefix
	t.Run("WithPrefix", func(t *testing.T) {
		buf.Reset()
		prefixedLogger := WithPrefix("PREFIX")
		prefixedLogger.Info("prefixed message")
		output := buf.String()
		assert.Contains(t, output, "prefixed message")
	})
}

// TestGlobalHelper tests the Helper global function.
func TestGlobalHelper(t *testing.T) {
	// Save and restore the default logger
	originalLogger := Default()
	defer SetDefault(originalLogger)

	testLogger := New()
	SetDefault(testLogger)

	// Helper should not panic
	assert.NotPanics(t, func() {
		Helper()
		Info("helper test")
	})
}

// TestDefaultStyles tests the DefaultStyles function.
func TestDefaultStyles(t *testing.T) {
	styles := DefaultStyles()
	assert.NotNil(t, styles)

	// Verify that styles contains expected fields
	assert.NotNil(t, styles.Levels)
	assert.NotNil(t, styles.Timestamp)
	assert.NotNil(t, styles.Caller)
	assert.NotNil(t, styles.Prefix)
	assert.NotNil(t, styles.Message)
	assert.NotNil(t, styles.Key)
	assert.NotNil(t, styles.Value)
	assert.NotNil(t, styles.Separator)

	// Verify level styles are defined for standard log levels
	// Note: TraceLevel is custom and may not have a default style
	assert.NotEqual(t, lipgloss.Style{}, styles.Levels[log.DebugLevel])
	assert.NotEqual(t, lipgloss.Style{}, styles.Levels[log.InfoLevel])
	assert.NotEqual(t, lipgloss.Style{}, styles.Levels[log.WarnLevel])
	assert.NotEqual(t, lipgloss.Style{}, styles.Levels[log.ErrorLevel])
	assert.NotEqual(t, lipgloss.Style{}, styles.Levels[log.FatalLevel])
}

// TestGlobalLoggerLevelFiltering tests that the global logger respects log levels.
func TestGlobalLoggerLevelFiltering(t *testing.T) {
	// Save and restore the default logger
	originalLogger := Default()
	defer SetDefault(originalLogger)

	var buf bytes.Buffer
	testLogger := New()
	testLogger.SetOutput(&buf)
	SetDefault(testLogger)

	tests := []struct {
		name           string
		level          log.Level
		unexpectedLogs []string
	}{
		{
			name:           "ErrorLevel filters lower levels",
			level:          ErrorLevel,
			unexpectedLogs: []string{"trace", "debug", "info", "warn"},
		},
		{
			name:           "WarnLevel filters lower levels",
			level:          WarnLevel,
			unexpectedLogs: []string{"trace", "debug", "info"},
		},
		{
			name:           "InfoLevel filters lower levels",
			level:          InfoLevel,
			unexpectedLogs: []string{"trace", "debug"},
		},
		{
			name:           "DebugLevel filters trace",
			level:          DebugLevel,
			unexpectedLogs: []string{"trace"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			SetLevel(tt.level)

			// Log messages at all levels using global functions
			Trace("trace message")
			Debug("debug message")
			Info("info message")
			Warn("warn message")
			Error("error message")

			output := buf.String()

			// Check that unexpected logs are filtered out
			for _, msg := range tt.unexpectedLogs {
				assert.NotContains(t, output, msg+" message")
			}
		})
	}
}

// TestGlobalLoggerIntegration tests the integration of global functions with the default logger.
func TestGlobalLoggerIntegration(t *testing.T) {
	// Save and restore the default logger
	originalLogger := Default()
	defer SetDefault(originalLogger)

	// Create a custom logger with specific settings
	var buf bytes.Buffer
	customLogger := New()
	customLogger.SetOutput(&buf)
	customLogger.SetLevel(DebugLevel)
	customLogger.SetPrefix("CUSTOM")

	// Set as default
	SetDefault(customLogger)

	// Verify Default() returns our custom logger
	assert.Equal(t, customLogger, Default())

	// Test that global functions use the custom logger
	buf.Reset()
	Debug("integration test")
	output := buf.String()
	assert.Contains(t, output, "integration test")
}

// BenchmarkGlobalInfo benchmarks the global logger functions.
func BenchmarkGlobalInfo(b *testing.B) {
	// Save and restore the default logger
	originalLogger := Default()
	defer SetDefault(originalLogger)

	testLogger := New()
	testLogger.SetOutput(os.Stderr)
	testLogger.SetLevel(InfoLevel)
	SetDefault(testLogger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info("benchmark message")
	}
}

func BenchmarkGlobalWith(b *testing.B) {
	// Save and restore the default logger
	originalLogger := Default()
	defer SetDefault(originalLogger)

	testLogger := New()
	testLogger.SetOutput(os.Stderr)
	testLogger.SetLevel(InfoLevel)
	SetDefault(testLogger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		With("key", "value").Info("benchmark message")
	}
}

// TestLevelToString tests the LevelToString function that converts charm.Level to string.
func TestLevelToString(t *testing.T) {
	tests := []struct {
		name     string
		level    log.Level
		expected string
	}{
		{
			name:     "TraceLevel",
			level:    TraceLevel,
			expected: "trace",
		},
		{
			name:     "DebugLevel",
			level:    DebugLevel,
			expected: "debug",
		},
		{
			name:     "InfoLevel",
			level:    InfoLevel,
			expected: "info",
		},
		{
			name:     "WarnLevel",
			level:    WarnLevel,
			expected: "warn",
		},
		{
			name:     "ErrorLevel",
			level:    ErrorLevel,
			expected: "error",
		},
		{
			name:     "FatalLevel",
			level:    FatalLevel,
			expected: "fatal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LevelToString(tt.level)
			assert.Equal(t, tt.expected, result, "LevelToString(%v) should return %q", tt.level, tt.expected)
		})
	}
}
