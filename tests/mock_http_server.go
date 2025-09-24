package tests

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
)

var mockServer *httptest.Server

// StartMockHTTPServer starts a simple file server for remote-config scenario.
func StartMockHTTPServer() string {
	if mockServer != nil {
		return mockServer.URL
	}

	// Serve the remote-config scenario directory
	// Use absolute path to ensure it works even if the working directory changes
	remoteConfigPath, err := filepath.Abs(filepath.Join("fixtures", "scenarios", "remote-config"))
	if err != nil {
		// Fall back to relative path if absolute path fails
		remoteConfigPath = filepath.Join("fixtures", "scenarios", "remote-config")
	}
	fileServer := http.FileServer(http.Dir(remoteConfigPath))

	mockServer = httptest.NewServer(fileServer)

	return mockServer.URL
}

// GetMockServerURL returns the current mock server URL.
func GetMockServerURL() string {
	if mockServer != nil {
		return mockServer.URL
	}
	return ""
}

// StopMockHTTPServer stops the mock HTTP server.
func StopMockHTTPServer() {
	if mockServer != nil {
		mockServer.Close()
		mockServer = nil
	}
}
