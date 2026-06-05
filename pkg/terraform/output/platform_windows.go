//go:build windows

package output

import (
	"runtime"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// windowsFileDelay adds a delay on Windows to prevent file locking issues.
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

// RetryOnWindows is the exported version for use by other packages.
// It wraps a function with retry logic for Windows file operations.
func RetryOnWindows(fn func() error) error {
	defer perf.Track(nil, "output.RetryOnWindows")()

	return retryOnWindows(fn)
}
