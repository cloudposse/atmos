package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// roundTripperFunc is a helper type for creating mock http.RoundTrippers.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNewDefaultClient(t *testing.T) {
	tests := []struct {
		name string
		opts []ClientOption
	}{
		{
			name: "creates client with default timeout",
			opts: nil,
		},
		{
			name: "creates client with custom timeout",
			opts: []ClientOption{WithTimeout(10 * time.Second)},
		},
		{
			name: "creates client with GitHub token",
			opts: []ClientOption{WithGitHubToken("test-token")},
		},
		{
			name: "creates client with timeout and token",
			opts: []ClientOption{
				WithTimeout(30 * time.Second),
				WithGitHubToken("test-token"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewDefaultClient(tt.opts...)
			assert.NotNil(t, client)
			assert.IsType(t, &DefaultClient{}, client)
		})
	}
}

func TestGetGitHubTokenFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		atmosToken  string
		githubToken string
		want        string
	}{
		{
			name:        "prefers ATMOS_GITHUB_TOKEN",
			atmosToken:  "atmos-token",
			githubToken: "github-token",
			want:        "atmos-token",
		},
		{
			name:        "falls back to GITHUB_TOKEN",
			atmosToken:  "",
			githubToken: "github-token",
			want:        "github-token",
		},
		{
			name:        "returns empty when neither set",
			atmosToken:  "",
			githubToken: "",
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_GITHUB_TOKEN", tt.atmosToken)
			t.Setenv("GITHUB_TOKEN", tt.githubToken)

			// Use an isolated viper instance to avoid mutating the global singleton.
			// This prevents BindEnv from leaking env-var mappings into subsequent tests.
			// In production, GlobalOptionsBuilder binds these on the global viper instance.
			v := viper.New()
			_ = v.BindEnv("github-token", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN")

			got := GetGitHubTokenFromEnv(v)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGitHubAuthenticatedTransport_RoundTrip(t *testing.T) {
	tests := []struct {
		name            string
		url             string
		token           string
		expectAuth      bool
		expectUserAgent bool
	}{
		{
			name:            "api.github.com sets headers",
			url:             "https://api.github.com/repos/test/repo",
			token:           "test-token",
			expectAuth:      true,
			expectUserAgent: true,
		},
		{
			name:            "raw.githubusercontent.com sets headers",
			url:             "https://raw.githubusercontent.com/test/repo/main/file.txt",
			token:           "test-token",
			expectAuth:      true,
			expectUserAgent: true,
		},
		{
			name:            "non-github domain does not set headers",
			url:             "https://example.com/api",
			token:           "test-token",
			expectAuth:      false,
			expectUserAgent: false,
		},
		{
			name:            "uploads.github.com sets headers",
			url:             "https://uploads.github.com/repos/test/repo/releases/1/assets",
			token:           "test-token",
			expectAuth:      true,
			expectUserAgent: true,
		},
		{
			// github.example.com looks like it has "github" in it but is NOT an allowed host.
			// Authorization must NOT be leaked to arbitrary subdomains.
			name:            "github.example.com does not set auth header",
			url:             "https://github.example.com/api",
			token:           "test-token",
			expectAuth:      false,
			expectUserAgent: false,
		},
		{
			// example.github.com is a GitHub-owned subdomain but NOT in the explicit allowlist.
			// Authorization must NOT be set for unlisted GitHub subdomains.
			name:            "example.github.com does not set auth header",
			url:             "https://example.github.com/api",
			token:           "test-token",
			expectAuth:      false,
			expectUserAgent: false,
		},
		{
			// Plain HTTP (not HTTPS) api.github.com — Authorization MUST NOT be set.
			// Sending tokens over unencrypted HTTP would leak credentials.
			name:            "http scheme api.github.com does not set auth header",
			url:             "http://api.github.com/repos/test/repo",
			token:           "test-token",
			expectAuth:      false,
			expectUserAgent: false,
		},
		{
			name:            "empty token does not set auth header",
			url:             "https://api.github.com/repos/test/repo",
			token:           "",
			expectAuth:      false,
			expectUserAgent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock transport to capture the request
			var capturedRequest *http.Request
			mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				capturedRequest = req
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("OK")),
				}, nil
			})

			// Initialize the GitHubAuthenticatedTransport
			transport := &GitHubAuthenticatedTransport{
				Base:        mockTransport,
				GitHubToken: tt.token,
			}

			// Create a test request
			req, err := http.NewRequest("GET", tt.url, nil)
			require.NoError(t, err)

			// Perform the RoundTrip
			resp, err := transport.RoundTrip(req)
			require.NoError(t, err)
			require.NotNil(t, resp)
			defer resp.Body.Close()

			// Verify headers
			if tt.expectAuth {
				expectedAuth := "Bearer " + tt.token
				assert.Equal(t, expectedAuth, capturedRequest.Header.Get("Authorization"))
			} else {
				assert.Empty(t, capturedRequest.Header.Get("Authorization"))
			}

			if tt.expectUserAgent {
				assert.Equal(t, "atmos-toolchain/1.0", capturedRequest.Header.Get("User-Agent"))
			} else {
				assert.Empty(t, capturedRequest.Header.Get("User-Agent"))
			}
		})
	}
}

