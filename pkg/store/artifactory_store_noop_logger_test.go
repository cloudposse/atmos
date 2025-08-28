package store

import (
	"bytes"
	"io"
	"testing"

	al "github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
)

// Test each method individually to ensure coverage.
func TestNoopLoggerMethods(t *testing.T) {
	logger := &noopLogger{}

	t.Run("GetLogLevel", func(t *testing.T) {
		level := logger.GetLogLevel()
		assert.Equal(t, al.ERROR, level)
	})

	t.Run("SetLogLevel", func(t *testing.T) {
		// Store original level
		originalLevel := logger.GetLogLevel()

		// Try to change it
		logger.SetLogLevel(al.DEBUG)

		// Verify it didn't change
		newLevel := logger.GetLogLevel()
		assert.Equal(t, originalLevel, newLevel)
	})

	t.Run("GetOutputWriter", func(t *testing.T) {
		writer := logger.GetOutputWriter()
		assert.Equal(t, io.Discard, writer)
	})

	t.Run("SetOutputWriter", func(t *testing.T) {
		buffer := &bytes.Buffer{}
		logger.SetOutputWriter(buffer)

		// Verify it still returns io.Discard
		assert.Equal(t, io.Discard, logger.GetOutputWriter())
	})

	t.Run("SetLogsWriter", func(t *testing.T) {
		buffer := &bytes.Buffer{}
		logger.SetLogsWriter(buffer)
		// No way to verify, but we've executed the method for coverage
	})

	t.Run("Debug", func(t *testing.T) {
		logger.Debug("test debug message")
		// No way to verify, but we've executed the method for coverage
	})

	t.Run("Info", func(t *testing.T) {
		logger.Info("test info message")
		// No way to verify, but we've executed the method for coverage
	})

	t.Run("Warn", func(t *testing.T) {
		logger.Warn("test warn message")
		// No way to verify, but we've executed the method for coverage
	})

	t.Run("Error", func(t *testing.T) {
		logger.Error("test error message")
		// No way to verify, but we've executed the method for coverage
	})

	t.Run("Output", func(t *testing.T) {
		logger.Output("test output message")
		// No way to verify, but we've executed the method for coverage
	})
}

func TestCreateNoopLogger(t *testing.T) {
	// Test the factory function
	logger := createNoopLogger()
	assert.NotNil(t, logger, "createNoopLogger should return a non-nil logger")
	assert.IsType(t, &noopLogger{}, logger, "createNoopLogger should return a noopLogger instance")

	// Verify the returned logger behaves as expected
	assert.Equal(t, al.ERROR, logger.GetLogLevel(), "Returned logger should have ERROR level")
}
