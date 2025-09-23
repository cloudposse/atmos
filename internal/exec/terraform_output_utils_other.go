//go:build !windows
// +build !windows

package exec

// windowsFileDelay is a no-op on non-Windows platforms.
func windowsFileDelay() {
	// No delay needed on Unix-like systems
}

// retryOnWindows immediately executes the function on non-Windows platforms.
func retryOnWindows(fn func() error) error {
	return fn()
}