func TestGet_Success(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   string
		responseStatus int
		want           []byte
	}{
		{
			name:           "successful GET request with JSON response",
			responseBody:   `{"status":"ok"}`,
			responseStatus: http.StatusOK,
			want:           []byte(`{"status":"ok"}`),
		},
		{
			name:           "successful GET request with empty response",
			responseBody:   "",
			responseStatus: http.StatusOK,
			want:           []byte{},
		},
		{
			name:           "successful GET request with text response",
			responseBody:   "Hello, World!",
			responseStatus: http.StatusOK,
			want:           []byte("Hello, World!"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create client and make request.
			client := NewDefaultClient(WithTimeout(10 * time.Second))
			ctx := context.Background()
			result, err := Get(ctx, server.URL, client)

			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestGet_Errors(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *httptest.Server
		checkError  func(*testing.T, error)
	}{
		{
			name: "returns error for non-200 status code",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("Internal Server Error"))
				}))
			},
			checkError: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrHTTPRequestFailed), "should wrap ErrHTTPRequestFailed")
				assert.Contains(t, err.Error(), "returned status 500")
			},
		},
		{
			name: "returns error for 404 not found",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte("Not Found"))
				}))
			},
			checkError: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrHTTPRequestFailed))
				assert.Contains(t, err.Error(), "returned status 404")
			},
		},
		{
			name: "returns error for 401 unauthorized",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte("Unauthorized"))
				}))
			},
			checkError: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrHTTPRequestFailed))
				assert.Contains(t, err.Error(), "returned status 401")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			client := NewDefaultClient(WithTimeout(10 * time.Second))
			ctx := context.Background()
			_, err := Get(ctx, server.URL, client)

			tt.checkError(t, err)
		})
	}
}

func TestGet_InvalidURL(t *testing.T) {
	client := NewDefaultClient(WithTimeout(10 * time.Second))
	ctx := context.Background()

	_, err := Get(ctx, "://invalid-url", client)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrHTTPRequestFailed))
}

func TestGet_ContextCancellation(t *testing.T) {
	// Create a server that delays response.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("delayed response"))
	}))
	defer server.Close()

	// Create context that cancels immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewDefaultClient(WithTimeout(10 * time.Second))
	_, err := Get(ctx, server.URL, client)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, errUtils.ErrHTTPRequestFailed))
}

func TestGet_Timeout(t *testing.T) {
	// Create a server that delays response beyond timeout.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("delayed response"))
	}))
	defer server.Close()

	// Create client with very short timeout.
	client := NewDefaultClient(WithTimeout(10 * time.Millisecond))
	ctx := context.Background()
	_, err := Get(ctx, server.URL, client)

	assert.Error(t, err)
}

func TestGet_ReadBodyError(t *testing.T) {
	// Create a server that sends invalid Content-Length.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusOK)
		// Write less data than Content-Length claims.
		_, _ = w.Write([]byte("short"))
	}))
	defer server.Close()

	client := NewDefaultClient(WithTimeout(10 * time.Second))
	ctx := context.Background()

	// This will fail because Content-Length is 10 but only 5 bytes are sent.
	_, err := Get(ctx, server.URL, client)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrHTTPRequestFailed))
}

func TestGet_LargeResponse(t *testing.T) {
	// Create a server that sends a large response.
	largeBody := strings.Repeat("a", 1024*1024) // 1MB.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeBody))
	}))
	defer server.Close()

	client := NewDefaultClient(WithTimeout(10 * time.Second))
	ctx := context.Background()
	result, err := Get(ctx, server.URL, client)

	require.NoError(t, err)
	assert.Equal(t, len(largeBody), len(result))
}

func TestGet_Headers(t *testing.T) {
	// Verify that requests have expected headers.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that standard headers are present.
		assert.NotEmpty(t, r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := NewDefaultClient(WithTimeout(10 * time.Second))
	ctx := context.Background()
	_, err := Get(ctx, server.URL, client)

	require.NoError(t, err)
}

func TestGet_MultipleRequests(t *testing.T) {
	// Test that client can handle multiple sequential requests.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response"))
	}))
	defer server.Close()

	client := NewDefaultClient(WithTimeout(10 * time.Second))
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		result, err := Get(ctx, server.URL, client)
		require.NoError(t, err)
		assert.Equal(t, []byte("response"), result)
	}
}

// mockHTTPClient is a mock implementation for testing error scenarios.
type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

func TestGet_HTTPClientDoError(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("network error")
		},
	}

	ctx := context.Background()
	_, err := Get(ctx, "http://example.com", mockClient)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrHTTPRequestFailed))
}

