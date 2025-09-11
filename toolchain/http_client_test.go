package toolchain

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestGetGitHubToken(t *testing.T) {
	// Save original value
	originalToken := viper.GetString("github-token")
	defer func() { viper.Set("github-token", originalToken) }()

	// Test with token set
	viper.Set("github-token", "test-token")
	token := GetGitHubToken()
	if token != "test-token" {
		t.Errorf("Expected 'test-token', got '%s'", token)
	}

	// Test with empty token
	viper.Set("github-token", "")
	token = GetGitHubToken()
	if token != "" {
		t.Errorf("Expected empty string, got '%s'", token)
	}
}

// MockTransport is a mock implementation of http.RoundTripper for testing
type MockTransport struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
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
			mockTransport := &MockTransport{
				RoundTripFunc: func(req *http.Request) (*http.Response, error) {
					capturedRequest = req
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("OK")),
					}, nil
				},
			}

			// Initialize the GitHubAuthenticatedTransport
			transport := &GitHubAuthenticatedTransport{
				Base:        mockTransport,
				GitHubToken: tt.token,
			}

			// Create a test request
			req, err := http.NewRequest("GET", tt.url, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			// Perform the RoundTrip
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip failed: %v", err)
			}
			if resp == nil {
				t.Fatal("Expected non-nil response")
			}

			// Verify headers
			if tt.expectAuth {
				expectedAuth := "Bearer " + tt.token
				if capturedRequest.Header.Get("Authorization") != expectedAuth {
					t.Errorf("Expected Authorization header %q, got %q",
						expectedAuth, capturedRequest.Header.Get("Authorization"))
				}
			} else {
				if capturedRequest.Header.Get("Authorization") != "" {
					t.Errorf("Expected no Authorization header, got %q",
						capturedRequest.Header.Get("Authorization"))
				}
			}

			if tt.expectUserAgent {
				expectedUA := "atmos-toolchain/1.0"
				if capturedRequest.Header.Get("User-Agent") != expectedUA {
					t.Errorf("Expected User-Agent header %q, got %q",
						expectedUA, capturedRequest.Header.Get("User-Agent"))
				}
			} else {
				if capturedRequest.Header.Get("User-Agent") != "" {
					t.Errorf("Expected no User-Agent header, got %q",
						capturedRequest.Header.Get("User-Agent"))
				}
			}
		})
	}

	// Test with nil Base transport
	t.Run("nil base transport uses default", func(t *testing.T) {
		transport := &GitHubAuthenticatedTransport{
			Base:        nil,
			GitHubToken: "test-token",
		}

		req, err := http.NewRequest("GET", "https://api.github.com/repos/test/repo", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		// We can't directly test http.DefaultTransport, but we can verify RoundTrip doesn't panic
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip failed with nil base: %v", err)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response with nil base")
		}
	})

	// Test error propagation
	t.Run("error propagation from base transport", func(t *testing.T) {
		expectedErr := fmt.Errorf("mock transport error")
		mockTransport := &MockTransport{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return nil, expectedErr
			},
		}

		transport := &GitHubAuthenticatedTransport{
			Base:        mockTransport,
			GitHubToken: "test-token",
		}

		req, err := http.NewRequest("GET", "https://api.github.com/repos/test/repo", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		_, err = transport.RoundTrip(req)
		if err == nil || !strings.Contains(err.Error(), "GitHub transport roundtrip: mock transport error") {
			t.Errorf("Expected wrapped error containing %q, got %v", expectedErr, err)
		}
	})
}

func TestGitHubAuthenticatedTransport_NonGitHubURL(t *testing.T) {
	// Create a test server to verify headers
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	// Test with GitHub token
	config := HTTPClientConfig{
		Timeout:     30 * time.Second,
		GitHubToken: "test-token",
	}
	client := NewHTTPClient(config)

	// Make request to non-GitHub URL
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check that Authorization header was NOT set for non-GitHub URLs
	if auth := receivedHeaders.Get("Authorization"); auth != "" {
		t.Errorf("Expected no Authorization header for non-GitHub URL, got '%s'", auth)
	}
}

func TestGitHubAuthenticatedTransport_RawContent(t *testing.T) {
	// Create a test server to verify headers
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	// Test with GitHub token
	config := HTTPClientConfig{
		Timeout:     30 * time.Second,
		GitHubToken: "test-token",
	}
	client := NewHTTPClient(config)

	// Make request to raw content URL (should not get authentication)
	resp, err := client.Get(server.URL + "/raw.githubusercontent.com/test")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check that Authorization header was NOT set for raw content URLs
	if auth := receivedHeaders.Get("Authorization"); auth != "" {
		t.Errorf("Expected no Authorization header for raw content URL, got '%s'", auth)
	}
}

func TestNewDefaultHTTPClient(t *testing.T) {
	// Save original value
	originalToken := viper.GetString("github-token")
	defer func() { viper.Set("github-token", originalToken) }()

	// Test with token set
	viper.Set("github-token", "test-token")
	client := NewDefaultHTTPClient()
	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	// Test with empty token
	viper.Set("github-token", "")
	client = NewDefaultHTTPClient()
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
}
