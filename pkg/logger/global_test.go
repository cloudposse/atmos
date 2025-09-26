package logger

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGlobalLoggerConcurrency tests that the global logger is safe for concurrent access.
func TestGlobalLoggerConcurrency(t *testing.T) {
	// Save the original default logger
	originalLogger := Default()
	defer SetDefault(originalLogger)

	// Create multiple test loggers
	var buffers [10]bytes.Buffer
	var loggers [10]*AtmosLogger

	for i := range loggers {
		logger := New()
		logger.SetOutput(&buffers[i])
		logger.SetLevel(InfoLevel)
		loggers[i] = logger
	}

	// Use a WaitGroup to coordinate goroutines
	var wg sync.WaitGroup
	const numGoroutines = 100
	const numOperations = 100
	var failureCount atomic.Int32

	// Start goroutines that read from the default logger
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				logger := Default()
				if logger == nil {
					failureCount.Add(1)
					return
				}
				// Try to use the logger
				logger.GetLevel()
				logger.GetLevelString()
			}
		}()
	}

	// Start goroutines that write to the default logger
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				// Set a random logger as default
				SetDefault(loggers[id%len(loggers)])
			}
		}(i)
	}

	// Start goroutines that log messages
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				// These use the global Default() function
				Info("test message")
				Debug("debug message")
				Trace("trace message")
			}
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Check if any goroutine reported a failure
	require.Equal(t, int32(0), failureCount.Load(), "Some goroutines found nil logger")

	// Verify we can still get and set the default logger
	testLogger := New()
	SetDefault(testLogger)
	assert.Equal(t, testLogger, Default())
}

// TestSetDefaultNil tests that SetDefault ignores nil values.
func TestSetDefaultNil(t *testing.T) {
	// Save the original default logger
	originalLogger := Default()
	defer SetDefault(originalLogger)

	// Create a test logger
	testLogger := New()
	SetDefault(testLogger)
	assert.Equal(t, testLogger, Default())

	// Try to set nil - should be ignored
	SetDefault(nil)
	assert.Equal(t, testLogger, Default())
}

// TestDefaultLoggerInitialization tests that the default logger is properly initialized.
func TestDefaultLoggerInitialization(t *testing.T) {
	// The default logger should be non-nil
	logger := Default()
	assert.NotNil(t, logger)
	assert.NotNil(t, logger.charm)
}

// TestNewLogger tests the New function.
func TestNewLogger(t *testing.T) {
	logger := New()
	assert.NotNil(t, logger)
	assert.NotNil(t, logger.charm)

	// Test that each call to New returns a different instance
	logger2 := New()
	assert.NotNil(t, logger2)
	assert.NotSame(t, logger, logger2)
}

// BenchmarkConcurrentDefaultAccess benchmarks concurrent access to the default logger.
func BenchmarkConcurrentDefaultAccess(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger := Default()
			_ = logger.GetLevel()
		}
	})
}

// BenchmarkConcurrentSetDefault benchmarks concurrent SetDefault calls.
func BenchmarkConcurrentSetDefault(b *testing.B) {
	logger := New()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			SetDefault(logger)
		}
	})
}

// BenchmarkConcurrentMixedOperations benchmarks mixed concurrent operations.
func BenchmarkConcurrentMixedOperations(b *testing.B) {
	loggers := make([]*AtmosLogger, 10)
	for i := range loggers {
		loggers[i] = New()
	}

	var counter int
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%3 == 0 {
				SetDefault(loggers[i%len(loggers)])
			} else {
				logger := Default()
				_ = logger.GetLevel()
			}
			i++
		}
	})
	_ = counter
}
