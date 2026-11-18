package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
)

// Regression: http_attempts_count "0" enabled retries with zero attempts, so retry-go never ran
// the request and returned an empty, non-nil error.
func TestBuildHttpClientOpts_ZeroAttemptsRunsOnce(t *testing.T) {
	opts := BuildHttpClientOpts(&cfg.IDPAccount{HttpAttemptsCount: "0"})
	assert.Equal(t, uint(1), opts.AttemptsCount)

	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
	}))
	defer ts.Close()

	hc, err := NewHTTPClient(http.DefaultTransport, opts)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, ts.URL, http.NoBody)
	require.NoError(t, err)
	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), calls.Load())
}

// Options built by hand (not through BuildHttpClientOpts) are clamped the same way.
func TestClientDoWithRetry_ZeroAttemptsRunsOnce(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
	}))
	defer ts.Close()

	hc, err := NewHTTPClient(http.DefaultTransport, &HTTPClientOptions{IsWithRetries: true, AttemptsCount: 0, RetryDelay: time.Millisecond})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, ts.URL, http.NoBody)
	require.NoError(t, err)
	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, int32(1), calls.Load())
}

// A client created without options sends the request once instead of dereferencing nil options.
func TestClientDo_NilOptions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer ts.Close()

	hc, err := NewHTTPClient(http.DefaultTransport, nil)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, ts.URL, http.NoBody)
	require.NoError(t, err)
	var resp *http.Response
	require.NotPanics(t, func() { resp, err = hc.Do(req) })
	require.NoError(t, err)
	_ = resp.Body.Close()
}

// bodyConsumingTransport reads the whole request body on every attempt, fails the first attempts,
// and then answers 200.
type bodyConsumingTransport struct {
	failures int
	bodies   []string
}

func (b *bodyConsumingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	b.bodies = append(b.bodies, string(data))
	if len(b.bodies) <= b.failures {
		return nil, errors.New("transport failure")
	}
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
}

// Regression: a retried POST sends the original body again, not the body the failed attempt consumed.
func TestClientDoWithRetry_ResendsRequestBody(t *testing.T) {
	tr := &bodyConsumingTransport{failures: 1}
	hc, err := NewHTTPClient(tr, &HTTPClientOptions{IsWithRetries: true, AttemptsCount: 3, RetryDelay: time.Millisecond})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "https://idp.example.com/login", strings.NewReader("user=alice&pass=s3cret"))
	require.NoError(t, err)
	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, []string{"user=alice&pass=s3cret", "user=alice&pass=s3cret"}, tr.bodies)
}

// Bodyless requests stay retryable.
func TestClientDoWithRetry_BodylessRequestStillRetried(t *testing.T) {
	tr := &flakyTransport{failures: 1, next: http.DefaultTransport}
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer ts.Close()
	hc, err := NewHTTPClient(tr, &HTTPClientOptions{IsWithRetries: true, AttemptsCount: 2, RetryDelay: time.Millisecond})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	require.NoError(t, err)
	resp, err := hc.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, int32(2), tr.calls.Load())
}

// Regression: cancelling the request context stops the retry backoff instead of waiting it out.
func TestClientDoWithRetry_StopsOnCancelledContext(t *testing.T) {
	tr := &flakyTransport{failures: 10, next: http.DefaultTransport}
	hc, err := NewHTTPClient(tr, &HTTPClientOptions{IsWithRetries: true, AttemptsCount: 5, RetryDelay: time.Minute})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://idp.example.com/", nil)
	require.NoError(t, err)

	start := time.Now()
	_, err = hc.Do(req)
	require.Error(t, err)
	assert.Less(t, time.Since(start), 10*time.Second)
	assert.Equal(t, int32(1), tr.calls.Load())
}