func TestGet_ReadBodyIOError(t *testing.T) {
	// Create a mock response with a reader that fails.
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(&errorReader{}),
			}, nil
		},
	}

	ctx := context.Background()
	_, err := Get(ctx, "http://example.com", mockClient)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrHTTPRequestFailed))
}

// errorReader always returns an error on Read.
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("read error")
}

func TestWithTransport(t *testing.T) {
	// Create a mock transport that records requests.
	var capturedReq *http.Request
	mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	// Create client with custom transport.
	client := NewDefaultClient(WithTransport(mockTransport))
	assert.NotNil(t, client)

	// Make a request to verify custom transport is used.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotNil(t, capturedReq, "mock transport should have captured the request")
	assert.Equal(t, "http://example.com/test", capturedReq.URL.String())
}

// TestGitHubAuthenticatedTransport_NilBase verifies that when Base transport is nil,
// the GitHubAuthenticatedTransport falls back to http.DefaultTransport.
func TestGitHubAuthenticatedTransport_NilBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	transport := &GitHubAuthenticatedTransport{
		Base:        nil, // Explicitly set to nil - should fall back to http.DefaultTransport.
		GitHubToken: "",
	}

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestGetGitHubTokenFromEnv_ViperPrecedence verifies that the viper value takes
// precedence over environment variables.
func TestGetGitHubTokenFromEnv_ViperPrecedence(t *testing.T) {
	t.Setenv("ATMOS_GITHUB_TOKEN", "env-token")
	t.Setenv("GITHUB_TOKEN", "fallback-token")

	// Use an isolated viper instance to avoid mutating the global singleton.
	// This prevents BindEnv from leaking env-var mappings into subsequent tests.
	v := viper.New()
	_ = v.BindEnv("github-token", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN")

	// Override viper to simulate --github-token flag.
	v.Set("github-token", "viper-token")

	got := GetGitHubTokenFromEnv(v)
	assert.Equal(t, "viper-token", got)
}

// TestGetGitHubTokenFromEnv_NilViperFallsBackToOsEnv verifies that passing an explicit
// nil viper instance falls back to the global viper singleton (which has no token binding
// in this test context), and then falls through to the os.Getenv fallback path.
func TestGetGitHubTokenFromEnv_NilViperFallsBackToOsEnv(t *testing.T) {
	t.Setenv("ATMOS_GITHUB_TOKEN", "nil-guard-token")

	// Passing nil must not panic — it must fall back to global viper, which in turn
	// falls back to os.Getenv for ATMOS_GITHUB_TOKEN (since no BindEnv is active here).
	assert.NotPanics(t, func() {
		_ = GetGitHubTokenFromEnv(nil)
	})

	// The token is returned via the os.Getenv("ATMOS_GITHUB_TOKEN") fallback path,
	// not via the global viper key (which is unbound in this test).
	got := GetGitHubTokenFromEnv(nil)
	assert.Equal(t, "nil-guard-token", got)
}

// TestWithTransport_AfterWithGitHubToken verifies that WithTransport applied after
// WithGitHubToken does NOT drop the auth wrapper; the provided transport becomes the
// inner base of the GitHubAuthenticatedTransport.
func TestWithTransport_AfterWithGitHubToken(t *testing.T) {
	var capturedReq *http.Request
	mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	// WithGitHubToken first, then WithTransport.
	client := NewDefaultClient(
		WithGitHubToken("secret-token"),
		WithTransport(mockTransport),
	)

	// Verify the transport chain structure: outer is GitHubAuthenticatedTransport,
	// inner (Base) is the mock roundTripperFunc.
	authTransport, ok := client.client.Transport.(*GitHubAuthenticatedTransport)
	require.True(t, ok, "client transport should be *GitHubAuthenticatedTransport")
	_, baseIsRoundTripper := authTransport.Base.(roundTripperFunc)
	assert.True(t, baseIsRoundTripper, "Base should be a roundTripperFunc (the mockTransport)")
	assert.Equal(t, "secret-token", authTransport.GitHubToken)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/repos/test/repo", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.NotNil(t, capturedReq, "mock transport should have been reached")
	assert.Equal(t, "Bearer secret-token", capturedReq.Header.Get("Authorization"),
		"Authorization header must be set when WithGitHubToken is applied after WithTransport")
}

// TestWithGitHubToken_AfterWithTransport verifies that WithGitHubToken applied after
// WithTransport wraps the custom transport inside the auth layer.
func TestWithGitHubToken_AfterWithTransport(t *testing.T) {
	var capturedReq *http.Request
	mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	// WithTransport first, then WithGitHubToken wraps it.
	client := NewDefaultClient(
		WithTransport(mockTransport),
		WithGitHubToken("secret-token"),
	)

	// Verify the transport chain structure: outer is GitHubAuthenticatedTransport,
	// inner (Base) is the mock roundTripperFunc.
	authTransport, ok := client.client.Transport.(*GitHubAuthenticatedTransport)
	require.True(t, ok, "client transport should be *GitHubAuthenticatedTransport")

	// Structural check: verify Base is the exact mockTransport instance (not just the type).
	// Function values are not == comparable in Go; use reflect.ValueOf().Pointer() to compare
	// the underlying function pointer, which is stable and not affected by interface boxing.
	baseTransport, baseIsRoundTripper := authTransport.Base.(roundTripperFunc)
	require.True(t, baseIsRoundTripper, "Base should be a roundTripperFunc (the mockTransport)")
	assert.Equal(t, reflect.ValueOf(http.RoundTripper(mockTransport)).Pointer(),
		reflect.ValueOf(http.RoundTripper(baseTransport)).Pointer(),
		"Base transport pointer must match the exact mockTransport instance")
	assert.Equal(t, "secret-token", authTransport.GitHubToken)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/repos/test/repo", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.NotNil(t, capturedReq, "mock transport should have been reached")
	assert.Equal(t, "Bearer secret-token", capturedReq.Header.Get("Authorization"),
		"Authorization header must be set when WithGitHubToken is applied after WithTransport")
}

// TestWithTransport_TripleComposition verifies that the last WithTransport call replaces
// the Base of any existing GitHubAuthenticatedTransport, not the auth wrapper itself.
// Applied as: WithTransport(t1) → WithGitHubToken("x") → WithTransport(t2)
// Result: GitHubAuthenticatedTransport{Base: t2, Token: "x"} (t1 is discarded by the second WithTransport).
func TestWithTransport_TripleComposition(t *testing.T) {
	var t1Reached, t2Reached bool

	transport1 := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		t1Reached = true
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("t1"))}, nil
	})

	var capturedReq *http.Request
	transport2 := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		t2Reached = true
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("t2"))}, nil
	})

	// WithTransport(t1), WithGitHubToken wraps t1, then WithTransport(t2) replaces Base with t2.
	client := NewDefaultClient(
		WithTransport(transport1),
		WithGitHubToken("triple-token"),
		WithTransport(transport2),
	)

	// The auth wrapper must still be present (not replaced by the second WithTransport).
	authTransport, ok := client.client.Transport.(*GitHubAuthenticatedTransport)
	require.True(t, ok, "client transport must still be *GitHubAuthenticatedTransport")
	assert.Equal(t, "triple-token", authTransport.GitHubToken)
	// The second WithTransport replaces Base with transport2 (a roundTripperFunc).
	_, baseIsRoundTripper := authTransport.Base.(roundTripperFunc)
	assert.True(t, baseIsRoundTripper, "Base should be transport2 (roundTripperFunc) after second WithTransport")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/repos/test/repo", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Only transport2 must be reached; transport1 was replaced.
	assert.True(t, t2Reached, "transport2 should have been reached")
	assert.False(t, t1Reached, "transport1 should NOT be reached (replaced by transport2)")
	require.NotNil(t, capturedReq)
	assert.Equal(t, "Bearer triple-token", capturedReq.Header.Get("Authorization"),
		"Authorization header must be present after triple composition")
}

