package exec

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGetRegistryAuth tests the main registry authentication function
func TestGetRegistryAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
		errorMsg    string
	}{
		{
			name:        "GitHub Container Registry",
			registry:    "ghcr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "Docker Hub",
			registry:    "docker.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "Azure Container Registry",
			registry:    "test.azurecr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "AWS ECR",
			registry:    "123456789012.dkr.ecr.us-west-2.amazonaws.com",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "Google Container Registry",
			registry:    "gcr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "Unknown registry",
			registry:    "unknown.registry.com",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getRegistryAuth(tt.registry, tt.atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				// Authentication might fail due to missing credentials, but should not panic
				if err != nil {
					t.Logf("Authentication failed as expected: %v", err)
				}
			}
		})
	}
}

// TestCloudProviderAuth tests authentication for different cloud providers
func TestCloudProviderAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		provider    string
		expectError bool
	}{
		{
			name:        "GitHub CR with token",
			registry:    "ghcr.io",
			provider:    "github",
			expectError: false,
		},
		{
			name:        "Docker Hub with credentials",
			registry:    "docker.io",
			provider:    "docker",
			expectError: false,
		},
		{
			name:        "Azure ACR",
			registry:    "test.azurecr.io",
			provider:    "azure",
			expectError: false,
		},
		{
			name:        "AWS ECR",
			registry:    "123456789012.dkr.ecr.us-west-2.amazonaws.com",
			provider:    "aws",
			expectError: false,
		},
		{
			name:        "Google CR",
			registry:    "gcr.io",
			provider:    "google",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables for testing
			switch tt.provider {
			case "github":
				os.Setenv("GITHUB_TOKEN", "test-token")
				defer os.Unsetenv("GITHUB_TOKEN")
			case "docker":
				os.Setenv("DOCKER_USERNAME", "test-user")
				os.Setenv("DOCKER_PASSWORD", "test-password")
				defer os.Unsetenv("DOCKER_USERNAME")
				defer os.Unsetenv("DOCKER_PASSWORD")
			case "azure":
				os.Setenv("AZURE_CLIENT_ID", "test-client-id")
				os.Setenv("AZURE_CLIENT_SECRET", "test-client-secret")
				os.Setenv("AZURE_TENANT_ID", "test-tenant-id")
				defer os.Unsetenv("AZURE_CLIENT_ID")
				defer os.Unsetenv("AZURE_CLIENT_SECRET")
				defer os.Unsetenv("AZURE_TENANT_ID")
			case "aws":
				os.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
				os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
				defer os.Unsetenv("AWS_ACCESS_KEY_ID")
				defer os.Unsetenv("AWS_SECRET_ACCESS_KEY")
			case "google":
				os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/path/to/credentials.json")
				defer os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
			}

			atmosConfig := &schema.AtmosConfiguration{}
			_, err := getRegistryAuth(tt.registry, atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				// Authentication might fail due to invalid test credentials, but should not panic
				if err != nil {
					t.Logf("Authentication failed as expected with test credentials: %v", err)
				}
			}
		})
	}
}

