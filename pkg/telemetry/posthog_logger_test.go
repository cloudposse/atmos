package telemetry

import (
	"bytes"
	"io"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
)

// TestPosthogLogger_LogMethods tests that PosthogLogger methods work correctly.
func TestPosthogLogger_LogMethods(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(io.Discard) // Reset output after test
	log.SetLevel(log.DebugLevel)

	logger := NewPosthogLogger()

	// Test Debugf
	logger.Debugf("debug message: %s", "test")
	assert.Contains(t, buf.String(), "PostHog debug message")
	assert.Contains(t, buf.String(), "debug message: test")
	buf.Reset()

	// Test Logf (should use Debug level)
	logger.Logf("info message: %s", "test")
	assert.Contains(t, buf.String(), "PostHog info message")
	assert.Contains(t, buf.String(), "info message: test")
	buf.Reset()

	// Test Warnf
	logger.Warnf("warning message: %s", "test")
	assert.Contains(t, buf.String(), "PostHog warning")
	assert.Contains(t, buf.String(), "warning message: test")
	buf.Reset()

	// Test Errorf (should use Debug level to avoid polluting user output)
	logger.Errorf("error message: %s", "test")
	assert.Contains(t, buf.String(), "PostHog telemetry error")
	assert.Contains(t, buf.String(), "error message: test")
}

// TestPosthogLogger_ErrorsAtDebugLevel tests that errors are logged at debug level.
func TestPosthogLogger_ErrorsAtDebugLevel(t *testing.T) {
	// Save original log level
	originalLevel := log.GetLevel()
	defer func() {
		log.SetOutput(io.Discard)
		log.SetLevel(originalLevel)
	}()

	// Create a buffer to capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)

	logger := NewPosthogLogger()

	// Test that errors are not visible at INFO level
	log.SetLevel(log.InfoLevel)
	buf.Reset()
	logger.Errorf("502 Bad Gateway")
	// Should not appear in output when log level is INFO
	assert.Empty(t, buf.String())

	// Test that errors are visible at DEBUG level
	log.SetLevel(log.DebugLevel)
	buf.Reset()
	logger.Errorf("502 Bad Gateway")
	// Should appear in output when log level is DEBUG
	assert.Contains(t, buf.String(), "PostHog telemetry error")
	assert.Contains(t, buf.String(), "502 Bad Gateway")
}

// TestSilentLogger tests that SilentLogger discards all messages.
func TestSilentLogger(t *testing.T) {
	defer log.SetOutput(io.Discard) // Reset output after test

	// Create a buffer to capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.DebugLevel)

	logger := NewSilentLogger()

	// All methods should produce no output
	logger.Debugf("debug message")
	logger.Logf("info message")
	logger.Warnf("warning message")
	logger.Errorf("error message")

	// Buffer should remain empty
	assert.Empty(t, buf.String())
}

// TestDiscardLogger tests that DiscardLogger discards all messages.
func TestDiscardLogger(t *testing.T) {
	defer log.SetOutput(io.Discard) // Reset output after test

	// Create a buffer to capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.DebugLevel)

	logger := NewDiscardLogger()

	// Verify writer is set to io.Discard
	assert.Equal(t, io.Discard, logger.writer)

	// All methods should produce no output
	logger.Debugf("debug message")
	logger.Logf("info message")
	logger.Warnf("warning message")
	logger.Errorf("error message")

	// Buffer should remain empty
	assert.Empty(t, buf.String())
}

// TestPosthogLogger_PreventStderrPollution tests that PostHog errors don't leak to stderr.
func TestPosthogLogger_PreventStderrPollution(t *testing.T) {
	// This test ensures that PostHog error messages like
	// "posthog 2025/09/21 23:37:14 ERROR: 502 Bad Gateway"
	// are not printed directly to stderr but are handled by our logger

	// Save original log level
	originalLevel := log.GetLevel()
	defer func() {
		log.SetOutput(io.Discard)
		log.SetLevel(originalLevel)
	}()

	// Create a buffer to capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.InfoLevel) // Set to INFO level (production default)

	logger := NewPosthogLogger()

	// Simulate PostHog error messages
	logger.Errorf("response 502 502 Bad Gateway – <html><head><title>502 Bad Gateway</title></head><body><center><h1>502 Bad Gateway</h1></center></body></html>")
	logger.Errorf("1 messages dropped because they failed to be sent and the client was closed")

	// At INFO level, these errors should not appear
	assert.Empty(t, buf.String(), "PostHog errors should not appear at INFO log level")

	// Now test at DEBUG level
	log.SetLevel(log.DebugLevel)
	buf.Reset()

	logger.Errorf("response 502 502 Bad Gateway")
	logger.Errorf("1 messages dropped")

	// At DEBUG level, errors should appear with our prefix
	output := buf.String()
	assert.Contains(t, output, "PostHog telemetry error", "Errors should be prefixed at DEBUG level")
	assert.Contains(t, output, "502 Bad Gateway", "Error content should be present at DEBUG level")
	assert.Contains(t, output, "messages dropped", "Error content should be present at DEBUG level")
}