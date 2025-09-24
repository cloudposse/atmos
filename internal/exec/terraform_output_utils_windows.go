//go:build windows
// +build windows

package exec

import (
	"runtime"
	"time"
)

// windowsFileDelay adds a small delay on Windows to allow file handles to be released.
// This helps prevent "The process cannot access the file because another process has locked
// a portion of the file" errors when multiple terraform operations run in quick succession.
func windowsFileDelay() {
	if runtime.GOOS == "windows" {
		time.Sleep(200 * time.Millisecond)
	}
}

// retryOnWindows wraps a function with retry logic for Windows file operations.
// It will retry up to 3 times with increasing delays.
func retryOnWindows(fn func() error) error {
	if runtime.GOOS != "windows" {
		return fn()
	}

	var lastErr error
	delays := []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, 1000 * time.Millisecond}

	for i := 0; i < len(delays); i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
			if i < len(delays)-1 {
				time.Sleep(delays[i])
			}
		}
	}

	return lastErr
}