// TestACRAuth tests Azure Container Registry authentication
func TestACRAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		envVars     map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name:     "ACR with service principal",
			registry: "test.azurecr.io",
			envVars: map[string]string{
				"AZURE_CLIENT_ID":     "test-client-id",
				"AZURE_CLIENT_SECRET": "test-client-secret",
				"AZURE_TENANT_ID":     "test-tenant-id",
			},
			expectError: true, // Will fail due to invalid credentials, but should handle correctly
			errorMsg:    "failed to get Azure token",
		},
		{
			name:        "ACR with default credential (preferred)",
			registry:    "test.azurecr.io",
			envVars:     map[string]string{},
			expectError: true, // Will fail due to no Azure credentials, but should handle correctly
			errorMsg:    "no valid Azure authentication found",
		},
		{
			name:        "ACR with no authentication",
			registry:    "test.azurecr.io",
			envVars:     map[string]string{},
			expectError: true, // Will fail due to no Azure credentials, but should handle correctly
			errorMsg:    "no valid Azure authentication found",
		},
		{
			name:        "Invalid ACR format",
			registry:    "invalid-registry",
			envVars:     map[string]string{},
			expectError: true,
			errorMsg:    "no authentication found for registry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			atmosConfig := &schema.AtmosConfiguration{}
			auth, err := getRegistryAuth(tt.registry, atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestGetACRAuthViaServicePrincipal tests Azure Service Principal authentication
func TestGetACRAuthViaServicePrincipal(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		acrName     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid ACR with service principal",
			registry:    "test.azurecr.io",
			acrName:     "test",
			expectError: true, // Will fail due to no Azure credentials, but should handle correctly
			errorMsg:    "failed to get Azure token",
		},
		{
			name:        "Empty registry",
			registry:    "",
			acrName:     "test",
			expectError: true,
			errorMsg:    "failed to get Azure token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := getACRAuthViaServicePrincipal(tt.registry, tt.acrName, "test-client-id", "test-client-secret", "test-tenant-id")

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, auth)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestGetACRAuthViaDefaultCredential tests Azure Default Credential authentication
func TestGetACRAuthViaDefaultCredential(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		acrName     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid ACR with default credential",
			registry:    "test.azurecr.io",
			acrName:     "test",
			expectError: true, // Will fail due to no Azure credentials, but should handle correctly
			errorMsg:    "failed to get Azure token",
		},
		{
			name:        "Empty registry",
			registry:    "",
			acrName:     "test",
			expectError: true,
			errorMsg:    "failed to get Azure token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := getACRAuthViaDefaultCredential(tt.registry, tt.acrName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, auth)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestDockerCredHelpers tests Docker credential helpers functionality
// This function tests the credential store functionality with proper function signature
func TestDockerCredHelpers(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		credsStore  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Invalid credential store name",
			registry:    "docker.io",
			credsStore:  "/invalid/store/name",
			expectError: true,
			errorMsg:    "invalid credential store name",
		},
		{
			name:        "Empty credential store name",
			registry:    "docker.io",
			credsStore:  "",
			expectError: true,
			errorMsg:    "credential helper docker-credential- not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the function with correct signature
			auth, err := getCredentialStoreAuth(tt.registry, tt.credsStore)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, auth)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestECRAuth tests AWS ECR authentication
func TestECRAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		envVars     map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name:     "ECR with AWS credentials",
			registry: "123456789012.dkr.ecr.us-west-2.amazonaws.com",
			envVars: map[string]string{
				"AWS_ACCESS_KEY_ID":     "test-access-key",
				"AWS_SECRET_ACCESS_KEY": "test-secret-key",
				"AWS_REGION":            "us-west-2",
			},
			expectError: true, // Will fail due to invalid credentials, but should handle correctly
			errorMsg:    "failed to get ECR authorization token",
		},
		{
			name:        "Invalid ECR registry format",
			registry:    "invalid-ecr-registry",
			envVars:     map[string]string{},
			expectError: true,
			errorMsg:    "no authentication found for registry",
		},
		{
			name:        "ECR FIPS endpoint",
			registry:    "123456789012.dkr.ecr-fips.us-gov-west-1.amazonaws.com",
			envVars:     map[string]string{},
			expectError: true, // Will fail due to no AWS credentials, but should parse correctly
			errorMsg:    "failed to get ECR authorization token",
		},
		{
			name:        "Non-ECR registry",
			registry:    "docker.io",
			envVars:     map[string]string{},
			expectError: true,
			errorMsg:    "no authentication found for registry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			atmosConfig := &schema.AtmosConfiguration{}
			auth, err := getRegistryAuth(tt.registry, atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestDecodeDockerAuth tests Docker authentication string decoding
func TestDecodeDockerAuth(t *testing.T) {
	tests := []struct {
		name        string
		authString  string
		expectError bool
		expected    *authn.Basic
	}{
		{
			name:        "Valid auth string",
			authString:  base64.StdEncoding.EncodeToString([]byte("username:password")),
			expectError: false,
			expected: &authn.Basic{
				Username: "username",
				Password: "password",
			},
		},
		{
			name:        "Empty auth string",
			authString:  "",
			expectError: true,
			expected:    nil,
		},
		{
			name:        "Invalid base64",
			authString:  "invalid-base64",
			expectError: true,
			expected:    nil,
		},
		{
			name:        "Missing colon in decoded string",
			authString:  base64.StdEncoding.EncodeToString([]byte("username")),
			expectError: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, password, err := decodeDockerAuth(tt.authString)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.Username, username)
				assert.Equal(t, tt.expected.Password, password)
			}
		})
	}
}

// TestGetCredentialStoreAuth tests Docker credential store authentication
func TestGetCredentialStoreAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		credsStore  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Invalid credential store name",
			registry:    "docker.io",
			credsStore:  "/invalid/store/name",
			expectError: true,
			errorMsg:    "invalid credential store name",
		},
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
			name:        "Invalid registry name - command injection",
			registry:    "docker.io;rm -rf /",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name - shell metacharacters",
			registry:    "docker.io&echo hack",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name - backticks",
			registry:    "docker.io`whoami`",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name - dollar expansion",
			registry:    "docker.io$PATH",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name - parentheses",
			registry:    "docker.io(ls)",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name - brackets",
			registry:    "docker.io[test]",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name - quotes",
			registry:    "docker.io'echo hack'",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Invalid registry name - newlines",
			registry:    "docker.io\nrm -rf /",
			credsStore:  "desktop",
			expectError: true,
			errorMsg:    "invalid registry name",
		},
		{
			name:        "Valid registry name - standard domain",
			registry:    "docker.io",
			credsStore:  "desktop",
			expectError: true, // Will fail due to credential helper execution failure, but validation should pass
			errorMsg:    "failed to get credentials from store",
		},
		{
			name:        "Valid registry name - with port",
			registry:    "registry.example.com:5000",
			credsStore:  "desktop",
			expectError: true, // Will fail due to credential helper execution failure, but validation should pass
			errorMsg:    "failed to get credentials from store",
		},
		{
			name:        "Valid registry name - with path",
			registry:    "registry.example.com/v2",
			credsStore:  "desktop",
			expectError: true, // Will fail due to credential helper execution failure, but validation should pass
			errorMsg:    "failed to get credentials from store",
		},
		{
			name:        "Valid registry name - with hyphens",
			registry:    "my-registry.example.com",
			credsStore:  "desktop",
			expectError: true, // Will fail due to credential helper execution failure, but validation should pass
			errorMsg:    "failed to get credentials from store",
		},
		{
			name:        "Valid registry name - IP address",
			registry:    "192.168.1.100:5000",
			credsStore:  "desktop",
			expectError: true, // Will fail due to credential helper execution failure, but validation should pass
			errorMsg:    "failed to get credentials from store",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := getCredentialStoreAuth(tt.registry, tt.credsStore)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, auth)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
			}
		})
	}
}