// TestWithGitHubToken_MultipleCallsLastWins is a regression test for the multiple
// WithGitHubToken wrappers bug. When two WithGitHubToken calls are composed, the INNER
// (earlier-applied) transport's RoundTrip previously overwrote the OUTER (later-applied)
// transport's Authorization header, causing the wrong token to be sent.
// After the fix (only set Authorization if not already set), the outermost (last-applied)
// token must win.
func TestWithGitHubToken_MultipleCallsLastWins(t *testing.T) {
	var capturedReq *http.Request
	mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	// Apply t1 first, t2 second — t2 is the outermost (last-applied) wrapper.
	// t2's token must win. Before the fix, t1 (inner) would overwrite t2 (outer).
	client := NewDefaultClient(
		WithTransport(mockTransport),
		WithGitHubToken("token-t1"),
		WithGitHubToken("token-t2"),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/repos/test/repo", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.NotNil(t, capturedReq, "mock transport should have been reached")
	// The outermost (last-applied, t2) token must be in the Authorization header.
	// Before the fix, the inner t1 would overwrite: Authorization: Bearer token-t1 (wrong).
	// After the fix: Authorization: Bearer token-t2 (correct).
	assert.Equal(t, "Bearer token-t2", capturedReq.Header.Get("Authorization"),
		"last-applied (outermost) token must win when multiple WithGitHubToken calls are composed")
}

// TestGitHubAuthenticatedTransport_PresetAuthorizationNotClobbered verifies that a
// pre-existing Authorization header on the request is NOT overwritten by the transport.
// This tests the "only set if empty" guard at the transport level.
func TestGitHubAuthenticatedTransport_PresetAuthorizationNotClobbered(t *testing.T) {
	var capturedReq *http.Request
	mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	client := NewDefaultClient(
		WithTransport(mockTransport),
		WithGitHubToken("injected-token"),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/repos/test/repo", nil)
	require.NoError(t, err)
	// Pre-set a caller-supplied Authorization header — must survive the transport.
	req.Header.Set("Authorization", "Bearer preset-token")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.NotNil(t, capturedReq, "mock transport must be reached")
	assert.Equal(t, "Bearer preset-token", capturedReq.Header.Get("Authorization"),
		"transport must not overwrite a pre-set Authorization header on the request")
}

// TestGitHubAuthenticatedTransport_GHES verifies that a GitHub Enterprise Server host
// (specified via GITHUB_API_URL) receives authentication headers.
func TestGitHubAuthenticatedTransport_GHES(t *testing.T) {
	ghesHost := "github.mycorp.example.com"
	ghesURL := "https://" + ghesHost

	// Set GITHUB_API_URL so isGitHubHost recognises the GHES host.
	t.Setenv("GITHUB_API_URL", ghesURL)

	var capturedReq *http.Request
	mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("OK")),
		}, nil
	})

	transport := &GitHubAuthenticatedTransport{
		Base:        mockTransport,
		GitHubToken: "ghes-token",
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ghesURL+"/api/v3/repos/org/repo", nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	require.NotNil(t, capturedReq, "mock transport must be reached")
	assert.Equal(t, "Bearer ghes-token", capturedReq.Header.Get("Authorization"),
		"GHES host from GITHUB_API_URL must receive Authorization header")
	assert.Equal(t, userAgent, capturedReq.Header.Get("User-Agent"))
}

