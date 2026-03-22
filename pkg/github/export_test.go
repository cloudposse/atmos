package github

import "time"

// ArchivedCheckTimeoutForTest returns the current archived-check timeout for use in tests.
//
// NOTE: These helpers mutate package-level state and are not safe for use with t.Parallel().
// Do not add t.Parallel() to tests that call SetArchivedCheckTimeoutForTest.
func ArchivedCheckTimeoutForTest() time.Duration {
	return archivedCheckTimeout
}

// SetArchivedCheckTimeoutForTest sets the archived-check timeout and returns a function
// that resets it to the previous value. Intended for use with t.Cleanup.
//
// NOTE: Not goroutine-safe. Do not call from parallel sub-tests.
func SetArchivedCheckTimeoutForTest(d time.Duration) func() {
	prev := archivedCheckTimeout
	archivedCheckTimeout = d
	return func() { archivedCheckTimeout = prev }
}
