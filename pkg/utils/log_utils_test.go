package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOsExit(t *testing.T) {
	// Store the original OsExit
	originalOsExit := OsExit
	defer func() {
		// Restore the original OsExit after the test
		OsExit = originalOsExit
	}()

	exitCalled := false
	exitCode := 0

	// Mock OsExit
	OsExit = func(code int) {
		exitCalled = true
		exitCode = code
	}

	// Test the exit function
	OsExit(1)

	assert.True(t, exitCalled, "OsExit was not called")
	assert.Equal(t, 1, exitCode, "Unexpected exit code")
}

func TestLogLevelConstants(t *testing.T) {
	assert.Equal(t, "Trace", LogLevelTrace)
	assert.Equal(t, "Debug", LogLevelDebug)
}