// TestGitHubAuthenticatedTransport_GHES_NegativeSubdomain verifies that a host that
// contains the GHES hostname as a substring (e.g. attacker.github.mycorp.example.com)
// does NOT receive the Authorization header.
func TestGitHubAuthenticatedTransport_GHES_NegativeSubdomain(t *testing.T) {
	// Set GITHUB_API_URL for a specific GHES host.
	t.Setenv("GITHUB_API_URL", "https://github.mycorp.example.com")

	var capturedReq *http.Request
	mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("OK")),
		}, nil
	})

	transport := &GitHubAuthenticatedTransport{
		Base:        mockTransport,
		GitHubToken: "ghes-token",
	}

	// This host is a superdomain of the GHES host — must NOT get auth headers.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://attacker.github.mycorp.example.com/evil", nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.NotNil(t, capturedReq)
	assert.Empty(t, capturedReq.Header.Get("Authorization"),
		"superdomain of GHES host must NOT receive Authorization header")
}

// TestWithGitHubHostMatcher verifies that WithGitHubHostMatcher allows configuring
// a custom host predicate on the GitHubAuthenticatedTransport.
func TestWithGitHubHostMatcher(t *testing.T) {
	var capturedReq *http.Request
	mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("OK")),
		}, nil
	})

	// Create a client with a custom host matcher that allows only our test host.
	client := NewDefaultClient(
		WithTransport(mockTransport),
		WithGitHubToken("custom-token"),
		WithGitHubHostMatcher(func(host string) bool {
			return host == "custom-git.example.com"
		}),
	)

	// Allowed custom host — should get auth headers.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://custom-git.example.com/api/v1/repos", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.NotNil(t, capturedReq)
	assert.Equal(t, "Bearer custom-token", capturedReq.Header.Get("Authorization"),
		"custom host matcher must allow the configured host")

	// Default allowed host (api.github.com) — should NOT get auth with a custom matcher
	// that only allows custom-git.example.com.
	req2, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/repos", nil)
	require.NoError(t, err)

	resp2, err := client.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Empty(t, capturedReq.Header.Get("Authorization"),
		"custom host matcher must override the default allowlist")
}

