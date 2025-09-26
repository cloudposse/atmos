package logger

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtmosLogger_Trace(t *testing.T) {
	var buf bytes.Buffer
	logger := New()
	logger.SetOutput(&buf)
	logger.SetLevel(TraceLevel)

	logger.Trace("test trace message")
	output := buf.String()

	// The trace message should be in the output
	assert.Contains(t, output, "test trace message")
	// Note: The TRCE prefix requires styles to be set up, which is done in cmd/root.go
}

func TestAtmosLogger_GetLevelString(t *testing.T) {
	logger := New()

	logger.SetLevel(TraceLevel)
	assert.Equal(t, "trace", logger.GetLevelString())

	logger.SetLevel(DebugLevel)
	assert.Equal(t, "debug", strings.ToLower(logger.GetLevelString()))

	logger.SetLevel(InfoLevel)
	assert.Equal(t, "info", strings.ToLower(logger.GetLevelString()))
}

func TestPackageLevelFunctions(t *testing.T) {
	// Save and restore default logger
	oldLogger := Default()
	defer SetDefault(oldLogger)

	var buf bytes.Buffer
	testLogger := New()
	testLogger.SetOutput(&buf)
	testLogger.SetLevel(TraceLevel)
	SetDefault(testLogger)

	// Test trace function
	Trace("package level trace")
	assert.Contains(t, buf.String(), "package level trace")
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
		hasError bool
	}{
		{"Trace", LogLevelTrace, false},
		{"Debug", LogLevelDebug, false},
		{"Info", LogLevelInfo, false},
		{"Warning", LogLevelWarning, false},
		{"Off", LogLevelOff, false},
		{"", LogLevelInfo, false}, // Default to Info
		{"Invalid", "", true},
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
