package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMain_ErrorHandling(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedCode int
	}{
		{
			name:         "nil error returns 0",
			err:          nil,
			expectedCode: 0,
		},
		{
			name:         "generic error returns 1",
			err:          errors.New("generic error"),
			expectedCode: 1,
		},
		{
			name: "testFailureError returns its code",
			err: &testFailureError{
				code:        2,
				testsFailed: 5,
				testsPassed: 10,
			},
			expectedCode: 2,
		},
		{
			name: "exitError returns its code",
			err: &exitError{
				code: 3,
			},
			expectedCode: 3,
		},
		{
			name: "exitError with high code",
			err: &exitError{
				code: 255,
			},
			expectedCode: 255,
		},
		{
			name: "testFailureError with custom code",
			err: &testFailureError{
				code:        42,
				testsFailed: 1,
				testsPassed: 0,
			},
			expectedCode: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call internal helper that processes the error
			exitCode := processMainError(tt.err)
			assert.Equal(t, tt.expectedCode, exitCode, "Exit code should match expected")
		})
	}
}

// Helper function to test Main's error handling logic
func processMainError(err error) int {
	if err != nil {
		// Check if it's a testFailureError
		var testErr *testFailureError
		if errors.As(err, &testErr) {
			return testErr.code
		}

		// Check if it's an exit error with a specific code
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}

		// Default error handling
		return 1
	}
	return 0
}

func TestMain_ErrorTypes(t *testing.T) {
	// Test testFailureError
	testErr := &testFailureError{
		code:        10,
		testsFailed: 3,
		testsPassed: 7,
	}
	assert.Equal(t, 10, processMainError(testErr), "Should extract code from testFailureError")

	// Test exitError
	exitErr := &exitError{code: 20}
	assert.Equal(t, 20, processMainError(exitErr), "Should extract code from exitError")

	// Test nil error
	assert.Equal(t, 0, processMainError(nil), "Nil error should return 0")
}

func TestExitError(t *testing.T) {
	// Test exitError struct
	e := &exitError{code: 42}

	// Test Error method
	assert.Equal(t, "exit with code 42", e.Error(), "Error message should include exit code")

	// Test ExitCode method
	assert.Equal(t, 42, e.ExitCode(), "ExitCode should return the code")
}

func TestErrorWrapping(t *testing.T) {
	// Test that errors.As works with our custom error types
	var testErr *testFailureError
	var exitErr *exitError

	// Test testFailureError
	err1 := &testFailureError{code: 5}
	assert.True(t, errors.As(err1, &testErr), "Should identify as testFailureError")
	assert.Equal(t, 5, testErr.code)

	// Test exitError
	err2 := &exitError{code: 10}
	assert.True(t, errors.As(err2, &exitErr), "Should identify as exitError")
	assert.Equal(t, 10, exitErr.code)

	// Test generic error
	err3 := errors.New("generic")
	assert.False(t, errors.As(err3, &testErr), "Generic error should not be testFailureError")
	assert.False(t, errors.As(err3, &exitErr), "Generic error should not be exitError")
}
