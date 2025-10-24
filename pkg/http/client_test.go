package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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
	// Save original values
	originalAtmosToken := os.Getenv("ATMOS_GITHUB_TOKEN")
	originalGitHubToken := os.Getenv("GITHUB_TOKEN")
	defer func() {
		os.Setenv("ATMOS_GITHUB_TOKEN", originalAtmosToken)
		os.Setenv("GITHUB_TOKEN", originalGitHubToken)
	}()

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
			os.Setenv("ATMOS_GITHUB_TOKEN", tt.atmosToken)
			os.Setenv("GITHUB_TOKEN", tt.githubToken)

			got := GetGitHubTokenFromEnv()
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
				assert.Contains(t, err.Error(), "unexpected status code: 500")
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
				assert.Contains(t, err.Error(), "unexpected status code: 404")
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
				assert.Contains(t, err.Error(), "unexpected status code: 401")
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
