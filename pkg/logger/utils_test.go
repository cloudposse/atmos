package logger

import (
	"math"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
)

func TestConvertLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    LogLevel
		expected log.Level
	}{
		{
			name:     "Convert Trace",
			input:    LogLevelTrace,
			expected: TraceLevel,
		},
		{
			name:     "Convert Debug",
			input:    LogLevelDebug,
			expected: DebugLevel,
		},
		{
			name:     "Convert Info",
			input:    LogLevelInfo,
			expected: InfoLevel,
		},
		{
			name:     "Convert Warning",
			input:    LogLevelWarning,
			expected: WarnLevel,
		},
		{
			name:     "Convert Error",
			input:    LogLevelError,
			expected: ErrorLevel,
		},
		{
			name:     "Convert Off",
			input:    LogLevelOff,
			expected: log.Level(math.MaxInt32), // Off is maximum level
		},
		{
			name:     "Convert Unknown (defaults to Info)",
			input:    LogLevel("unknown"),
			expected: InfoLevel,
		},
		{
			name:     "Convert Empty (defaults to Info)",
			input:    LogLevel(""),
			expected: InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertLogLevel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseLogLevelComprehensive(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected LogLevel
		hasError bool
	}{
		// Valid levels - case insensitive
		{name: "Trace uppercase", input: "TRACE", expected: LogLevelTrace, hasError: false},
		{name: "Trace lowercase", input: "trace", expected: LogLevelTrace, hasError: false},
		{name: "Trace mixed case", input: "Trace", expected: LogLevelTrace, hasError: false},
		{name: "Trace with spaces", input: " trace ", expected: LogLevelTrace, hasError: false},

		{name: "Debug uppercase", input: "DEBUG", expected: LogLevelDebug, hasError: false},
		{name: "Debug lowercase", input: "debug", expected: LogLevelDebug, hasError: false},
		{name: "Debug mixed case", input: "Debug", expected: LogLevelDebug, hasError: false},

		{name: "Info uppercase", input: "INFO", expected: LogLevelInfo, hasError: false},
		{name: "Info lowercase", input: "info", expected: LogLevelInfo, hasError: false},
		{name: "Info mixed case", input: "Info", expected: LogLevelInfo, hasError: false},

		{name: "Warning uppercase", input: "WARNING", expected: LogLevelWarning, hasError: false},
		{name: "Warning lowercase", input: "warning", expected: LogLevelWarning, hasError: false},
		{name: "Warning mixed case", input: "Warning", expected: LogLevelWarning, hasError: false},

		{name: "Warn uppercase", input: "WARN", expected: LogLevelWarning, hasError: false},
		{name: "Warn lowercase", input: "warn", expected: LogLevelWarning, hasError: false},
		{name: "Warn mixed case", input: "Warn", expected: LogLevelWarning, hasError: false},

		{name: "Error uppercase", input: "ERROR", expected: LogLevelError, hasError: false},
		{name: "Error lowercase", input: "error", expected: LogLevelError, hasError: false},
		{name: "Error mixed case", input: "Error", expected: LogLevelError, hasError: false},

		{name: "Off uppercase", input: "OFF", expected: LogLevelOff, hasError: false},
		{name: "Off lowercase", input: "off", expected: LogLevelOff, hasError: false},
		{name: "Off mixed case", input: "Off", expected: LogLevelOff, hasError: false},

		// Empty string defaults to Info
		{name: "Empty string", input: "", expected: LogLevelInfo, hasError: false},
		{name: "Whitespace only", input: "   ", expected: LogLevelInfo, hasError: false},

		// Invalid levels
		{name: "Invalid level", input: "invalid", expected: "", hasError: true},
		{name: "Number", input: "123", expected: "", hasError: true},
		{name: "Special chars", input: "!@#$", expected: "", hasError: true},
		{name: "Fatal (not supported)", input: "fatal", expected: "", hasError: true},
		{name: "Verbose (not supported)", input: "verbose", expected: "", hasError: true},
		{name: "Silent (not supported)", input: "silent", expected: "", hasError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, err := ParseLogLevel(tt.input)
			if tt.hasError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidLogLevel)
				assert.Contains(t, err.Error(), "invalid log level")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, level)
			}
		})
	}
}

func TestLogLevelConstants(t *testing.T) {
	// Test that our constants are properly defined
	assert.Equal(t, LogLevel("Trace"), LogLevelTrace)
	assert.Equal(t, LogLevel("Debug"), LogLevelDebug)
	assert.Equal(t, LogLevel("Info"), LogLevelInfo)
	assert.Equal(t, LogLevel("Warning"), LogLevelWarning)
	assert.Equal(t, LogLevel("Error"), LogLevelError)
	assert.Equal(t, LogLevel("Off"), LogLevelOff)
}

func TestCharmLogLevelConstants(t *testing.T) {
	// Test that our charm/log level constants are properly defined
	// TraceLevel is a custom constant we define as DebugLevel - 1
	assert.Equal(t, log.DebugLevel-1, TraceLevel)
	assert.Equal(t, log.DebugLevel, DebugLevel)
	assert.Equal(t, log.InfoLevel, InfoLevel)
	assert.Equal(t, log.WarnLevel, WarnLevel)
	assert.Equal(t, log.ErrorLevel, ErrorLevel)
	assert.Equal(t, log.FatalLevel, FatalLevel)
}

func TestLogLevelConversion(t *testing.T) {
	// Test the complete flow: parse -> convert -> use
	testCases := []struct {
		input         string
		expectedParse LogLevel
		expectedLevel log.Level
	}{
		{"trace", LogLevelTrace, TraceLevel},
		{"debug", LogLevelDebug, DebugLevel},
		{"info", LogLevelInfo, InfoLevel},
		{"warn", LogLevelWarning, WarnLevel},
		{"warning", LogLevelWarning, WarnLevel},
		{"error", LogLevelError, ErrorLevel},
		{"off", LogLevelOff, log.Level(math.MaxInt32)},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			// Parse the string
			parsed, err := ParseLogLevel(tc.input)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedParse, parsed)

			// Convert to charm/log level
			converted := ConvertLogLevel(parsed)
			assert.Equal(t, tc.expectedLevel, converted)
		})
	}
}

func BenchmarkParseLogLevel(b *testing.B) {
	levels := []string{"trace", "debug", "info", "warning", "error", "off"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		level := levels[i%len(levels)]
		_, _ = ParseLogLevel(level)
	}
}

func BenchmarkConvertLogLevel(b *testing.B) {
	levels := []LogLevel{
		LogLevelTrace,
		LogLevelDebug,
		LogLevelInfo,
		LogLevelWarning,
		LogLevelError,
		LogLevelOff,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		level := levels[i%len(levels)]
		_ = ConvertLogLevel(level)
	}
}
