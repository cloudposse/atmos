package exec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDockerCredHelpers tests Docker credential helper authentication
func TestDockerCredHelpers(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Docker Hub with desktop credential helper",
			registry:    "docker.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true, // Will fail without actual credential helper
			errorMsg:    "credential helper docker-credential-desktop not found",
		},
		{
			name:        "Private registry with desktop credential helper",
			registry:    "my-registry.com",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true,
			errorMsg:    "credential helper docker-credential-desktop not found",
		},
		{
			name:        "Registry with non-existent credential helper",
			registry:    "test.registry.com",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true,
			errorMsg:    "credential helper docker-credential-nonexistent not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getDockerAuth(tt.registry, tt.atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDecodeDockerAuth tests Docker auth string decoding
func TestDecodeDockerAuth(t *testing.T) {
	tests := []struct {
		name         string
		authString   string
		expectError  bool
		errorMsg     string
		expectedUser string
		expectedPass string
	}{
		{
			name:         "Valid auth string",
			authString:   "dXNlcm5hbWU6cGFzc3dvcmQ=", // "username:password" in base64
			expectError:  false,
			expectedUser: "username",
			expectedPass: "password",
		},
		{
			name:        "Invalid base64 string",
			authString:  "invalid-base64",
			expectError: true,
			errorMsg:    "failed to decode base64 auth string",
		},
		{
			name:        "Invalid format (no colon)",
			authString:  "dXNlcm5hbWU=", // "username" in base64
			expectError: true,
			errorMsg:    "invalid auth string format",
		},
		{
			name:        "Empty string",
			authString:  "",
			expectError: true,
			errorMsg:    "invalid auth string format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, password, err := decodeDockerAuth(tt.authString)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedUser, username)
				assert.Equal(t, tt.expectedPass, password)
			}
		})
	}
}

// TestGetCredentialStoreAuth tests credential store authentication
func TestGetCredentialStoreAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		credsStore  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Empty credential store name",
			registry:    "docker.io",
			credsStore:  "",
			expectError: true,
			errorMsg:    "credential helper docker-credential- not found",
		},
		{
			name:        "Non-existent credential helper",
			registry:    "docker.io",
			credsStore:  "nonexistent",
			expectError: true,
			errorMsg:    "credential helper docker-credential-nonexistent not found",
		},
		{
			name:        "Invalid registry name with command injection attempt",
			registry:    "registry; rm -rf /",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name with shell metacharacters",
			registry:    "registry&echo hello",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name with backticks",
			registry:    "registry`whoami`",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name with dollar expansion",
			registry:    "registry$HOME",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name with parentheses",
			registry:    "registry$(whoami)",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name with brackets",
			registry:    "registry[test]",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name with quotes",
			registry:    "registry'test'",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name with newlines",
			registry:    "registry\ntest",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Valid registry name - standard domain",
			registry:    "docker.io",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "failed to get credentials from store",
		},
		{
			name:        "Valid registry name - with port",
			registry:    "registry.com:5000",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "failed to get credentials from store",
		},
		{
			name:        "Valid registry name - with path",
			registry:    "registry.com/path",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "failed to get credentials from store",
		},
		{
			name:        "Valid registry name - with hyphens",
			registry:    "my-registry.com",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "failed to get credentials from store",
		},
		{
			name:        "Valid registry name - IP address",
			registry:    "192.168.1.100:5000",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "failed to get credentials from store",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getCredentialStoreAuth(tt.registry, tt.credsStore)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper function to create temporary files for testing
func createTempFile(t *testing.T, content string) string {
	tmpfile, err := os.CreateTemp("", "docker-config-*.json")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}

	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	return tmpfile.Name()
}