// TestWithGitHubHostMatcher_Precedence is a table-driven test that validates the
// three-level host-matcher precedence documented in pkg/http/doc.go:
//
//  1. [WithGitHubHostMatcher] — an explicit custom predicate always wins.
//  2. GITHUB_API_URL — when set and [WithGitHubHostMatcher] was NOT applied.
//  3. Built-in allowlist — api.github.com, raw.githubusercontent.com, uploads.github.com.
func TestWithGitHubHostMatcher_Precedence(t *testing.T) {
	// Cannot use t.Parallel() here because subtests call t.Setenv which modifies
	// the process-wide GITHUB_API_URL environment variable.

	cases := []struct {
		name          string
		gitHubAPIURL  string            // GITHUB_API_URL env value ("" = not set)
		customMatcher func(string) bool // nil = don't call WithGitHubHostMatcher
		requestURL    string            // HTTPS URL to test
		wantAuth      bool              // whether Authorization should be injected
	}{
		// ── Level 3: built-in allowlist ──────────────────────────────────────────
		{
			name:       "builtin_api_github_com",
			requestURL: "https://api.github.com/repos",
			wantAuth:   true,
		},
		{
			name:       "builtin_raw_githubusercontent_com",
			requestURL: "https://raw.githubusercontent.com/owner/repo/main/file.go",
			wantAuth:   true,
		},
		{
			name:       "builtin_uploads_github_com",
			requestURL: "https://uploads.github.com/releases/assets",
			wantAuth:   true,
		},
		{
			name:       "builtin_negative_example_com",
			requestURL: "https://example.com/api",
			wantAuth:   false,
		},
		{
			name:       "builtin_negative_github_example_com",
			requestURL: "https://github.example.com/api",
			wantAuth:   false,
		},
		// ── Level 2: GITHUB_API_URL overrides the default allowlist ──────────────
		{
			name:         "github_api_url_adds_ghes_host",
			gitHubAPIURL: "https://github.mycorp.example.com",
			requestURL:   "https://github.mycorp.example.com/api/v3/repos",
			wantAuth:     true,
		},
		{
			name:         "github_api_url_still_allows_builtin",
			gitHubAPIURL: "https://github.mycorp.example.com",
			requestURL:   "https://api.github.com/repos",
			wantAuth:     true,
		},
		{
			name:         "github_api_url_does_not_allow_unrelated_host",
			gitHubAPIURL: "https://github.mycorp.example.com",
			requestURL:   "https://other.example.com/api",
			wantAuth:     false,
		},
		// ── Level 1: WithGitHubHostMatcher overrides GITHUB_API_URL ──────────────
		{
			// Custom matcher for "custom-git.example.com" — GHES host set via env
			// must NOT receive auth because the custom matcher doesn't include it.
			name:         "custom_matcher_overrides_github_api_url",
			gitHubAPIURL: "https://ghes.mycorp.example.com",
			customMatcher: func(host string) bool {
				return host == "custom-git.example.com"
			},
			requestURL: "https://ghes.mycorp.example.com/api/v3/repos",
			wantAuth:   false, // custom matcher wins — GHES host is excluded
		},
		{
			// Custom matcher — host included in custom predicate gets auth.
			name:         "custom_matcher_allows_its_own_host",
			gitHubAPIURL: "https://ghes.mycorp.example.com",
			customMatcher: func(host string) bool {
				return host == "custom-git.example.com"
			},
			requestURL: "https://custom-git.example.com/api",
			wantAuth:   true,
		},
		{
			// Custom matcher replaces the default allowlist too.
			name: "custom_matcher_overrides_builtin_allowlist",
			customMatcher: func(host string) bool {
				return host == "custom-git.example.com"
			},
			requestURL: "https://api.github.com/repos",
			wantAuth:   false, // api.github.com is NOT in the custom matcher
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// No t.Parallel() — subtests may call t.Setenv.

			var capturedReq *http.Request
			mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				capturedReq = req.Clone(req.Context())
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("OK")),
				}, nil
			})

			t.Setenv("GITHUB_API_URL", tc.gitHubAPIURL)

			opts := []ClientOption{
				WithTransport(mockTransport),
				WithGitHubToken("precedence-test-token"),
			}
			if tc.customMatcher != nil {
				opts = append(opts, WithGitHubHostMatcher(tc.customMatcher))
			}
			client := NewDefaultClient(opts...)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tc.requestURL, nil)
			require.NoError(t, err)

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.NotNil(t, capturedReq, "transport must have been invoked")
			got := capturedReq.Header.Get("Authorization")
			if tc.wantAuth {
				assert.Equal(t, "Bearer precedence-test-token", got,
					"[%s] expected Authorization to be set on %s", tc.name, tc.requestURL)
			} else {
				assert.Empty(t, got,
					"[%s] expected Authorization to be absent on %s", tc.name, tc.requestURL)
			}
		})
	}
}

// TestGet_LargeErrorBodyTruncation verifies that when an HTTP server returns a non-2xx
// response with a body larger than maxErrorBodySize, the error message:
//   - contains the "[truncated]" marker
//   - contains the content-type from the response
//   - wraps ErrHTTPRequestFailed
func TestGet_LargeErrorBodyTruncation(t *testing.T) {
	// Build a response body that is one byte larger than the limit.
	// We use a mix of characters to make it identifiable.
	oversizeBody := strings.Repeat("x", maxErrorBodySize+1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprint(w, oversizeBody)
	}))
	defer server.Close()

	client := NewDefaultClient(WithTimeout(10 * time.Second))
	_, err := Get(context.Background(), server.URL, client)

	require.Error(t, err, "a non-2xx response should return an error")
	assert.True(t, errors.Is(err, errUtils.ErrHTTPRequestFailed), "error must wrap ErrHTTPRequestFailed")
	assert.Contains(t, err.Error(), "[truncated]", "error message must contain truncation marker")
	assert.Contains(t, err.Error(), "text/plain", "error message must contain content-type from response")
	assert.Contains(t, err.Error(), "returned status 502", "error message must contain the status code")
}

