package provider

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"log"
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

func TestClientDoGetOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	}))
	defer ts.Close()

	rt := NewDefaultTransport(false)
	opts := &HTTPClientOptions{IsWithRetries: false}
	hc, err := NewHTTPClient(rt, opts)
	require.Nil(t, err)

	// hc := &HTTPClient{Client: http.Client{}}

	req, err := http.NewRequest("GET", ts.URL, nil)
	require.Nil(t, err)

	res, err := hc.Do(req)
	require.Nil(t, err)

	require.Equal(t, 200, res.StatusCode)
}

func TestClientDisableRedirect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(302)
		_, _ = w.Write([]byte("OK"))
	}))
	defer ts.Close()

	rt := NewDefaultTransport(false)

	opts := &HTTPClientOptions{IsWithRetries: false}
	hc, err := NewHTTPClient(rt, opts)
	require.Nil(t, err)

	hc.DisableFollowRedirect()

	req, err := http.NewRequest("GET", ts.URL, nil)
	require.Nil(t, err)

	res, err := hc.Do(req)
	require.Nil(t, err)
	require.Equal(t, 302, res.StatusCode)
}

func TestClientDoResponseCheck(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte("OK"))
	}))
	defer ts.Close()

	opts := &HTTPClientOptions{IsWithRetries: false}
	hc := &HTTPClient{Client: http.Client{}, Options: opts}

	hc.CheckResponseStatus = SuccessOrRedirectResponseValidator

	req, err := http.NewRequest("GET", ts.URL, nil)
	require.Nil(t, err)

	res, err := hc.Do(req)
	require.Error(t, err)
	require.Equal(t, 400, res.StatusCode)
}

var errTransport = errors.New("transport failure")

// flakyTransport fails the first `failures` round trips and then delegates to next.
type flakyTransport struct {
	failures int32
	calls    atomic.Int32
	next     http.RoundTripper
}

func (f *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.calls.Add(1) <= f.failures {
		return nil, errTransport
	}
	return f.next.RoundTrip(req)
}

// captureLog redirects the standard logger for the duration of a test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	origOut, origFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(origOut)
		log.SetFlags(origFlags)
	})
	return &buf
}

func TestNewDefaultTransport(t *testing.T) {
	for _, skipVerify := range []bool{true, false} {
		tr := NewDefaultTransport(skipVerify)
		require.NotNil(t, tr.TLSClientConfig)
		assert.Equal(t, skipVerify, tr.TLSClientConfig.InsecureSkipVerify)
		// Regression: upstream left MinVersion unset (code-scanning missing-ssl-minversion).
		assert.Equal(t, uint16(tls.VersionTLS12), tr.TLSClientConfig.MinVersion)
		assert.NotNil(t, tr.Proxy)
		assert.NotNil(t, tr.DialContext)
	}
}

func TestBuildHttpClientOpts(t *testing.T) {
	tests := []struct {
		name        string
		attempts    string
		delay       string
		wantRetries bool
		wantCount   uint
		wantDelay   time.Duration
	}{
		{name: "defaults when unset", wantRetries: false, wantCount: DefaultAttemptsCount, wantDelay: DefaultRetryDelay},
		{name: "explicit values", attempts: "3", delay: "5", wantRetries: true, wantCount: 3, wantDelay: 5 * time.Second},
		{name: "invalid values fall back", attempts: "many", delay: "soon", wantRetries: false, wantCount: DefaultAttemptsCount, wantDelay: DefaultRetryDelay},
		{name: "negative values fall back", attempts: "-1", delay: "-2", wantRetries: false, wantCount: DefaultAttemptsCount, wantDelay: DefaultRetryDelay},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := BuildHttpClientOpts(&cfg.IDPAccount{HttpAttemptsCount: tt.attempts, HttpRetryDelay: tt.delay})
			assert.Equal(t, tt.wantRetries, opts.IsWithRetries)
			assert.Equal(t, tt.wantCount, opts.AttemptsCount)
			assert.Equal(t, tt.wantDelay, opts.RetryDelay)
		})
	}
}

func TestClientDoSetsUserAgent(t *testing.T) {
	var userAgent atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent.Store(r.UserAgent())
	}))
	defer ts.Close()

	hc, err := NewHTTPClient(http.DefaultTransport, &HTTPClientOptions{})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	require.NoError(t, err)
	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	assert.True(t, strings.HasPrefix(userAgent.Load().(string), "saml2aws/1.0 ("))
}

