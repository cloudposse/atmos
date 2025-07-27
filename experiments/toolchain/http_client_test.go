package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestGetGitHubToken(t *testing.T) {
	// Save original value
	originalToken := githubToken
	defer func() { githubToken = originalToken }()

	// Test with token set
	githubToken = "test-token"
	token := GetGitHubToken()
	if token != "test-token" {
		t.Errorf("Expected 'test-token', got '%s'", token)
	}

	// Test with empty token
	githubToken = ""
	token = GetGitHubToken()
	if token != "" {
		t.Errorf("Expected empty string, got '%s'", token)
	}
}

func TestGitHubAuthenticatedTransport(t *testing.T) {
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

	// Make request to a URL that contains "api.github.com" to trigger authentication
	resp, err := client.Get(server.URL + "/api.github.com/test")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check that Authorization header was set
	if auth := receivedHeaders.Get("Authorization"); auth != "Bearer test-token" {
		t.Errorf("Expected Authorization header 'Bearer test-token', got '%s'", auth)
	}

	// Check that User-Agent was set
	if userAgent := receivedHeaders.Get("User-Agent"); userAgent != "atmos-toolchain/1.0" {
		t.Errorf("Expected User-Agent header 'atmos-toolchain/1.0', got '%s'", userAgent)
	}
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

	// Make request to raw.githubusercontent.com (should NOT get authentication)
	resp, err := client.Get(server.URL + "/raw.githubusercontent.com/test")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check that Authorization header was NOT set for raw content
	if auth := receivedHeaders.Get("Authorization"); auth != "" {
		t.Errorf("Expected no Authorization header for raw content URL, got '%s'", auth)
	}
}

func TestNewDefaultHTTPClient(t *testing.T) {
	// Set a test token
	os.Setenv("ATMOS_GITHUB_TOKEN", "test-token")
	defer os.Unsetenv("ATMOS_GITHUB_TOKEN")

	client := NewDefaultHTTPClient()
	if client == nil {
		t.Fatal("Expected non-nil HTTP client")
	}

	// Verify timeout is set (30 seconds)
	if client.Timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", client.Timeout)
	}
}
