package logger

import (
	"os"
	"sync"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
)

func TestGetLogger(t *testing.T) {
	// Reset the global state for testing
	resetGlobalLogger()

	// First call should create a new logger
	logger1 := GetLogger()
	assert.NotNil(t, logger1)

	// Second call should return the same logger (singleton pattern)
	logger2 := GetLogger()
	assert.Same(t, logger1, logger2)

	// Verify it's the same instance
	assert.Equal(t, logger1, logger2)
}

func TestSetLogger(t *testing.T) {
	// Reset the global state for testing
	resetGlobalLogger()

	// Create a custom logger
	customLogger := log.New(os.Stderr)
	customLogger.SetLevel(log.DebugLevel)

	// Set the custom logger
	SetLogger(customLogger)

	// GetLogger should now return our custom logger
	retrievedLogger := GetLogger()
	assert.Same(t, customLogger, retrievedLogger)
	assert.Equal(t, log.DebugLevel, retrievedLogger.GetLevel())
}

func TestNewStyledLogger(t *testing.T) {
	// Create a new styled logger
	logger := NewStyledLogger()

	assert.NotNil(t, logger)

	// Test that the logger has styles configured
	// The logger should have the default styles we set
	// Note: We can't directly test the styles, but we can verify the logger is created

	// Test that we can use the logger without panicking
	assert.NotPanics(t, func() {
		// These won't actually output in tests, but verify they don't panic
		logger.Debug("test debug message")
		logger.Info("test info message")
		logger.Warn("test warn message")
		logger.Error("test error message")
	})
}

func TestLoggerStyles(t *testing.T) {
	// Test that NewStyledLogger creates a logger with proper styles
	logger := NewStyledLogger()

	// Verify logger is not nil
	assert.NotNil(t, logger)

	// Test that all log levels can be used
	tests := []struct {
		name    string
		logFunc func(msg interface{}, keyvals ...interface{})
		message string
	}{
		{
			name:    "debug level",
			logFunc: logger.Debug,
			message: "debug test",
		},
		{
			name:    "info level",
			logFunc: logger.Info,
			message: "info test",
		},
		{
			name:    "warn level",
			logFunc: logger.Warn,
			message: "warn test",
		},
		{
			name:    "error level",
			logFunc: logger.Error,
			message: "error test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				tt.logFunc(tt.message)
			})
		})
	}
}

func TestLoggerWithKeyValues(t *testing.T) {
	logger := NewStyledLogger()

	// Test logging with key-value pairs
	assert.NotPanics(t, func() {
		logger.Info("test message",
			"key1", "value1",
			"key2", 123,
			"key3", true,
		)
	})

	// Test with various data types
	assert.NotPanics(t, func() {
		logger.Debug("complex values",
			"string", "text",
			"int", 42,
			"float", 3.14,
			"bool", false,
			"nil", nil,
		)
	})
}

func TestConcurrentLoggerAccess(t *testing.T) {
	// Reset the global state for testing
	resetGlobalLogger()

	// Test concurrent access to GetLogger
	var wg sync.WaitGroup
	loggers := make([]*log.Logger, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			loggers[idx] = GetLogger()
		}(i)
	}

	wg.Wait()

	// All loggers should be the same instance
	firstLogger := loggers[0]
	for i := 1; i < 10; i++ {
		assert.Same(t, firstLogger, loggers[i], "Logger instance %d should be the same", i)
	}
}

func TestSetLoggerBeforeGet(t *testing.T) {
	// Reset the global state for testing
	resetGlobalLogger()

	// Set a custom logger before any GetLogger call
	customLogger := log.New(os.Stderr)
	customLogger.SetLevel(log.WarnLevel)
	SetLogger(customLogger)

	// First GetLogger should return the custom logger
	logger := GetLogger()
	assert.Same(t, customLogger, logger)
	assert.Equal(t, log.WarnLevel, logger.GetLevel())
}

func TestLoggerOutput(t *testing.T) {
	// Create a logger that writes to stderr (default)
	logger := NewStyledLogger()

	// Verify the logger is configured to write to stderr
	// Note: We can't directly test the output destination,
	// but we can verify the logger doesn't panic
	assert.NotPanics(t, func() {
		logger.SetLevel(log.FatalLevel) // Set to fatal to suppress test output
		logger.Info("This should not appear in test output")
	})
}

func TestLoggerLevelSetting(t *testing.T) {
	logger := NewStyledLogger()

	// Test setting different log levels
	levels := []log.Level{
		log.DebugLevel,
		log.InfoLevel,
		log.WarnLevel,
		log.ErrorLevel,
		log.FatalLevel,
	}

	for _, level := range levels {
		logger.SetLevel(level)
		assert.Equal(t, level, logger.GetLevel())
	}
}

func TestLoggerWithContext(t *testing.T) {
	logger := NewStyledLogger()

	// Test creating a logger with context
	contextLogger := logger.With("component", "test", "version", "1.0.0")
	assert.NotNil(t, contextLogger)

	// The context logger should work without panicking
	assert.NotPanics(t, func() {
		contextLogger.Info("message with context")
	})
}

func TestLoggerPrefix(t *testing.T) {
	logger := NewStyledLogger()

	// Test setting a prefix
	logger.SetPrefix("TEST")

	// Should not panic with prefix
	assert.NotPanics(t, func() {
		logger.Info("message with prefix")
	})
}

func TestMultipleSetLogger(t *testing.T) {
	// Reset the global state for testing
	resetGlobalLogger()

	// Create first logger
	logger1 := log.New(os.Stderr)
	logger1.SetLevel(log.DebugLevel)
	SetLogger(logger1)

	// Verify it's set
	assert.Same(t, logger1, GetLogger())

	// Create second logger
	logger2 := log.New(os.Stderr)
	logger2.SetLevel(log.InfoLevel)
	SetLogger(logger2)

	// Verify it's replaced
	assert.Same(t, logger2, GetLogger())
	assert.NotSame(t, logger1, GetLogger())
}

// Helper function to reset the global logger state for testing
func resetGlobalLogger() {
	globalLogger = nil
	once = sync.Once{}
}

// TestMain ensures clean test environment
func TestMain(m *testing.M) {
	// Reset before running tests
	resetGlobalLogger()

	// Run tests
	code := m.Run()

	// Reset after tests
	resetGlobalLogger()

	os.Exit(code)
}
