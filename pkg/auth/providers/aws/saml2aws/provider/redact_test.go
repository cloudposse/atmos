package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

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
	// errors.As must expose the sanitized URL, not the original one with the session token.
	assert.Equal(t, "https://idp.example/login", urlErr.URL)
	require.ErrorIs(t, urlErr, errTransport)
}

// Regression: errors.As selects only the first *url.Error of a joined error, so user info in a later URL
// error stayed in the displayed message.
func TestRedactErrorJoinedURLErrors(t *testing.T) {
	errTransport := errors.New("transport failed")
	first := &url.Error{Op: "Get", URL: "https://idp.example/first", Err: errTransport}
	second := &url.Error{Op: "Get", URL: "https://user:pa55word@idp.example/second", Err: errTransport} // gitleaks:allow -- Fake credentials proving redaction.
	joined := errors.Join(first, second)

	redacted := RedactError(joined)
	assert.NotContains(t, redacted.Error(), "pa55word")
	assert.NotContains(t, redacted.Error(), "user:")
	assert.Contains(t, redacted.Error(), "https://idp.example/first")
	assert.Contains(t, redacted.Error(), "https://idp.example/second")
	require.ErrorIs(t, redacted, errTransport)
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
		// Regression: whitespace inside a quoted URL ended the redaction before the rest of the query string.
		{
			in:   `failed to parse Location header "/bad%zz?label=hello world&sessionToken=secret-value": invalid URL escape "%zz"`,
			want: `failed to parse Location header "/bad%zz?[redacted]": invalid URL escape "%zz"`,
		},
		{in: `header "/x?a=b c&code=secret-value" rejected`, want: `header "/x?[redacted]" rejected`},
		{in: `header "/x#a b&otp=secret-value"`, want: `header "/x?[redacted]"`},
		{in: `escaped "/x?a=\" b&code=secret-value": done`, want: `escaped "/x?[redacted]": done`},
		{in: `quoted "no query here" stays`, want: `quoted "no query here" stays`},
		// The same URL twice in one message.
		{
			in:   `Get "https://h/p?code=secret-value": a; Get "https://h/p?code=secret-value": b`,
			want: `Get "https://h/p?[redacted]": a; Get "https://h/p?[redacted]": b`,
		},
		{in: "to /p?code=secret-value and /p?code=secret-value", want: "to /p?[redacted] and /p?[redacted]"},
		// Overlapping URLs, where one is a prefix of the other.
		{
			in:   `Get "https://h/p?code=": a; Get "https://h/p?code=secret-value": b`,
			want: `Get "https://h/p?[redacted]": a; Get "https://h/p?[redacted]": b`,
		},
		// An unbalanced quote does not stop the unquoted pass.
		{in: `odd "quote then /p?code=secret-value end`, want: `odd "quote then /p?[redacted] end`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := redactMessage(tt.in)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "secret-value")
			// Redaction is idempotent, so a message redacted twice (cause, then wrapper) is unchanged.
			assert.Equal(t, got, redactMessage(got))
		})
	}
}

// Regression: a raw replacement of a URL that is a prefix of another URL cut the query delimiter off the
// longer URL, so its secret survived in the displayed message. Both orders and duplicates are covered.
func TestRedactErrorOverlappingURLs(t *testing.T) {
	errTransport := errors.New("transport failed")
	short := &url.Error{Op: "Get", URL: "https://idp.example/login?sessionToken=", Err: errTransport}
	long := &url.Error{Op: "Get", URL: "https://idp.example/login?sessionToken=secret-value", Err: errTransport} // gitleaks:allow -- Fake token proving redaction.
	tests := []struct {
		name string
		err  error
	}{
		{name: "short first", err: errors.Join(short, long)},
		{name: "long first", err: errors.Join(long, short)},
		{name: "duplicates", err: errors.Join(short, long, short, long)},
		{name: "wrapped", err: fmt.Errorf("authenticating: %w", errors.Join(short, long))},
		// The longer URL appears only as raw text, outside any *url.Error, so it is not replaced as a whole.
		{name: "raw text", err: fmt.Errorf("redirected to %s: %w", long.URL, short)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redacted := RedactError(tt.err)
			assert.NotContains(t, redacted.Error(), "secret-value")
			assert.NotContains(t, redacted.Error(), "sessionToken")
			assert.Contains(t, redacted.Error(), "https://idp.example/login")
			require.ErrorIs(t, redacted, errTransport)
		})
	}
}

// Regression: Go quotes a malformed redirect Location in its error, and whitespace inside it ended the
// redaction early, so the rest of the query string reached the returned error and the retry log.
func TestRedactErrorMalformedRedirectLocationWithSpace(t *testing.T) {
	const secret = "spaced-token-value" // gitleaks:allow -- Fake token proving redaction.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/bad%zz?label=hello world&sessionToken="+secret)
		w.WriteHeader(http.StatusFound)
	}))
	defer ts.Close()

	// Precondition: Go puts the complete Location, space included, in the error.
	plainReq, err := http.NewRequest(http.MethodGet, ts.URL+"/login", http.NoBody)
	require.NoError(t, err)
	plainRes, err := http.DefaultClient.Do(plainReq)
	if plainRes != nil {
		_ = plainRes.Body.Close()
	}
	require.Error(t, err)
	require.Contains(t, err.Error(), "hello world&sessionToken="+secret)

	for _, withRetries := range []bool{false, true} {
		t.Run(fmt.Sprintf("retries=%t", withRetries), func(t *testing.T) {
			logrusLog := captureLogrus(t)
			hc, err := NewHTTPClient(http.DefaultTransport, &HTTPClientOptions{
				IsWithRetries: withRetries,
				AttemptsCount: 2,
				RetryDelay:    time.Millisecond,
			})
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodGet, ts.URL+"/login", http.NoBody)
			require.NoError(t, err)

			res, err := hc.Do(req)
			if res != nil {
				_ = res.Body.Close()
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "/login")
			assert.NotContains(t, err.Error(), secret)
			assert.NotContains(t, err.Error(), "sessionToken")
			if withRetries {
				require.Contains(t, logrusLog.String(), "Retry", "precondition: the failed attempt is logged")
			}
			assert.NotContains(t, logrusLog.String(), secret)
		})
	}
}
