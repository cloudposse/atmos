package exec

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestACRAuthDirect tests Azure Container Registry authentication directly
func TestACRAuthDirect(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
		errorMsg    string
	}{
		{
			name:        "ACR with no authentication",
			registry:    "test.azurecr.io",
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
			name:         "Valid Service Principal credentials",
			registry:     "test.azurecr.io",
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
	tests := []struct {
		name        string
		registry    string
		acrName     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Default credential without Azure environment",
			registry:    "test.azurecr.io",
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
			name:        "Valid parameters but network failure",
			registry:    "test.azurecr.io",
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
			ctx := context.Background()
			_, err := exchangeAADForACRRefreshToken(ctx, tt.registry, tt.tenantID, tt.aadToken)

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
