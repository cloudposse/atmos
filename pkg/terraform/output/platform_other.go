//go:build !windows

package output

import "github.com/cloudposse/atmos/pkg/perf"

// windowsFileDelay adds a delay on Windows to prevent file locking issues.
// On non-Windows platforms, this is a no-op.
func windowsFileDelay() {
	// No delay needed on Unix-like systems.
}

// retryOnWindows immediately executes the function on non-Windows platforms.
func retryOnWindows(fn func() error) error {
	return fn()
}

// RetryOnWindows is the exported version for use by other packages.
// On non-Windows platforms, it immediately executes the function.
func RetryOnWindows(fn func() error) error {
	defer perf.Track(nil, "output.RetryOnWindows")()

	return fn()
}
