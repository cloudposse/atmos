package provider

import (
	"net/http"
	"net/http/httptest"
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
