//go:build !windows
// +build !windows

package exec

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWindowsFileDelay_NoOp(t *testing.T) {
	// Test that windowsFileDelay is a no-op on non-Windows platforms.
	start := time.Now()
	windowsFileDelay()
	elapsed := time.Since(start)

	// Should have no significant delay on non-Windows platforms.
	assert.Less(t, elapsed.Milliseconds(), int64(10), "Expected no significant delay on non-Windows platforms")
}

func TestRetryOnWindows_NoRetry(t *testing.T) {
	// Test that retryOnWindows executes immediately without retries on non-Windows.
	callCount := 0
	testErr := errors.New("test error")

	// Test with error - should not retry on non-Windows.
	err := retryOnWindows(func() error {
		callCount++
		return testErr
	})

	assert.Error(t, err)
	assert.Equal(t, testErr, err)
	assert.Equal(t, 1, callCount, "Function should be called exactly once on non-Windows platforms")
}

func TestRetryOnWindows_SuccessNoRetry(t *testing.T) {
	// Test successful execution on non-Windows.
	callCount := 0

	err := retryOnWindows(func() error {
		callCount++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount, "Function should be called exactly once on non-Windows platforms")
}

func TestRetryOnWindows_NoDelay(t *testing.T) {
	// Test that there's no delay even on error for non-Windows platforms.
	testErr := errors.New("test error")

	start := time.Now()
	err := retryOnWindows(func() error {
		return testErr
	})
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.Less(t, elapsed.Milliseconds(), int64(10), "Expected no delay on non-Windows platforms even with errors")
}
