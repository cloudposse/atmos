//go:build windows

package config

import (
	"time"
)

func init() {
	// Set the platform-specific locking function.
	withCacheFileLock = withCacheFileLockWindows
}

func withCacheFileLockWindows(cacheFile string, fn func() error) error {
	// No file locking on Windows to avoid timeout issues.
	// The cache is non-critical functionality, so we can operate
	// without strict locking on Windows.

	// Add a small delay after operations to let Windows release file handles.
	defer func() {
		time.Sleep(50 * time.Millisecond)
	}()

	// Just execute the function without any locking.
	return fn()
}
