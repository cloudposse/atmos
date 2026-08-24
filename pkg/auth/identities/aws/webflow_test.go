package aws

// Shared test fixtures and helper utilities for the webflow_*_test.go files.
// Individual feature tests live in the matching webflow_<subject>_test.go file.

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// refreshCachePreservationTestCase describes a single transient-failure scenario
// where the refresh cache must be preserved (not deleted).
type refreshCachePreservationTestCase struct {
	name     string
	handler  http.HandlerFunc
	netError error // If set, bypass httptest and return this error from the client.
}

// mockHTTPClient is a test helper for mocking HTTP requests.
type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

// overrideStdinReadable forces webflowStdinIsReadableFunc to return true for
// the duration of the test. Required whenever a test swaps webflowStdinReader
// for a non-os.Stdin reader (pipe, fake, etc.) because the default readability
// check only returns true for a real terminal os.Stdin. Without this override,
// the stdin reader goroutine in browserWebflowNonInteractive is never started
// and stdin-driven tests time out on the callback.
func overrideStdinReadable(t *testing.T) {
	t.Helper()
	orig := webflowStdinIsReadableFunc
	webflowStdinIsReadableFunc = func() bool { return true }
	t.Cleanup(func() { webflowStdinIsReadableFunc = orig })
}

// blockStdin swaps os.Stdin for a pipe whose write end is never written to,
// so callers reading from stdin block indefinitely. Used by non-interactive
// webflow tests to prevent the stdin-scanner goroutine from winning the
// select race (in `go test` the real os.Stdin is closed, producing an
// immediate EOF). Cleanup restores the original os.Stdin and closes the pipe,
// which unblocks any stranded reader so it can exit.
//
// Also overrides webflowStdinIsReadableFunc so browserWebflowNonInteractive
// actually starts its reader goroutine; the default readability check only
// accepts a real terminal os.Stdin.
func blockStdin(t *testing.T) {
	t.Helper()
	overrideStdinReadable(t)
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := webflowStdinReader
	webflowStdinReader = r
	t.Cleanup(func() {
		// Close writer first so any stranded Scan() returns with EOF,
		// then reset the override after the goroutine has had time to exit.
		_ = w.Close()
		_ = r.Close()
		webflowStdinReader = orig
	})
}

// stdinReaderWith replaces webflowStdinReader with a pipe whose write end is
// returned to the caller, enabling tests to inject bytes and then close the
// writer. Used by the stdin-driven authorization-code tests. Also overrides
// webflowStdinIsReadableFunc so the reader goroutine is actually started.
func stdinReaderWith(t *testing.T) *os.File {
	t.Helper()
	overrideStdinReadable(t)
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := webflowStdinReader
	webflowStdinReader = r
	t.Cleanup(func() {
		_ = w.Close()
		_ = r.Close()
		webflowStdinReader = orig
	})
	return w
}

// simulateCallback posts an OAuth2 callback to the redirect_uri encoded in the
// authorization URL. Used by tests that mock displayWebflowDialogFunc /
// displayWebflowPlainTextFunc to drive the callback path without needing stdin
// or a real browser.
func simulateCallback(t *testing.T, authURL, code string) {
	t.Helper()
	parsed, err := url.Parse(authURL)
	require.NoError(t, err)
	redirectURI := parsed.Query().Get("redirect_uri")
	state := parsed.Query().Get("state")
	callbackURL := fmt.Sprintf("%s?code=%s&state=%s", redirectURI, code, state)
	resp, err := http.Get(callbackURL)
	if err == nil {
		resp.Body.Close()
	}
}
