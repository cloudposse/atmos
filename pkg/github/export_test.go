package github

import (
	"context"
	"time"

	ghpkg "github.com/google/go-github/v59/github"
)

// ArchivedCheckTimeoutForTest returns the current archived-check timeout for use in tests.
//
// NOTE: These helpers manipulate package-level state via atomic operations and are safe
// for concurrent access, but they mutate shared state — do not add t.Parallel() to tests
// that call SetArchivedCheckTimeoutForTest, as parallel sub-tests with different timeout
// values would interfere with each other.
//
// IMPORTANT: ATMOS_GITHUB_ARCHIVED_CHECK_TIMEOUT is read once in init() and stored
// atomically. Calling os.Setenv("ATMOS_GITHUB_ARCHIVED_CHECK_TIMEOUT", ...) in a test
// has NO EFFECT because init() has already run. Use SetArchivedCheckTimeoutForTest
// to override the timeout in tests.
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

// SetNewGitHubClientHookForTest replaces the GitHub client creator used by IsRepoArchived
// with a custom function, enabling tests to inject an httptest.NewServer-backed client
// without making real network calls. Returns a cleanup function that restores the
// original (nil) hook.
//
// Thread-safe: uses newGitHubClientHookMu so concurrent IsRepoArchived calls are safe.
// Do not use in sub-tests that run in parallel with each other — the last writer wins.
func SetNewGitHubClientHookForTest(fn func(ctx context.Context) *ghpkg.Client) func() {
	newGitHubClientHookMu.Lock()
	prev := newGitHubClientHook
	newGitHubClientHook = fn
	newGitHubClientHookMu.Unlock()
	return func() {
		newGitHubClientHookMu.Lock()
		newGitHubClientHook = prev
		newGitHubClientHookMu.Unlock()
	}
}
