package github

import "time"

// ArchivedCheckTimeoutForTest returns the current archived-check timeout for use in tests.
//
// NOTE: These helpers manipulate package-level state via atomic operations and are safe
// for concurrent access, but they mutate shared state — do not add t.Parallel() to tests
// that call SetArchivedCheckTimeoutForTest, as parallel sub-tests with different timeout
// values would interfere with each other.
func ArchivedCheckTimeoutForTest() time.Duration {
	return getArchivedCheckTimeout()
}

// SetArchivedCheckTimeoutForTest sets the archived-check timeout and returns a function
// that resets it to the previous value. Intended for use with t.Cleanup.
//
// Thread-safe: uses atomic store internally, so it is safe to call concurrently
// with IsRepoArchived. However, parallel sub-tests that each set a different value
// will race on the shared timeout; only use in sequential sub-tests.
func SetArchivedCheckTimeoutForTest(d time.Duration) func() {
	prev := getArchivedCheckTimeout()
	setArchivedCheckTimeout(d)
	return func() { setArchivedCheckTimeout(prev) }
}
