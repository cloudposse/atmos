package github

import "time"

// ArchivedCheckTimeoutForTest returns the current archived-check timeout for use in tests.
func ArchivedCheckTimeoutForTest() time.Duration {
	return archivedCheckTimeout
}

// SetArchivedCheckTimeoutForTest sets the archived-check timeout and returns a function
// that resets it to the previous value. Intended for use with t.Cleanup.
func SetArchivedCheckTimeoutForTest(d time.Duration) func() {
	prev := archivedCheckTimeout
	archivedCheckTimeout = d
	return func() { archivedCheckTimeout = prev }
}
