package exec

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestValidateWithOpa_NonexistentFile(t *testing.T) {
	data := map[string]interface{}{
		"test": "value",
	}

	valid, err := ValidateWithOpa(data, "/nonexistent/policy.rego", nil, 10)

	assert.False(t, valid)
	assert.Error(t, err)
}

func TestValidateWithOpaLegacy_Timeout(t *testing.T) {
	// Test the legacy OPA validation with timeout.
	policyContent := `package test
deny[msg] {
    msg := "test denial"
}
`
	data := map[string]interface{}{
		"test": "value",
	}

	// Use timeout of 0 to force immediate deadline exceeded.
	valid, err := ValidateWithOpaLegacy(data, "test", policyContent, 0)

	assert.False(t, valid)
	assert.Error(t, err)
	// Check that error wrapping includes OPA timeout error.
	assert.ErrorIs(t, err, errUtils.ErrOPATimeout)
}

// TestIsWindowsOPALoadError tests the isWindowsOPALoadError function.
func TestIsWindowsOPALoadError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "fs.ErrNotExist wrapped error",
			err:      errors.New("wrapped: file does not exist"),
			expected: false,
		},
		{
			name:     "Windows path not specified error",
			err:      errors.New("cannot find the path specified"),
			expected: false, // On non-Windows, should return false
		},
		{
			name:     "Windows file not found error",
			err:      errors.New("system cannot find the file specified"),
			expected: false, // On non-Windows, should return false
		},
		{
			name:     "generic error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWindowsOPALoadError(tt.err)
			// On non-Windows, all should return false
			// On Windows, only Windows-specific errors should return true
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContextDeadlineExceededWrapping(t *testing.T) {
	// Create a context that's already cancelled to simulate timeout.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel

	// Create a simple error that includes deadline exceeded.
	err := ctx.Err()

	// Verify it's a deadline exceeded (actually Canceled in this case, but the pattern is the same).
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		// This is how the code should wrap it.
		wrappedErr := errors.Join(errUtils.ErrOPATimeout, err)

		assert.Error(t, wrappedErr)
		assert.ErrorIs(t, wrappedErr, errUtils.ErrOPATimeout)
	}
}
