package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	// Function values are not == comparable in Go; verify identity by checking that Base
	// is a roundTripperFunc AND that its pointer matches mockTransport's pointer.
	baseTransport, baseIsRoundTripper := authTransport.Base.(roundTripperFunc)
	require.True(t, baseIsRoundTripper, "Base should be a roundTripperFunc (the mockTransport)")
	assert.Equal(t, fmt.Sprintf("%p", http.RoundTripper(mockTransport)), fmt.Sprintf("%p", http.RoundTripper(baseTransport)),
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
