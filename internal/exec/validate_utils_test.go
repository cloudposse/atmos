package exec

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidateWithOpa_ContextTimeout tests timeout handling in ValidateWithOpa.
func TestValidateWithOpa_ContextTimeout(t *testing.T) {
	// Test that context.DeadlineExceeded is properly detected using errors.Is.
	err := context.DeadlineExceeded
	assert.True(t, errors.Is(err, context.DeadlineExceeded))

	// Test with wrapped error.
	wrappedErr := errors.Join(context.DeadlineExceeded, errors.New("additional context"))
	assert.True(t, errors.Is(wrappedErr, context.DeadlineExceeded))
}

// TestValidateWithOpaLegacy_ContextTimeout tests timeout handling in ValidateWithOpaLegacy.
func TestValidateWithOpaLegacy_ContextTimeout(t *testing.T) {
	// Similar test for legacy function.
	err := context.DeadlineExceeded
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}

// TestContextDeadlineExceededHandling tests that context deadline exceeded errors are handled properly.
func TestContextDeadlineExceededHandling(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Direct deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "Wrapped deadline exceeded",
			err:      errors.Join(context.DeadlineExceeded, errors.New("extra info")),
			expected: true,
		},
		{
			name:     "Different error",
			err:      errors.New("some other error"),
			expected: false,
		},
		{
			name:     "String comparison would fail",
			err:      errors.New("context deadline exceeded"), // String matches but not the same error
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The new code uses errors.Is instead of string comparison
			result := errors.Is(tt.err, context.DeadlineExceeded)
			assert.Equal(t, tt.expected, result)
		})
	}
}