// TestGet_ErrorBodyContentType verifies that the content-type header value from a
// non-2xx response is correctly reported in the error message when the body fits within
// the truncation limit.
func TestGet_ErrorBodyContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"error":"forbidden"}`)
	}))
	defer server.Close()

	client := NewDefaultClient(WithTimeout(10 * time.Second))
	_, err := Get(context.Background(), server.URL, client)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "application/json", "error must contain the Content-Type header value")
	assert.NotContains(t, err.Error(), "[truncated]", "short body must not be truncated")
}

// TestIsGitHubHost_DefaultAllowlist verifies the default isGitHubHost allowlist
// without any GITHUB_API_URL override.
func TestIsGitHubHost_DefaultAllowlist(t *testing.T) {
	// Ensure GITHUB_API_URL is not set so we test default behavior.
	t.Setenv("GITHUB_API_URL", "")

	assert.True(t, isGitHubHost("api.github.com"))
	assert.True(t, isGitHubHost("raw.githubusercontent.com"))
	assert.True(t, isGitHubHost("uploads.github.com"), "uploads.github.com must be in the default allowlist")

	assert.False(t, isGitHubHost("github.com"))
	assert.False(t, isGitHubHost("example.com"))
	assert.False(t, isGitHubHost("github.example.com"))
	assert.False(t, isGitHubHost("example.github.com"))
	assert.False(t, isGitHubHost(""))
}

// TestIsGitHubHost_GITHUB_API_URL verifies that GITHUB_API_URL adds a GHES host.
func TestIsGitHubHost_GITHUB_API_URL(t *testing.T) {
	t.Setenv("GITHUB_API_URL", "https://github.mycorp.example.com")

	assert.True(t, isGitHubHost("github.mycorp.example.com"), "GITHUB_API_URL hostname should be allowed")
	assert.True(t, isGitHubHost("api.github.com"), "default allowlist still applies")

	// Only exact hostname match, not substring.
	assert.False(t, isGitHubHost("evil.github.mycorp.example.com"))
	assert.False(t, isGitHubHost("github.mycorp.example.com.evil.tld"))
}

// TestIsGitHubHost_InvalidGITHUB_API_URL verifies that an unparsable GITHUB_API_URL
// does not panic and falls back to the default allowlist.
func TestIsGitHubHost_InvalidGITHUB_API_URL(t *testing.T) {
	t.Setenv("GITHUB_API_URL", "://not-a-valid-url")

	// Default allowlist still applies even when GITHUB_API_URL is invalid.
	assert.True(t, isGitHubHost("api.github.com"))
	assert.False(t, isGitHubHost("example.com"))
}

// TestNormalizeHost verifies that normalizeHost canonicalises hostnames correctly.
func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"api.github.com", "api.github.com"},
		{"API.GITHUB.COM", "api.github.com"},
		{"Api.GitHub.Com", "api.github.com"},
		// Trailing dot (FQDN form).
		{"api.github.com.", "api.github.com"},
		// Upper-case + trailing dot.
		{"API.GITHUB.COM.", "api.github.com"},
		{"", ""},
		// Default port 443 should be stripped.
		{"api.github.com:443", "api.github.com"},
		// Default port 80 should be stripped.
		{"api.github.com:80", "api.github.com"},
		// Non-default port should be preserved.
		{"api.github.com:8443", "api.github.com:8443"},
		// Port 443 + upper-case: both normalised.
		{"API.GITHUB.COM:443", "api.github.com"},
		// Port 443 + trailing dot: trailing dot stripped then port stripped.
		{"api.github.com.:443", "api.github.com"},
		// IPv6 with default port: brackets are stripped by net.SplitHostPort.
		{"[::1]:443", "::1"},
		// IPv6 with non-default port: preserved (with brackets stripped by SplitHostPort).
		{"[::1]:8080", "[::1]:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeHost(tt.input))
		})
	}
}

// TestIsGitHubHost_CaseAndTrailingDot verifies that isGitHubHost tolerates case
// variations and trailing dots in the host parameter.
func TestIsGitHubHost_CaseAndTrailingDot(t *testing.T) {
	t.Setenv("GITHUB_API_URL", "")

	positives := []string{
		"API.GITHUB.COM",
		"api.github.com.",
		"API.GITHUB.COM.",
		"Raw.GitHubUserContent.com",
		"UPLOADS.GITHUB.COM",
		// Port variants: default port should be stripped before matching.
		"api.github.com:443",
		"API.GITHUB.COM:443",
		"uploads.github.com:443",
		"raw.githubusercontent.com:80",
	}
	for _, h := range positives {
		assert.True(t, isGitHubHost(h), "expected %q to be allowed", h)
	}

	negatives := []string{
		"GITHUB.EXAMPLE.COM",
		"EXAMPLE.GITHUB.COM",
		"github.com",
		// Port variants on disallowed hosts should still be denied.
		"github.example.com:443",
		"example.github.com:443",
	}
	for _, h := range negatives {
		assert.False(t, isGitHubHost(h), "expected %q to be denied", h)
	}
}

// TestGitHubAuthenticatedTransport_CrossHostRedirect verifies that Authorization is
// NOT forwarded when the http.Client follows a redirect to a different host.
// The transport only adds auth per-hop for allowed hosts; this test also verifies
// that the CheckRedirect installed by WithGitHubToken strips any stale Authorization.
func TestGitHubAuthenticatedTransport_CrossHostRedirect(t *testing.T) {
	// Target server — asserts Authorization is NOT present.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"),
			"Authorization must not be forwarded to the redirect target host")
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	// Origin server — redirects to target (different host).
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/landed", http.StatusFound)
	}))
	defer origin.Close()

	// Build a client with a GitHub token.  The origin and target are both plain HTTP
	// httptest servers, so Authorization will NOT be added by the transport anyway
	// (HTTPS-only rule).  But we still verify that CheckRedirect is wired up and
	// does not add the header on the redirect leg.
	client := NewDefaultClient(
		WithGitHubToken("test-redir-token"),
	)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, origin.URL+"/start", nil)
	require.NoError(t, err)

	// Manually add an Authorization header to simulate a caller that pre-set it.
	req.Header.Set("Authorization", "Bearer manual-token")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestGitHubAuthenticatedTransport_AllRedirectStatusCodes verifies that Authorization
// is stripped when the http.Client follows any of the standard redirect status codes
// (301 MovedPermanently, 302 Found, 303 SeeOther, 307 TemporaryRedirect, 308 PermanentRedirect)
// to a different host.
func TestGitHubAuthenticatedTransport_AllRedirectStatusCodes(t *testing.T) {
	t.Parallel()

	redirectCases := []struct {
		code int
		name string
	}{
		{http.StatusMovedPermanently, "301_MovedPermanently"},
		{http.StatusFound, "302_Found"},
		{http.StatusSeeOther, "303_SeeOther"},
		{http.StatusTemporaryRedirect, "307_TemporaryRedirect"},
		{http.StatusPermanentRedirect, "308_PermanentRedirect"},
	}

	for _, rc := range redirectCases {
		t.Run(rc.name, func(t *testing.T) {
			t.Parallel()

			// Target: a different host that must NOT receive Authorization.
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Empty(t, r.Header.Get("Authorization"),
					"Authorization must not be forwarded after %d redirect", rc.code)
				w.WriteHeader(http.StatusOK)
			}))
			defer target.Close()

			// 307 and 308 require the same method and body so we use a GET to keep it simple.
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL+"/landed", rc.code)
			}))
			defer origin.Close()

			client := NewDefaultClient(
				WithGitHubToken("redir-test-token"),
			)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, origin.URL+"/start", nil)
			require.NoError(t, err)
			// Pre-set Authorization to simulate a caller that added it manually.
			req.Header.Set("Authorization", "Bearer manual-token")

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode,
				"final response after %d redirect must be 200 OK", rc.code)
		})
	}
}

// TestStripAuthOnCrossHostRedirect_RedirectLimitExceeded verifies that CheckRedirect
// returns ErrRedirectLimitExceeded when a redirect chain reaches 10 hops.
func TestStripAuthOnCrossHostRedirect_RedirectLimitExceeded(t *testing.T) {
	// Build a server that always redirects back to itself (infinite loop).
	// After 10 hops our CheckRedirect (stripAuthOnCrossHostRedirect) returns
	// ErrRedirectLimitExceeded and the http.Client propagates the error.
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/loop", http.StatusFound)
	}))
	defer server.Close()

	client := NewDefaultClient(WithGitHubToken("test-token"))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrRedirectLimitExceeded),
		"redirect chain >= 10 must return ErrRedirectLimitExceeded, got: %v", err)
}

// TestGitHubAuthenticatedTransport_RoundTripError verifies that an error returned by
// the base transport is wrapped with the "GitHub transport roundtrip" prefix.
func TestGitHubAuthenticatedTransport_RoundTripError(t *testing.T) {
	baseErr := fmt.Errorf("connection refused")
	failTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, baseErr
	})

	transport := &GitHubAuthenticatedTransport{
		Base:        failTransport,
		GitHubToken: "token",
	}

	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos", nil)
	require.NoError(t, err)

	_, err = transport.RoundTrip(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GitHub transport roundtrip",
		"transport error must be wrapped with the 'GitHub transport roundtrip' prefix")
}

// TestGet_ErrorBodyReadFails verifies that when reading a non-2xx response body fails,
// the error is wrapped with ErrHTTPRequestFailed and mentions "failed to read error body".
func TestGet_ErrorBodyReadFails(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(&errorReader{}),
				Header:     http.Header{},
			}, nil
		},
	}

	ctx := context.Background()
	_, err := Get(ctx, "http://example.com", mockClient)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrHTTPRequestFailed),
		"error must wrap ErrHTTPRequestFailed")
	assert.Contains(t, err.Error(), "failed to read error body",
		"error must describe the read failure")
}
