package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression: Go reports a malformed redirect Location inside the *url.Error cause, so redacting only the
// URL field left a session token from the Location in the returned error.
func TestRedactErrorMalformedRedirectLocation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/bad%zz?sessionToken=leaked-token-value") // gitleaks:allow -- Fake token proving redaction.
		w.WriteHeader(http.StatusFound)
	}))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/login?code=leaked-code-value", http.NoBody) // gitleaks:allow -- Fake code proving redaction.
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	if res != nil {
		_ = res.Body.Close()
	}
	require.Error(t, err)
	require.Contains(t, err.Error(), "leaked-token-value", "precondition: Go puts the Location in the cause")

	redacted := RedactError(err)
	assert.NotContains(t, redacted.Error(), "leaked-token-value")
	assert.NotContains(t, redacted.Error(), "leaked-code-value")
	assert.Contains(t, redacted.Error(), "/login")

	var urlErr *url.Error
	require.ErrorAs(t, redacted, &urlErr)
	assert.Equal(t, ts.URL+"/login", urlErr.URL)
}

// Regression: (*url.Error).Error formats the URL with %q, so a URL containing a quote appears escaped in a
// wrapped message and a raw-text replacement missed it.
func TestRedactErrorWrappedQuotedURL(t *testing.T) {
	errTransport := errors.New("transport failed")
	inner := &url.Error{Op: "Get", URL: `https://idp.example/login?sessionToken=se"cr\et-value`, Err: errTransport} // gitleaks:allow -- Fake token proving redaction.
	wrapped := fmt.Errorf("authenticating: %w", inner)

	redacted := RedactError(wrapped)
	assert.NotContains(t, redacted.Error(), "sessionToken")
	assert.NotContains(t, redacted.Error(), "et-value")
	assert.Contains(t, redacted.Error(), "https://idp.example/login")
	assert.Contains(t, redacted.Error(), "transport failed")
	require.ErrorIs(t, redacted, errTransport)
	var urlErr *url.Error
	require.ErrorAs(t, redacted, &urlErr)
}

// The redacted cause still reports timeouts, so callers relying on (*url.Error).Timeout keep working.
func TestRedactErrorPreservesTimeout(t *testing.T) {
	err := &url.Error{Op: "Post", URL: "https://idp.example/authn?token=secret-value", Err: context.DeadlineExceeded} // gitleaks:allow -- Fake token proving redaction.

	redacted := RedactError(err)
	var urlErr *url.Error
	require.ErrorAs(t, redacted, &urlErr)
	assert.True(t, urlErr.Timeout())
	require.ErrorIs(t, redacted, context.DeadlineExceeded)
	assert.NotContains(t, redacted.Error(), "secret-value")
}

func TestRedactMessage(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: `Get "https://h/p?token=abc": dial tcp: refused`, want: `Get "https://h/p?[redacted]": dial tcp: refused`},
		{in: `parse "/bad%zz?sessionToken=abc": invalid URL escape "%zz"`, want: `parse "/bad%zz?[redacted]": invalid URL escape "%zz"`},
		{in: "redirect to /next?code=abc failed", want: "redirect to /next?[redacted] failed"},
		{in: "ends with /x?code=abc", want: "ends with /x?[redacted]"},
		{in: "fragment /x#otp=123456 here", want: "fragment /x?[redacted] here"},
		{in: "no url here", want: "no url here"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, redactMessage(tt.in))
		})
	}
}