func TestClientDoWithRetries(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	}))
	defer ts.Close()

	tests := []struct {
		name      string
		failures  int32
		attempts  uint
		wantErr   bool
		wantCalls int32
	}{
		{name: "succeeds after transient failures", failures: 2, attempts: 3, wantCalls: 3},
		{name: "gives up after all attempts fail", failures: 5, attempts: 3, wantErr: true, wantCalls: 3},
		{name: "first attempt succeeds", failures: 0, attempts: 3, wantCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &flakyTransport{failures: tt.failures, next: http.DefaultTransport}
			hc, err := NewHTTPClient(tr, &HTTPClientOptions{
				IsWithRetries: true,
				AttemptsCount: tt.attempts,
				RetryDelay:    time.Millisecond,
			})
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
			require.NoError(t, err)

			res, err := hc.Do(req)
			assert.Equal(t, tt.wantCalls, tr.calls.Load())
			if tt.wantErr {
				require.Error(t, err)
				// retry-go v3 aggregates attempt errors without implementing Unwrap.
				assert.Contains(t, err.Error(), errTransport.Error())
				assert.Nil(t, res)
				return
			}
			require.NoError(t, err)
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Equal(t, "OK", string(body))
		})
	}
}

func TestClientDoTransportErrorSkipsResponseCheck(t *testing.T) {
	tr := &flakyTransport{failures: 1}
	hc, err := NewHTTPClient(tr, &HTTPClientOptions{})
	require.NoError(t, err)

	checked := false
	hc.CheckResponseStatus = func(*http.Request, *http.Response) error {
		checked = true
		return nil
	}

	req, err := http.NewRequest(http.MethodGet, "http://example.invalid", nil)
	require.NoError(t, err)

	_, err = hc.Do(req)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTransport)
	assert.False(t, checked)
}

func TestClientEnableFollowRedirect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("final"))
	}))
	defer ts.Close()

	hc, err := NewHTTPClient(http.DefaultTransport, &HTTPClientOptions{})
	require.NoError(t, err)
	hc.DisableFollowRedirect()
	hc.EnableFollowRedirect()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/start", nil)
	require.NoError(t, err)
	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "/final", res.Request.URL.Path)
}

func TestResponseValidators(t *testing.T) {
	tests := []struct {
		status             int
		wantErr            bool
		wantErrAllowing401 bool
	}{
		{status: http.StatusOK},
		{status: http.StatusNoContent},
		{status: http.StatusFound},
		{status: 399},
		{status: http.StatusBadRequest, wantErr: true, wantErrAllowing401: true},
		{status: http.StatusUnauthorized, wantErr: true, wantErrAllowing401: false},
		{status: http.StatusForbidden, wantErr: true, wantErrAllowing401: true},
		{status: http.StatusInternalServerError, wantErr: true, wantErrAllowing401: true},
		{status: 199, wantErr: true, wantErrAllowing401: true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "https://idp.example.com/login", nil)
			require.NoError(t, err)
			resp := &http.Response{StatusCode: tt.status, Status: http.StatusText(tt.status)}

			err = SuccessOrRedirectResponseValidator(req, resp)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "https://idp.example.com/login")
			} else {
				assert.NoError(t, err)
			}

			err = SuccessOrRedirectOrUnauthorizedResponseValidator(req, resp)
			if tt.wantErrAllowing401 {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestClientDoDumpsContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("response-body"))
	}))
	defer ts.Close()

	tests := []struct {
		name     string
		dump     string
		wantDump bool
	}{
		{name: "enabled", dump: "true", wantDump: true},
		{name: "disabled", dump: "false", wantDump: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DUMP_CONTENT", tt.dump)
			logs := captureLog(t)

			hc, err := NewHTTPClient(http.DefaultTransport, &HTTPClientOptions{})
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/submit", strings.NewReader("request-body"))
			require.NoError(t, err)

			res, err := hc.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			// The response body must remain readable after it has been dumped.
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Equal(t, "response-body", string(body))

			if tt.wantDump {
				assert.Contains(t, logs.String(), "POST /submit HTTP/1.1")
				assert.Contains(t, logs.String(), "request-body")
				assert.Contains(t, logs.String(), "response-body")
			} else {
				assert.Empty(t, logs.String())
			}
		})
	}
}
