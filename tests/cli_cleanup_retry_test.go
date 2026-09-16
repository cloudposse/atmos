package tests

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	cleanupRemoveAttempts = 10
	// Win32 ERROR_SHARING_VIOLATION and ERROR_LOCK_VIOLATION. Numeric values
	// allow deterministic regression tests on non-Windows hosts too.
	cleanupSharingViolation = syscall.Errno(32)
	cleanupLockViolation    = syscall.Errno(33)
)

// removeTestOutput tolerates short-lived Windows sharing/lock violations when
// removing generated files between CLI tests. It never suppresses a permanent
// error: the next test must not run against a partially cleaned fixture.
func removeTestOutput(path string) error {
	return retryRemoveTestOutput(path, runtime.GOOS, os.RemoveAll, time.Sleep)
}

func retryRemoveTestOutput(path, goos string, remove func(string) error, sleep func(time.Duration)) error {
	delay := 200 * time.Millisecond
	const maxDelay = 2 * time.Second
	for attempt := 1; ; attempt++ {
		err := remove(path)
		if err == nil || attempt >= cleanupRemoveAttempts || goos != "windows" {
			return err
		}
		if !errors.Is(err, cleanupSharingViolation) && !errors.Is(err, cleanupLockViolation) {
			return err
		}
		sleep(delay)
		if delay *= 2; delay > maxDelay {
			delay = maxDelay
		}
	}
}

func TestRemoveTestOutputRetry(t *testing.T) {
	t.Parallel()

	sharing := &os.PathError{Op: "unlinkat", Path: "plan.tfplan.tar", Err: syscall.Errno(32)}
	locked := &os.PathError{Op: "unlinkat", Path: "plan.tfplan.tar", Err: syscall.Errno(33)}
	tests := []struct {
		name     string
		goos     string
		failures []error
		wantErr  error
		calls    int
	}{
		{name: "success", goos: "windows", calls: 1},
		{name: "sharing violation clears", goos: "windows", failures: []error{sharing}, calls: 2},
		{name: "lock violation clears", goos: "windows", failures: []error{locked, locked}, calls: 3},
		{name: "persistent lock fails", goos: "windows", failures: []error{sharing}, wantErr: sharing, calls: cleanupRemoveAttempts},
		{name: "permission denied fails immediately", goos: "windows", failures: []error{os.ErrPermission}, wantErr: os.ErrPermission, calls: 1},
		{name: "other error after lock fails", goos: "windows", failures: []error{sharing, os.ErrInvalid}, wantErr: os.ErrInvalid, calls: 2},
		{name: "unix errno 32 is not a Windows lock", goos: "linux", failures: []error{sharing}, wantErr: sharing, calls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			var delays []time.Duration
			err := retryRemoveTestOutput("output", tt.goos, func(path string) error {
				assert.Equal(t, "output", path)
				calls++
				if calls <= len(tt.failures) {
					return tt.failures[calls-1]
				}
				return tt.wantErr
			}, func(delay time.Duration) { delays = append(delays, delay) })
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
			assert.Equal(t, tt.calls, calls)
			assert.Len(t, delays, tt.calls-1)
			for _, delay := range delays {
				assert.Positive(t, delay)
				assert.LessOrEqual(t, delay, 2*time.Second)
			}
		})
	}
}
