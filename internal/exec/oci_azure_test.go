package exec

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestACRAuthDirect tests Azure Container Registry authentication directly
func TestACRAuthDirect(t *testing.T) {
	if os.Getenv("ATMOS_AZURE_E2E") == "" {
		t.Skip("Skipping Azure integration test (set ATMOS_AZURE_E2E=1 to run)")
	}

	tests := []struct {
		name        string
		registry    string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
		errorMsg    string
	}{
		{
			name:        "ACR .io with no authentication",
			registry:    "test.azurecr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true,
			errorMsg:    "no valid Azure authentication found",
		},
		{
			name:        "ACR .us with no authentication",
			registry:    "test.azurecr.us",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true,
			errorMsg:    "no valid Azure authentication found",
		},
		{
			name:        "ACR .cn with no authentication",
			registry:    "test.azurecr.cn",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true,
			errorMsg:    "no valid Azure authentication found",
		},
		{
			name:        "Non-ACR registry",
			registry:    "docker.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true,
			errorMsg:    "invalid Azure Container Registry format",
		},
		{
			name:        "Invalid ACR format",
			registry:    "test.azurecr.invalid",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true,
			errorMsg:    "invalid Azure Container Registry format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getACRAuth(tt.registry, tt.atmosConfig)

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

// TestGetACRAuthViaServicePrincipalDirect tests Azure Service Principal authentication directly
func TestGetACRAuthViaServicePrincipalDirect(t *testing.T) {
	if os.Getenv("ATMOS_AZURE_E2E") == "" {
		t.Skip("Skipping Azure integration test (set ATMOS_AZURE_E2E=1 to run)")
	}

	tests := []struct {
		name         string
		registry     string
		acrName      string
		clientID     string
		clientSecret string
		tenantID     string
		expectError  bool
		errorMsg     string
	}{
		{
			name:         "Valid Service Principal credentials (.io)",
			registry:     "test.azurecr.io",
			acrName:      "test",
			clientID:     "test-client-id",
			clientSecret: "test-client-secret",
			tenantID:     "test-tenant-id",
			expectError:  true, // Will fail due to invalid credentials
			errorMsg:     "failed to get Azure token",
		},
		{
			name:         "Valid Service Principal credentials (.us)",
			registry:     "test.azurecr.us",
			acrName:      "test",
			clientID:     "test-client-id",
			clientSecret: "test-client-secret",
			tenantID:     "test-tenant-id",
			expectError:  true, // Will fail due to invalid credentials
			errorMsg:     "failed to get Azure token",
		},
		{
			name:         "Valid Service Principal credentials (.cn)",
			registry:     "test.azurecr.cn",
			acrName:      "test",
			clientID:     "test-client-id",
			clientSecret: "test-client-secret",
			tenantID:     "test-tenant-id",
			expectError:  true, // Will fail due to invalid credentials
			errorMsg:     "failed to get Azure token",
		},
		{
			name:         "Missing client ID",
			registry:     "test.azurecr.io",
			acrName:      "test",
			clientID:     "",
			clientSecret: "test-client-secret",
			tenantID:     "test-tenant-id",
			expectError:  true,
			errorMsg:     "failed to get Azure token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getACRAuthViaServicePrincipal(tt.registry, tt.acrName, tt.clientID, tt.clientSecret, tt.tenantID)

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

// TestGetACRAuthViaDefaultCredentialDirect tests Azure Default Credential authentication directly
func TestGetACRAuthViaDefaultCredentialDirect(t *testing.T) {
	if os.Getenv("ATMOS_AZURE_E2E") == "" {
		t.Skip("Skipping Azure integration test (set ATMOS_AZURE_E2E=1 to run)")
	}

	tests := []struct {
		name        string
		registry    string
		acrName     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Default credential without Azure environment (.io)",
			registry:    "test.azurecr.io",
			acrName:     "test",
			expectError: true,
			errorMsg:    "failed to get Azure token",
		},
		{
			name:        "Default credential without Azure environment (.us)",
			registry:    "test.azurecr.us",
			acrName:     "test",
			expectError: true,
			errorMsg:    "failed to get Azure token",
		},
		{
			name:        "Default credential without Azure environment (.cn)",
			registry:    "test.azurecr.cn",
			acrName:     "test",
			expectError: true,
			errorMsg:    "failed to get Azure token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getACRAuthViaDefaultCredential(tt.registry, tt.acrName)

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

// TestExchangeAADForACRRefreshTokenDirect tests AAD to ACR token exchange directly
func TestExchangeAADForACRRefreshTokenDirect(t *testing.T) {
	if os.Getenv("ATMOS_AZURE_E2E") == "" {
		t.Skip("Skipping Azure integration test (set ATMOS_AZURE_E2E=1 to run)")
	}

	tests := []struct {
		name        string
		registry    string
		tenantID    string
		aadToken    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Invalid registry format",
			registry:    "invalid-registry",
			tenantID:    "test-tenant",
			aadToken:    "test-token",
			expectError: true,
			errorMsg:    "failed to execute token exchange request",
		},
		{
			name:        "Empty AAD token",
			registry:    "test.azurecr.io",
			tenantID:    "test-tenant",
			aadToken:    "",
			expectError: true,
			errorMsg:    "failed to execute token exchange request",
		},
		{
			name:        "Network error with invalid registry",
			registry:    "nonexistent.azurecr.io",
			tenantID:    "test-tenant",
			aadToken:    "test-token",
			expectError: true,
			errorMsg:    "failed to execute token exchange request",
		},
		{
			name:        "Valid parameters but network failure (.io)",
			registry:    "test.azurecr.io",
			tenantID:    "test-tenant",
			aadToken:    "test-token",
			expectError: true,
			errorMsg:    "failed to execute token exchange request",
		},
		{
			name:        "Valid parameters but network failure (.us)",
			registry:    "test.azurecr.us",
			tenantID:    "test-tenant",
			aadToken:    "test-token",
			expectError: true,
			errorMsg:    "failed to execute token exchange request",
		},
		{
			name:        "Valid parameters but network failure (.cn)",
			registry:    "test.azurecr.cn",
			tenantID:    "test-tenant",
			aadToken:    "test-token",
			expectError: true,
			errorMsg:    "failed to execute token exchange request",
		},
		{
			name:        "Empty tenant ID",
			registry:    "test.azurecr.io",
			tenantID:    "",
			aadToken:    "test-token",
			expectError: true,
			errorMsg:    "failed to execute token exchange request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := exchangeAADForACRRefreshToken(context.Background(), tt.registry, tt.tenantID, tt.aadToken)

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

// TestExchangeAADForACRRefreshTokenWithStubbedHTTP demonstrates how to stub HTTP client for testing
func TestExchangeAADForACRRefreshTokenWithStubbedHTTP(t *testing.T) {
	// Save original HTTP client
	originalClient := httpClient
	defer func() {
		httpClient = originalClient
	}()

	// Create a mock HTTP client that returns a predefined response
	mockClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &mockTransport{
			response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"refresh_token": "mock-refresh-token", "access_token": "mock-access-token"}`)),
			},
		},
	}

	// Override the HTTP client
	httpClient = mockClient

	// Test the token exchange with stubbed HTTP
	refreshToken, err := exchangeAADForACRRefreshToken(
		context.Background(),
		"test.azurecr.io",
		"test-tenant",
		"test-token",
	)

	// Verify the result
	assert.NoError(t, err)
	assert.Equal(t, "mock-refresh-token", refreshToken)
}

// mockTransport is a simple mock transport for testing
type mockTransport struct {
	response *http.Response
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.response, nil
}

// TestExtractTenantIDFromTokenDirect tests JWT token parsing for tenant ID directly
func TestExtractTenantIDFromTokenDirect(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		expectError bool
		expectedID  string
		errorMsg    string
	}{
		{
			name:        "Valid JWT with tenant ID",
			token:       createValidJWTDirect("test-tenant-id"),
			expectError: false,
			expectedID:  "test-tenant-id",
		},
		{
			name:        "JWT without tenant ID",
			token:       createJWTWithoutTenantIDDirect(),
			expectError: true,
			errorMsg:    "tenant ID not found in JWT token",
		},
		{
			name:        "Invalid JWT format",
			token:       "invalid.jwt.token",
			expectError: true,
			errorMsg:    "failed to parse JWT payload",
		},
		{
			name:        "Empty token",
			token:       "",
			expectError: true,
			errorMsg:    "invalid JWT token format",
		},
		{
			name:        "JWT with invalid JSON in payload",
			token:       createJWTWithInvalidJSONDirect(),
			expectError: true,
			errorMsg:    "failed to parse JWT payload",
		},
		{
			name:        "JWT with missing payload",
			token:       "header.signature",
			expectError: true,
			errorMsg:    "invalid JWT token format",
		},
		{
			name:        "JWT with empty payload",
			token:       "header..signature",
			expectError: true,
			errorMsg:    "failed to parse JWT payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenantID, err := extractTenantIDFromToken(tt.token)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedID, tenantID)
			}
		})
	}
}

// Helper functions for creating test JWT tokens
func createValidJWTDirect(tenantID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"tid":"%s","sub":"test","iss":"test"}`, tenantID)))
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return fmt.Sprintf("%s.%s.%s", header, payload, signature)
}

func createJWTWithoutTenantIDDirect() string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"test","iss":"test"}`))
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return fmt.Sprintf("%s.%s.%s", header, payload, signature)
}

func createJWTWithInvalidJSONDirect() string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"invalid json`))
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return fmt.Sprintf("%s.%s.%s", header, payload, signature)
}
