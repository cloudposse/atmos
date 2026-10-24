package tests

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

type rateLimitRoundTripFunc func(*http.Request) (*http.Response, error)

func (f rateLimitRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGitHubRateLimitProbeFailures(t *testing.T) {
	transportErr := errors.New("test transport failure")
	bodyErr := errors.New("test response read failure")
	tests := []struct {
		name       string
		url        string
		status     int
		body       string
		readErr    error
		requestErr error
		wantErr    bool
		wantCalls  int
	}{
		{name: "invalid request URL", url: "://invalid", wantErr: true},
		{name: "transport failure", requestErr: transportErr, wantErr: true, wantCalls: 1},
		{name: "unsuccessful response", status: http.StatusServiceUnavailable, wantCalls: 1},
		{name: "unreadable body", status: http.StatusOK, readErr: bodyErr, wantCalls: 1},
		{name: "invalid JSON", status: http.StatusOK, body: "{", wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestURL := tt.url
			if requestURL == "" {
				requestURL = githubRateLimitURL
			}
			calls := 0
			client := &http.Client{Transport: rateLimitRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, githubRateLimitURL, req.URL.String())
				if tt.requestErr != nil {
					return nil, tt.requestErr
				}
				var body io.Reader = strings.NewReader(tt.body)
				if tt.readErr != nil {
					body = iotest.ErrReader(tt.readErr)
				}
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(body), Header: make(http.Header)}, nil
			})}

			info, err := probeGitHubRateLimit(client, requestURL, "")
			assert.Nil(t, info)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrHTTPRequestFailed)
				if tt.requestErr != nil {
					assert.ErrorIs(t, err, tt.requestErr)
				}
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantCalls, calls)

			// A failed best-effort probe must not skip the caller's test.
			var ranPastProbe bool
			t.Run("wrapper does not skip", func(t *testing.T) {
				assert.Nil(t, checkGitHubRateLimit(t, client, requestURL, ""))
				ranPastProbe = true
			})
			assert.True(t, ranPastProbe)
			assert.Equal(t, 2*tt.wantCalls, calls)
		})
	}
}

func TestCheckGitHubRateLimit_LowQuotaStillRuns(t *testing.T) {
	client := &http.Client{Transport: rateLimitRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"rate":{"limit":60,"remaining":1,"reset":1893456000}}`)),
			Header:     make(http.Header),
		}, nil
	})}
	var ranPastProbe bool
	t.Run("low quota does not skip", func(t *testing.T) {
		info := checkGitHubRateLimit(t, client, githubRateLimitURL, "")
		require.NotNil(t, info)
		assert.Equal(t, 1, info.Remaining)
		assert.Equal(t, 60, info.Limit)
		assert.Equal(t, int64(1893456000), info.Reset.Unix())
		ranPastProbe = true
	})
	assert.True(t, ranPastProbe)
}

func TestRequireLiveGitHubAuthenticated_WithTokenAndProbeBypass(t *testing.T) {
	t.Setenv("ATMOS_TEST_OFFLINE", "false")
	t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	t.Setenv("GITHUB_TOKEN", "test-token")
	var ranPastProbe bool
	t.Run("authenticated bypass does not skip", func(t *testing.T) {
		assert.Nil(t, RequireLiveGitHubAuthenticated(t))
		ranPastProbe = true
	})
	assert.True(t, ranPastProbe)
}
