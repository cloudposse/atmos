package logger

import (
	"bytes"
	"os"
	"strings"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
)

func TestTraceLevel_RelativeToDebug(t *testing.T) {
	// Verify trace is exactly one level more verbose than debug.
	assert.Equal(t, log.DebugLevel-1, TraceLevel)

	// Verify ordering.
	assert.Less(t, int(TraceLevel), int(log.DebugLevel),
		"Trace level should be more verbose (lower value) than Debug")
}

func TestLogLevelHierarchy(t *testing.T) {
	// Test complete hierarchy with trace.
	tests := []struct {
		name   string
		level1 log.Level
		level2 log.Level
	}{
		{"Trace < Debug", TraceLevel, log.DebugLevel},
		{"Debug < Info", log.DebugLevel, log.InfoLevel},
		{"Info < Warn", log.InfoLevel, log.WarnLevel},
		{"Warn < Error", log.WarnLevel, log.ErrorLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Less(t, int(tt.level1), int(tt.level2),
				"%s: %d should be less than %d", tt.name, tt.level1, tt.level2)
		})
	}
}

func TestTrace(t *testing.T) {
	// Save original state.
	originalLevel := log.GetLevel()
	defer func() {
		log.SetLevel(originalLevel)
		log.SetOutput(os.Stderr) // Reset to default
	}()

	var buf bytes.Buffer
	log.SetOutput(&buf)

	t.Run("trace visible at trace level", func(t *testing.T) {
		log.SetLevel(TraceLevel)
		buf.Reset()

		Trace("test message", "key", "value")
		output := buf.String()

		assert.Contains(t, output, "test message")
		assert.Contains(t, output, "key")
		assert.Contains(t, output, "value")
	})

	t.Run("trace hidden at debug level", func(t *testing.T) {
		log.SetLevel(log.DebugLevel)
		buf.Reset()

		Trace("should not appear")
		output := buf.String()

		assert.Empty(t, output, "Trace should not be visible at debug level")
	})

	t.Run("trace hidden at info level", func(t *testing.T) {
		log.SetLevel(log.InfoLevel)
		buf.Reset()

		Trace("should not appear")
		output := buf.String()

		assert.Empty(t, output, "Trace should not be visible at info level")
	})
}

func TestTracef(t *testing.T) {
	// Save original state.
	originalLevel := log.GetLevel()
	defer func() {
		log.SetLevel(originalLevel)
		log.SetOutput(os.Stderr) // Reset to default
	}()

	var buf bytes.Buffer
	log.SetOutput(&buf)

	t.Run("formatted message at trace level", func(t *testing.T) {
		log.SetLevel(TraceLevel)
		buf.Reset()

		Tracef("formatted %s with %d items", "message", 42)
		output := buf.String()

		assert.Contains(t, output, "formatted message with 42 items")
	})

	t.Run("formatted message hidden at debug level", func(t *testing.T) {
		log.SetLevel(log.DebugLevel)
		buf.Reset()

		Tracef("should not appear %s", "test")
		output := buf.String()

		assert.Empty(t, output, "Tracef should not be visible at debug level")
	})
}

func TestTraceLevelFiltering(t *testing.T) {
	// Save original state.
	originalLevel := log.GetLevel()
	defer func() {
		log.SetLevel(originalLevel)
		log.SetOutput(os.Stderr) // Reset to default
	}()

	tests := []struct {
		name         string
		setLevel     log.Level
		traceVisible bool
		debugVisible bool
		infoVisible  bool
	}{
		{
			name:         "Trace level shows all",
			setLevel:     TraceLevel,
			traceVisible: true,
			debugVisible: true,
			infoVisible:  true,
		},
		{
			name:         "Debug level hides trace",
			setLevel:     log.DebugLevel,
			traceVisible: false,
			debugVisible: true,
			infoVisible:  true,
		},
		{
			name:         "Info level hides trace and debug",
			setLevel:     log.InfoLevel,
			traceVisible: false,
			debugVisible: false,
			infoVisible:  true,
		},
		{
			name:         "Warn level hides trace, debug, and info",
			setLevel:     log.WarnLevel,
			traceVisible: false,
			debugVisible: false,
			infoVisible:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log.SetOutput(&buf)
			log.SetLevel(tt.setLevel)

			// Test trace visibility.
			buf.Reset()
			Trace("trace message")
			hasTrace := strings.Contains(buf.String(), "trace message")
			assert.Equal(t, tt.traceVisible, hasTrace,
				"Trace visibility incorrect at %v level", tt.setLevel)

			// Test debug visibility.
			buf.Reset()
			log.Debug("debug message")
			hasDebug := strings.Contains(buf.String(), "debug message")
			assert.Equal(t, tt.debugVisible, hasDebug,
				"Debug visibility incorrect at %v level", tt.setLevel)

			// Test info visibility.
			buf.Reset()
			log.Info("info message")
			hasInfo := strings.Contains(buf.String(), "info message")
			assert.Equal(t, tt.infoVisible, hasInfo,
				"Info visibility incorrect at %v level", tt.setLevel)
		})
	}
}

func TestTraceWithKeyValues(t *testing.T) {
	// Save original state.
	originalLevel := log.GetLevel()
	defer func() {
		log.SetLevel(originalLevel)
		log.SetOutput(os.Stderr) // Reset to default
	}()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(TraceLevel)

	// Test with various key-value pairs.
	buf.Reset()
	Trace("operation", "component", "vpc", "status", "success", "count", 42)
	output := buf.String()

	assert.Contains(t, output, "operation")
	assert.Contains(t, output, "component")
	assert.Contains(t, output, "vpc")
	assert.Contains(t, output, "status")
	assert.Contains(t, output, "success")
	assert.Contains(t, output, "count")
	assert.Contains(t, output, "42")
}

func TestTraceLevelComparison(t *testing.T) {
	// Test that we can compare trace level properly.
	tests := []struct {
		name     string
		level    log.Level
		expected bool
	}{
		{"Trace level equals TraceLevel", TraceLevel, true},
		{"Debug level not equals TraceLevel", log.DebugLevel, false},
		{"Info level not equals TraceLevel", log.InfoLevel, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.level == TraceLevel
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTraceLevelIsLowest(t *testing.T) {
	// Verify that trace is the lowest (most verbose) standard level.
	levels := []log.Level{
		TraceLevel,
		log.DebugLevel,
		log.InfoLevel,
		log.WarnLevel,
		log.ErrorLevel,
	}

	// Find the minimum level.
	minLevel := levels[0]
	for _, level := range levels {
		if int(level) < int(minLevel) {
			minLevel = level
		}
	}

	assert.Equal(t, TraceLevel, minLevel,
		"Trace should be the most verbose (lowest value) level")
}