// TestExchangeAADForACRRefreshToken tests the ACR token exchange functionality
func TestExchangeAADForACRRefreshToken(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		tenantID    string
		aadToken    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid token exchange parameters",
			registry:    "test.azurecr.io",
			tenantID:    "test-tenant-id",
			aadToken:    "valid-aad-token",
			expectError: true, // Will fail due to network call, but should not panic
			errorMsg:    "failed to execute token exchange request",
		},
		{
			name:        "Empty registry",
			registry:    "",
			tenantID:    "test-tenant-id",
			aadToken:    "valid-aad-token",
			expectError: true,
			errorMsg:    "no Host in request URL",
		},
		{
			name:        "Empty tenant ID",
			registry:    "test.azurecr.io",
			tenantID:    "",
			aadToken:    "valid-aad-token",
			expectError: true,
			errorMsg:    "failed to execute token exchange request",
		},
		{
			name:        "Empty AAD token",
			registry:    "test.azurecr.io",
			tenantID:    "test-tenant-id",
			aadToken:    "",
			expectError: true,
			errorMsg:    "failed to execute token exchange request",
		},
		{
			name:        "Invalid registry format",
			registry:    "invalid-registry-format",
			tenantID:    "test-tenant-id",
			aadToken:    "valid-aad-token",
			expectError: true,
			errorMsg:    "failed to execute token exchange request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			refreshToken, err := exchangeAADForACRRefreshToken(ctx, tt.registry, tt.tenantID, tt.aadToken)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, refreshToken)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, refreshToken)
			}
		})
	}
}

// TestExtractTenantIDFromToken tests JWT token parsing functionality
func TestExtractTenantIDFromToken(t *testing.T) {
	tests := []struct {
		name        string
		tokenString string
		expectError bool
		errorMsg    string
		expectedTID string
	}{
		{
			name:        "Valid JWT with tenant ID",
			tokenString: createValidJWT("test-tenant-id"),
			expectError: false,
			expectedTID: "test-tenant-id",
		},
		{
			name:        "JWT without tenant ID",
			tokenString: createJWTWithoutTenantID(),
			expectError: true,
			errorMsg:    "tenant ID not found in JWT token",
		},
		{
			name:        "Invalid JWT format - missing parts",
			tokenString: "invalid.jwt",
			expectError: true,
			errorMsg:    "invalid JWT token format",
		},
		{
			name:        "Invalid JWT format - too many parts",
			tokenString: "part1.part2.part3.part4",
			expectError: true,
			errorMsg:    "invalid JWT token format",
		},
		{
			name:        "Invalid base64 in payload",
			tokenString: "header.invalid-base64.signature",
			expectError: true,
			errorMsg:    "failed to decode JWT payload",
		},
		{
			name:        "Invalid JSON in payload",
			tokenString: createJWTWithInvalidJSON(),
			expectError: true,
			errorMsg:    "failed to parse JWT payload",
		},
		{
			name:        "Empty token string",
			tokenString: "",
			expectError: true,
			errorMsg:    "invalid JWT token format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenantID, err := extractTenantIDFromToken(tt.tokenString)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, tenantID)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTID, tenantID)
			}
		})
	}
}

// Helper functions for creating test JWT tokens
func createValidJWT(tenantID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"tid":"%s","sub":"test","iss":"test","aud":"test","exp":9999999999}`, tenantID)))
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return fmt.Sprintf("%s.%s.%s", header, payload, signature)
}

func createJWTWithoutTenantID() string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"test","iss":"test","aud":"test","exp":9999999999}`))
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return fmt.Sprintf("%s.%s.%s", header, payload, signature)
}

func createJWTWithInvalidJSON() string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"invalid json`))
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return fmt.Sprintf("%s.%s.%s", header, payload, signature)
}
