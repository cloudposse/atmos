//go:build windows
// +build windows

package exec

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWindowsFileDelay(t *testing.T) {
	// Test that windowsFileDelay introduces a delay on Windows.
	start := time.Now()
	windowsFileDelay()
	elapsed := time.Since(start)

	// Should have at least 200ms delay on Windows.
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(180), "Expected at least 180ms delay on Windows")
	assert.LessOrEqual(t, elapsed.Milliseconds(), int64(250), "Expected no more than 250ms delay on Windows")
}

func TestRetryOnWindows_Success(t *testing.T) {
	// Test successful execution on first try.
	callCount := 0
	err := retryOnWindows(func() error {
		callCount++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount, "Function should be called exactly once on success")
}

func TestRetryOnWindows_RetryThenSuccess(t *testing.T) {
	// Test retry logic - fail twice, then succeed.
	callCount := 0
	testErr := errors.New("temporary error")

	start := time.Now()
	err := retryOnWindows(func() error {
		callCount++
		if callCount < 3 {
			return testErr
		}
		return nil
	})
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Equal(t, 3, callCount, "Function should be called 3 times (2 failures + 1 success)")
	// Should have delays: 200ms after first failure, 500ms after second failure.
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(650), "Expected at least 650ms total delay for 2 retries")
}

func TestRetryOnWindows_AllFailures(t *testing.T) {
	// Test when all retries fail.
	callCount := 0
	testErr := errors.New("persistent error")

	start := time.Now()
	err := retryOnWindows(func() error {
		callCount++
		return testErr
	})
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.Equal(t, testErr, err, "Should return the last error")
	assert.Equal(t, 3, callCount, "Function should be called 3 times (max retries)")
	// Should have delays: 200ms + 500ms = 700ms minimum.
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(650), "Expected at least 650ms total delay for all retries")
}

func TestRetryOnWindows_DifferentErrors(t *testing.T) {
	// Test that the last error is returned when different errors occur.
	callCount := 0
	errList := []error{
		errors.New("first error"),
		errors.New("second error"),
		errors.New("final error"),
	}

	err := retryOnWindows(func() error {
		err := errList[callCount]
		callCount++
		return err
	})

	assert.Error(t, err)
	assert.Equal(t, errList[2], err, "Should return the last error")
	assert.Equal(t, 3, callCount, "Function should be called 3 times")
}
