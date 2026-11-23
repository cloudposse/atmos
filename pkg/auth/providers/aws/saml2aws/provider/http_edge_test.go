package provider

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With redirects disabled, the redirect response itself is returned instead of the redirect target.
func TestClientDisableRedirectReturnsRedirectResponse(t *testing.T) {
	var targetHits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			targetHits++
			return
		}
		http.Redirect(w, r, "/target", http.StatusFound)
	}))
	defer ts.Close()

	hc, err := NewHTTPClient(NewDefaultTransport(false), &HTTPClientOptions{})
	require.NoError(t, err)
	hc.DisableFollowRedirect()

	req, err := http.NewRequest(http.MethodGet, ts.URL, http.NoBody)
	require.NoError(t, err)
	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusFound, res.StatusCode)
	assert.Equal(t, "/target", res.Header.Get("Location"))
	assert.Zero(t, targetHits, "the redirect must not be followed")
}

// If the request body cannot be recreated for a retry, the retry stops with that error.
func TestClientDoWithRetriesStopsWhenBodyCannotBeRewound(t *testing.T) {
	errGetBody := errors.New("cannot rewind body")
	tr := &flakyTransport{failures: 5, next: http.DefaultTransport}
	hc, err := NewHTTPClient(tr, &HTTPClientOptions{IsWithRetries: true, AttemptsCount: 3, RetryDelay: time.Millisecond})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:1/", strings.NewReader("payload"))
	require.NoError(t, err)
	req.GetBody = func() (io.ReadCloser, error) { return nil, errGetBody }

	res, err := hc.Do(req)
	if res != nil {
		_ = res.Body.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), errGetBody.Error())
	assert.Equal(t, int32(1), tr.calls.Load(), "no second attempt after the body could not be rewound")
}

// A *url.Error without a cause is redacted without panicking.
func TestRedactErrorNilCause(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://idp.example.com/login?token=secret-value", Err: nil} // gitleaks:allow -- Fake token proving redaction.
	redacted := RedactError(err)

	var urlErr *url.Error
	require.ErrorAs(t, redacted, &urlErr)
	assert.NotContains(t, urlErr.URL, "secret-value")
	assert.NoError(t, urlErr.Err)
}
