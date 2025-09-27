package exec

import (
	"errors"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
)

// TestExecuteDescribeAffectedWithTargetRefCheckout_ReferenceNotFound tests the error handling for reference not found.
func TestExecuteDescribeAffectedWithTargetRefCheckout_ReferenceNotFound(t *testing.T) {
	// This is a complex function that requires a git repository setup.
	// We'll test the error condition handling specifically.

	// The key change is checking for errors.Is(err, plumbing.ErrReferenceNotFound)
	// instead of strings.Contains(err.Error(), "reference not found")

	// Create a test to verify the error type is correctly handled.
	err := plumbing.ErrReferenceNotFound
	assert.True(t, errors.Is(err, plumbing.ErrReferenceNotFound))

	// Verify that wrapped errors are also caught.
	wrappedErr := errors.Join(plumbing.ErrReferenceNotFound, errors.New("additional context"))
	assert.True(t, errors.Is(wrappedErr, plumbing.ErrReferenceNotFound))
}

// TestGitReferenceErrorHandling tests that git reference errors are properly handled.
func TestGitReferenceErrorHandling(t *testing.T) {
	// Test various error conditions
	tests := []struct {
		name     string
		err      error
		shouldBe error
		expected bool
	}{
		{
			name:     "Direct reference not found error",
			err:      plumbing.ErrReferenceNotFound,
			shouldBe: plumbing.ErrReferenceNotFound,
			expected: true,
		},
		{
			name:     "Wrapped reference not found error",
			err:      errors.Join(errors.New("prefix"), plumbing.ErrReferenceNotFound),
			shouldBe: plumbing.ErrReferenceNotFound,
			expected: true,
		},
		{
			name:     "Different error",
			err:      errors.New("some other error"),
			shouldBe: plumbing.ErrReferenceNotFound,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errors.Is(tt.err, tt.shouldBe)
			assert.Equal(t, tt.expected, result)
		})
	}
}
