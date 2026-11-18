package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression: the client-wide timeout cut off a request whose context allowed a longer wait, such as the
// ADFS Azure MFA wait, where the server can hold the request open until the user approves.
func TestClientDoContextDeadlineOutlastsClientTimeout(t *testing.T) {
	const (
		clientTimeout = 50 * time.Millisecond
		serverHold    = 300 * time.Millisecond
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(serverHold)
		_, _ = w.Write([]byte("approved"))
	}))
	defer server.Close()

	for _, withRetries := range []bool{false, true} {
		t.Run(fmt.Sprintf("retries=%t", withRetries), func(t *testing.T) {
			hc, err := NewHTTPClient(http.DefaultTransport, &HTTPClientOptions{IsWithRetries: withRetries, AttemptsCount: 1})
			require.NoError(t, err)
			hc.Timeout = clientTimeout

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, http.NoBody)
			require.NoError(t, err)

			res, err := hc.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Equal(t, "approved", string(body))
			assert.Equal(t, clientTimeout, hc.Timeout, "the shared client's timeout is not changed")
		})
	}
}

// The context deadline still bounds the request when it is shorter than the server's wait.
func TestClientDoContextDeadlineBoundsRequest(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	hc, err := NewHTTPClient(http.DefaultTransport, &HTTPClientOptions{})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, http.NoBody)
	require.NoError(t, err)

	res, err := hc.Do(req)
	if res != nil {
		_ = res.Body.Close()
	}
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// cookieRotatingTransport answers the first /login with a redirect that rotates the session cookie,
// fails the redirect target once, and records the Cookie header of every /login request.
type cookieRotatingTransport struct {
	mu           sync.Mutex
	calls        int
	loginCookies []string
	loginHeaders []string
}

func (c *cookieRotatingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if req.URL.Path == "/next" {
		return nil, errTransport
	}
	c.loginCookies = append(c.loginCookies, req.Header.Get("Cookie"))
	c.loginHeaders = append(c.loginHeaders, req.Header.Get("X-Caller"))
	if c.calls == 1 {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": {"/next"}, "Set-Cookie": {"sid=new; Path=/"}},
			Body:       http.NoBody,
			Request:    req,
		}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: http.NoBody, Request: req}, nil
}

// Regression: http.Client adds the jar's cookies to the request it is given, so a retry of the same
// request sent the expired session cookie of the failed attempt next to its replacement.
func TestClientDoRetrySendsOnlyCurrentCookies(t *testing.T) {
	tr := &cookieRotatingTransport{}
	hc, err := NewHTTPClient(tr, &HTTPClientOptions{IsWithRetries: true, AttemptsCount: 2, RetryDelay: time.Millisecond})
	require.NoError(t, err)
	loginURL, err := url.Parse("http://idp.example.com/login")
	require.NoError(t, err)
	hc.Jar.SetCookies(loginURL, []*http.Cookie{{Name: "sid", Value: "old", Path: "/"}})

	req, err := http.NewRequest(http.MethodGet, loginURL.String(), http.NoBody)
	require.NoError(t, err)
	req.Header.Set("X-Caller", "kept")

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	assert.Equal(t, []string{"sid=old", "sid=new"}, tr.loginCookies)
	assert.Equal(t, []string{"kept", "kept"}, tr.loginHeaders, "caller-set headers are sent on every attempt")
}

// Regression: a rejected response status was a string-only error that errors.Is could not identify.
func TestResponseValidatorErrorIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	hc, err := NewHTTPClient(http.DefaultTransport, &HTTPClientOptions{})
	require.NoError(t, err)
	hc.CheckResponseStatus = SuccessOrRedirectResponseValidator
	req, err := http.NewRequest(http.MethodGet, server.URL+"/login?code=secret-value", http.NoBody) // gitleaks:allow -- Fake code proving redaction.
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NotNil(t, res)
	_ = res.Body.Close()
	require.ErrorIs(t, err, ErrUnexpectedStatus)
	assert.Equal(t, "request for url: "+server.URL+"/login failed status: 403 Forbidden", err.Error())

	// Callers wrap the error with github.com/pkg/errors, and errors.Is still matches through the wrapper.
	wrapped := pkgerrors.Wrap(err, "error retrieving form")
	require.ErrorIs(t, wrapped, ErrUnexpectedStatus)
	assert.False(t, errors.Is(errTransport, ErrUnexpectedStatus), "other errors do not match")
}
