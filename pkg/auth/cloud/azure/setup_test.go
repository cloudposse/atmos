package azure

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// createTestJWT creates a test JWT token with the given payload claims.
func createTestJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
	payloadJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	return header + "." + payload + ".signature"
}

func TestSetupFiles(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now().UTC()
	azureCreds := &types.AzureCredentials{
		AccessToken:    "test-access-token",
		TokenType:      "Bearer",
		TenantID:       "tenant-123",
		SubscriptionID: "sub-456",
		Location:       "eastus",
		Expiration:     now.Add(1 * time.Hour).Format(time.RFC3339),
	}

	tests := []struct {
		name         string
		providerName string
		identityName string
		creds        types.ICredentials
		basePath     string
		expectError  bool
	}{
		{
			name:         "successfully sets up Azure credentials",
			providerName: "test-provider",
			identityName: "test-identity",
			creds:        azureCreds,
			basePath:     tmpDir,
			expectError:  false,
		},
		{
			name:         "handles non-Azure credentials",
			providerName: "test-provider",
			identityName: "test-identity",
			creds:        &types.AWSCredentials{},
			basePath:     tmpDir,
			expectError:  false, // Non-Azure credentials are ignored.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetupFiles(tt.providerName, tt.identityName, tt.creds, tt.basePath)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify credentials file was created (only for Azure credentials).
			if _, ok := tt.creds.(*types.AzureCredentials); ok {
				credPath := filepath.Join(tt.basePath, tt.providerName, "credentials.json")
				_, err := os.Stat(credPath)
				require.NoError(t, err, "Credentials file should exist")
			}
		})
	}
}

func TestSetAuthContext(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now().UTC()
	azureCreds := &types.AzureCredentials{
		AccessToken:    "test-access-token",
		TokenType:      "Bearer",
		TenantID:       "tenant-123",
		SubscriptionID: "sub-456",
		Location:       "eastus",
		Expiration:     now.Add(1 * time.Hour).Format(time.RFC3339),
	}

	tests := []struct {
		name        string
		params      *SetAuthContextParams
		expectError bool
		errorType   error
		checkAuth   func(*testing.T, *schema.AuthContext)
	}{
		{
			name: "successfully sets auth context",
			params: &SetAuthContextParams{
				AuthContext:  &schema.AuthContext{},
				StackInfo:    nil,
				ProviderName: "test-provider",
				IdentityName: "test-identity",
				Credentials:  azureCreds,
				BasePath:     tmpDir,
			},
			expectError: false,
			checkAuth: func(t *testing.T, authContext *schema.AuthContext) {
				require.NotNil(t, authContext.Azure)
				assert.Equal(t, "test-identity", authContext.Azure.Profile)
				assert.Equal(t, "tenant-123", authContext.Azure.TenantID)
				assert.Equal(t, "sub-456", authContext.Azure.SubscriptionID)
				assert.Equal(t, "eastus", authContext.Azure.Location)
				assert.Contains(t, authContext.Azure.CredentialsFile, "test-provider")
			},
		},
		{
			name: "uses component-level location override",
			params: &SetAuthContextParams{
				AuthContext: &schema.AuthContext{},
				StackInfo: &schema.ConfigAndStacksInfo{
					ComponentAuthSection: map[string]any{
						"identities": map[string]any{
							"test-identity": map[string]any{
								"location": "westus",
							},
						},
					},
				},
				ProviderName: "test-provider",
				IdentityName: "test-identity",
				Credentials:  azureCreds,
				BasePath:     tmpDir,
			},
			expectError: false,
			checkAuth: func(t *testing.T, authContext *schema.AuthContext) {
				require.NotNil(t, authContext.Azure)
				assert.Equal(t, "westus", authContext.Azure.Location, "Should use location override")
			},
		},
		{
			name:        "nil params returns error",
			params:      nil,
			expectError: true,
			errorType:   errUtils.ErrInvalidAuthConfig,
		},
		{
			name: "nil auth context is no-op",
			params: &SetAuthContextParams{
				AuthContext:  nil,
				ProviderName: "test-provider",
				IdentityName: "test-identity",
				Credentials:  azureCreds,
				BasePath:     tmpDir,
			},
			expectError: false,
		},
		{
			name: "non-Azure credentials is no-op",
			params: &SetAuthContextParams{
				AuthContext:  &schema.AuthContext{},
				ProviderName: "test-provider",
				IdentityName: "test-identity",
				Credentials:  &types.AWSCredentials{},
				BasePath:     tmpDir,
			},
			expectError: false,
			checkAuth: func(t *testing.T, authContext *schema.AuthContext) {
				assert.Nil(t, authContext.Azure)
			},
		},
		{
			name: "typed-nil Azure credentials is no-op",
			params: &SetAuthContextParams{
				AuthContext:  &schema.AuthContext{},
				ProviderName: "test-provider",
				IdentityName: "test-identity",
				Credentials:  (*types.AzureCredentials)(nil),
				BasePath:     tmpDir,
			},
			expectError: false,
			checkAuth: func(t *testing.T, authContext *schema.AuthContext) {
				assert.Nil(t, authContext.Azure)
			},
		},
		{
			name: "expired credentials returns error",
			params: &SetAuthContextParams{
				AuthContext:  &schema.AuthContext{},
				ProviderName: "test-provider",
				IdentityName: "test-identity",
				Credentials: &types.AzureCredentials{
					AccessToken:    "test-access-token",
					TokenType:      "Bearer",
					TenantID:       "tenant-123",
					SubscriptionID: "sub-456",
					Location:       "eastus",
					Expiration:     now.Add(-1 * time.Hour).Format(time.RFC3339), // Expired 1 hour ago
				},
				BasePath: tmpDir,
			},
			expectError: true,
			errorType:   errUtils.ErrAuthenticationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetAuthContext(tt.params)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
				return
			}

			require.NoError(t, err)

			if tt.checkAuth != nil && tt.params != nil && tt.params.AuthContext != nil {
				tt.checkAuth(t, tt.params.AuthContext)
			}
		})
	}
}

func TestGetComponentLocationOverride(t *testing.T) {
	tests := []struct {
		name         string
		stackInfo    *schema.ConfigAndStacksInfo
		identityName string
		expected     string
	}{
		{
			name: "location override present",
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentAuthSection: map[string]any{
					"identities": map[string]any{
						"test-identity": map[string]any{
							"location": "westus2",
						},
					},
				},
			},
			identityName: "test-identity",
			expected:     "westus2",
		},
		{
			name:         "nil stack info",
			stackInfo:    nil,
			identityName: "test-identity",
			expected:     "",
		},
		{
			name: "nil component auth section",
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentAuthSection: nil,
			},
			identityName: "test-identity",
			expected:     "",
		},
		{
			name: "identities not a map",
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentAuthSection: map[string]any{
					"identities": "not-a-map",
				},
			},
			identityName: "test-identity",
			expected:     "",
		},
		{
			name: "identity not found",
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentAuthSection: map[string]any{
					"identities": map[string]any{
						"other-identity": map[string]any{
							"location": "westus",
						},
					},
				},
			},
			identityName: "test-identity",
			expected:     "",
		},
		{
			name: "identity config not a map",
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentAuthSection: map[string]any{
					"identities": map[string]any{
						"test-identity": "not-a-map",
					},
				},
			},
			identityName: "test-identity",
			expected:     "",
		},
		{
			name: "location not a string",
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentAuthSection: map[string]any{
					"identities": map[string]any{
						"test-identity": map[string]any{
							"location": 123,
						},
					},
				},
			},
			identityName: "test-identity",
			expected:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getComponentLocationOverride(tt.stackInfo, tt.identityName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetEnvironmentVariables(t *testing.T) {
	tmpDir := t.TempDir()

	// Create credentials file for testing.
	mgr, err := NewAzureFileManager(tmpDir)
	require.NoError(t, err)

	now := time.Now().UTC()
	creds := &types.AzureCredentials{
		AccessToken:    "test-access-token",
		TokenType:      "Bearer",
		TenantID:       "tenant-123",
		SubscriptionID: "sub-456",
		Location:       "eastus",
		Expiration:     now.Add(1 * time.Hour).Format(time.RFC3339),
	}

	err = mgr.WriteCredentials("test-provider", "test-identity", creds)
	require.NoError(t, err)

	credPath := mgr.GetCredentialsPath("test-provider")

	tests := []struct {
		name             string
		authContext      *schema.AuthContext
		stackInfo        *schema.ConfigAndStacksInfo
		expectedContains map[string]string
	}{
		{
			name: "sets environment variables from auth context",
			authContext: &schema.AuthContext{
				Azure: &schema.AzureAuthContext{
					CredentialsFile: credPath,
					Profile:         "test-identity",
					SubscriptionID:  "sub-456",
					TenantID:        "tenant-123",
					Location:        "eastus",
				},
			},
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentEnvSection: map[string]any{},
			},
			expectedContains: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "sub-456",
				"ARM_SUBSCRIPTION_ID":   "sub-456",
				"AZURE_TENANT_ID":       "tenant-123",
				"ARM_TENANT_ID":         "tenant-123",
				"AZURE_LOCATION":        "eastus",
				"ARM_LOCATION":          "eastus",
				"ARM_USE_CLI":           "true",
			},
		},
		{
			name:        "nil auth context is no-op",
			authContext: nil,
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentEnvSection: map[string]any{},
			},
			expectedContains: map[string]string{},
		},
		{
			name: "nil Azure auth context is no-op",
			authContext: &schema.AuthContext{
				Azure: nil,
			},
			stackInfo: &schema.ConfigAndStacksInfo{
				ComponentEnvSection: map[string]any{},
			},
			expectedContains: map[string]string{},
		},
		{
			name: "nil stack info is no-op",
			authContext: &schema.AuthContext{
				Azure: &schema.AzureAuthContext{
					SubscriptionID: "sub-456",
					TenantID:       "tenant-123",
				},
			},
			stackInfo: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetEnvironmentVariables(tt.authContext, tt.stackInfo)
			require.NoError(t, err)

			if tt.stackInfo != nil && len(tt.expectedContains) > 0 {
				for key, expectedValue := range tt.expectedContains {
					value, exists := tt.stackInfo.ComponentEnvSection[key]
					require.True(t, exists, "Expected %s to exist", key)
					assert.Equal(t, expectedValue, value, "Expected %s=%s", key, expectedValue)
				}
			}
		})
	}
}

func TestExtractJWTClaims_Setup(t *testing.T) {
	// This function is duplicated from device_code_cache_test.go but tests the setup.go implementation.
	tests := []struct {
		name        string
		token       string
		expectError bool
		checkClaims func(*testing.T, map[string]interface{})
	}{
		{
			name: "valid JWT with standard claims",
			token: func() string {
				header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"RS256"}`))
				payload := base64.RawURLEncoding.EncodeToString([]byte(`{"oid":"12345","upn":"user@example.com","exp":1234567890}`))
				return header + "." + payload + ".signature"
			}(),
			expectError: false,
			checkClaims: func(t *testing.T, claims map[string]interface{}) {
				assert.Equal(t, "12345", claims["oid"])
				assert.Equal(t, "user@example.com", claims["upn"])
			},
		},
		{
			name:        "invalid JWT format - missing parts",
			token:       "invalid.token",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := extractJWTClaims(tt.token)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, claims)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, claims)
			if tt.checkClaims != nil {
				tt.checkClaims(t, claims)
			}
		})
	}
}

func TestExtractOIDFromToken_Setup(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		expectedOID string
		expectError bool
	}{
		{
			name:        "valid token with OID",
			token:       createTestJWT(map[string]interface{}{"oid": "abc123-def456"}),
			expectedOID: "abc123-def456",
			expectError: false,
		},
		{
			name:        "token without OID claim",
			token:       createTestJWT(map[string]interface{}{"sub": "user123"}),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oid, err := extractOIDFromToken(tt.token)

			if tt.expectError {
				require.Error(t, err)
				assert.Empty(t, oid)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedOID, oid)
		})
	}
}

func TestExtractUsernameFromToken_Setup(t *testing.T) {
	tests := []struct {
		name             string
		token            string
		expectedUsername string
		expectError      bool
	}{
		{
			name:             "token with upn claim",
			token:            createTestJWT(map[string]interface{}{"upn": "user@example.com"}),
			expectedUsername: "user@example.com",
			expectError:      false,
		},
		{
			name:        "token without username claims",
			token:       createTestJWT(map[string]interface{}{"sub": "user123"}),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, err := extractUsernameFromToken(tt.token)

			if tt.expectError {
				require.Error(t, err)
				assert.Empty(t, username)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedUsername, username)
		})
	}
}

func TestExtractUsernameOrFallback(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name: "valid token with username",
			token: func() string {
				header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
				payload := base64.RawURLEncoding.EncodeToString([]byte(`{"upn":"user@example.com"}`))
				return header + "." + payload + ".signature"
			}(),
			expected: "user@example.com",
		},
		{
			name:     "invalid token returns fallback",
			token:    "invalid-token",
			expected: "user@unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractUsernameOrFallback(tt.token)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStripBOM(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "data with UTF-8 BOM",
			input:    []byte{0xEF, 0xBB, 0xBF, 'h', 'e', 'l', 'l', 'o'},
			expected: []byte{'h', 'e', 'l', 'l', 'o'},
		},
		{
			name:     "data without BOM",
			input:    []byte{'h', 'e', 'l', 'l', 'o'},
			expected: []byte{'h', 'e', 'l', 'l', 'o'},
		},
		{
			name:     "empty data",
			input:    []byte{},
			expected: []byte{},
		},
		{
			name:     "data shorter than BOM",
			input:    []byte{0xEF, 0xBB},
			expected: []byte{0xEF, 0xBB},
		},
		{
			name:     "partial BOM-like data",
			input:    []byte{0xEF, 0xBB, 0xFF},
			expected: []byte{0xEF, 0xBB, 0xFF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripBOM(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpdateMSALCache(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid test JWT with OID claim.
	createTestJWT := func(oid, upn string) string {
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"RS256"}`))
		payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"oid":"%s","upn":"%s"}`, oid, upn)))
		return header + "." + payload + ".testsignature"
	}

	now := time.Now().UTC()
	accessToken := createTestJWT("user-oid-123", "user@example.com")
	expiration := now.Add(1 * time.Hour).Format(time.RFC3339)
	graphToken := createTestJWT("user-oid-123", "user@example.com")
	graphExpiration := now.Add(2 * time.Hour).Format(time.RFC3339)
	keyVaultToken := createTestJWT("user-oid-123", "user@example.com")
	keyVaultExpiration := now.Add(3 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name        string
		expectError bool
	}{
		{
			name:        "successfully updates MSAL cache",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := updateMSALCache(&msalCacheUpdate{
				Home:               tmpDir,
				AccessToken:        accessToken,
				Expiration:         expiration,
				GraphToken:         graphToken,
				GraphExpiration:    graphExpiration,
				KeyVaultToken:      keyVaultToken,
				KeyVaultExpiration: keyVaultExpiration,
				UserOID:            "user-oid-123",
				TenantID:           "tenant-123",
			})

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify MSAL cache was created.
			msalCachePath := filepath.Join(tmpDir, ".azure", "msal_token_cache.json")
			data, err := os.ReadFile(msalCachePath)
			require.NoError(t, err)

			var cache map[string]interface{}
			err = json.Unmarshal(data, &cache)
			require.NoError(t, err)

			// Verify AccessToken section exists.
			accessTokenSection, ok := cache["AccessToken"].(map[string]interface{})
			require.True(t, ok, "AccessToken section should exist")
			assert.NotEmpty(t, accessTokenSection, "AccessToken section should not be empty")

			// Verify Account section exists.
			accountSection, ok := cache["Account"].(map[string]interface{})
			require.True(t, ok, "Account section should exist")
			assert.NotEmpty(t, accountSection, "Account section should not be empty")
		})
	}
}

func TestUpdateAzureProfile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name           string
		setupProfile   func(string)
		username       string
		tenantID       string
		subscriptionID string
		expectError    bool
		checkProfile   func(*testing.T, map[string]interface{})
	}{
		{
			name: "creates new profile with subscription",
			setupProfile: func(home string) {
				// Don't create profile file.
			},
			username:       "user@example.com",
			tenantID:       "tenant-456",
			subscriptionID: "sub-123",
			expectError:    false,
			checkProfile: func(t *testing.T, profile map[string]interface{}) {
				subs, ok := profile["subscriptions"].([]interface{})
				require.True(t, ok)
				require.Len(t, subs, 1)

				sub := subs[0].(map[string]interface{})
				assert.Equal(t, "sub-123", sub["id"])
				assert.Equal(t, "tenant-456", sub["tenantId"])
				assert.True(t, sub["isDefault"].(bool))
			},
		},
		{
			name: "updates existing subscription",
			setupProfile: func(home string) {
				profile := map[string]interface{}{
					"installationId": "test-install",
					"subscriptions": []interface{}{
						map[string]interface{}{
							"id":        "sub-123",
							"tenantId":  "old-tenant",
							"isDefault": false,
						},
					},
				}
				data, _ := json.MarshalIndent(profile, "", "  ")
				azureDir := filepath.Join(home, ".azure")
				os.MkdirAll(azureDir, 0o700)
				os.WriteFile(filepath.Join(azureDir, "azureProfile.json"), data, 0o600)
			},
			username:       "user@example.com",
			tenantID:       "new-tenant",
			subscriptionID: "sub-123",
			expectError:    false,
			checkProfile: func(t *testing.T, profile map[string]interface{}) {
				subs, ok := profile["subscriptions"].([]interface{})
				require.True(t, ok)
				require.Len(t, subs, 1)

				sub := subs[0].(map[string]interface{})
				assert.Equal(t, "sub-123", sub["id"])
				assert.Equal(t, "new-tenant", sub["tenantId"])
				assert.True(t, sub["isDefault"].(bool))
			},
		},
		{
			name: "profile with UTF-8 BOM",
			setupProfile: func(home string) {
				profile := map[string]interface{}{
					"subscriptions": []interface{}{},
				}
				data, _ := json.MarshalIndent(profile, "", "  ")
				// Add BOM.
				dataWithBOM := append([]byte{0xEF, 0xBB, 0xBF}, data...)
				azureDir := filepath.Join(home, ".azure")
				os.MkdirAll(azureDir, 0o700)
				os.WriteFile(filepath.Join(azureDir, "azureProfile.json"), dataWithBOM, 0o600)
			},
			username:       "user@example.com",
			tenantID:       "tenant-123",
			subscriptionID: "sub-456",
			expectError:    false,
			checkProfile: func(t *testing.T, profile map[string]interface{}) {
				subs, ok := profile["subscriptions"].([]interface{})
				require.True(t, ok)
				require.Len(t, subs, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create separate home directory for each test.
			testHome := filepath.Join(tmpDir, tt.name)
			os.MkdirAll(filepath.Join(testHome, ".azure"), 0o755)

			tt.setupProfile(testHome)

			err := updateAzureProfile(testHome, tt.username, tt.tenantID, tt.subscriptionID, false)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Load and verify profile.
			profilePath := filepath.Join(testHome, ".azure", "azureProfile.json")
			data, err := os.ReadFile(profilePath)
			require.NoError(t, err)

			var profile map[string]interface{}
			err = json.Unmarshal(data, &profile)
			require.NoError(t, err)

			if tt.checkProfile != nil {
				tt.checkProfile(t, profile)
			}
		})
	}
}

func TestLoadMSALCache(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		setup       func(path string)
		expectError bool
		checkCache  func(*testing.T, map[string]interface{})
	}{
		{
			name: "load existing valid cache",
			setup: func(path string) {
				cache := map[string]interface{}{
					"AccessToken": map[string]interface{}{
						"key1": "token1",
					},
					"Account": map[string]interface{}{
						"key2": "account2",
					},
				}
				data, _ := json.Marshal(cache)
				os.WriteFile(path, data, 0o600)
			},
			expectError: false,
			checkCache: func(t *testing.T, cache map[string]interface{}) {
				require.NotNil(t, cache)
				assert.Contains(t, cache, "AccessToken")
				assert.Contains(t, cache, "Account")
			},
		},
		{
			name: "create new cache when file does not exist",
			setup: func(path string) {
				// Don't create file.
			},
			expectError: false,
			checkCache: func(t *testing.T, cache map[string]interface{}) {
				require.NotNil(t, cache)
				assert.Empty(t, cache)
			},
		},
		{
			name: "return error on read failure",
			setup: func(path string) {
				// Create directory instead of file to trigger read error.
				os.MkdirAll(path, 0o755)
			},
			expectError: true,
		},
		{
			name: "return error on invalid JSON",
			setup: func(path string) {
				os.WriteFile(path, []byte("not valid json"), 0o600)
			},
			expectError: true,
		},
		{
			name: "handle empty file",
			setup: func(path string) {
				os.WriteFile(path, []byte("{}"), 0o600)
			},
			expectError: false,
			checkCache: func(t *testing.T, cache map[string]interface{}) {
				require.NotNil(t, cache)
				assert.Empty(t, cache)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cachePath := filepath.Join(tmpDir, tt.name, "msal_token_cache.json")
			os.MkdirAll(filepath.Dir(cachePath), 0o755)

			tt.setup(cachePath)

			cache, err := loadMSALCache(cachePath)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.checkCache != nil {
				tt.checkCache(t, cache)
			}
		})
	}
}

func TestCreateServicePrincipalEntry(t *testing.T) {
	tests := []struct {
		name           string
		clientID       string
		tenantID       string
		federatedToken string
		expected       map[string]interface{}
	}{
		{
			name:           "creates entry with correct Azure CLI field names",
			clientID:       "app-client-id-123",
			tenantID:       "tenant-id-456",
			federatedToken: "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.test.signature",
			expected: map[string]interface{}{
				"client_id":        "app-client-id-123",
				"tenant":           "tenant-id-456",
				"client_assertion": "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.test.signature",
			},
		},
		{
			name:           "handles empty values",
			clientID:       "",
			tenantID:       "",
			federatedToken: "",
			expected: map[string]interface{}{
				"client_id":        "",
				"tenant":           "",
				"client_assertion": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createServicePrincipalEntry(tt.clientID, tt.tenantID, tt.federatedToken)
			assert.Equal(t, tt.expected, result)

			// Verify field names match Azure CLI's ServicePrincipalStore constants.
			_, hasClientID := result["client_id"]
			assert.True(t, hasClientID, "Should have 'client_id' field (not 'servicePrincipalId')")

			_, hasTenant := result["tenant"]
			assert.True(t, hasTenant, "Should have 'tenant' field (not 'servicePrincipalTenant')")

			_, hasAssertion := result["client_assertion"]
			assert.True(t, hasAssertion, "Should have 'client_assertion' field")
		})
	}
}

func TestUpdateServicePrincipalEntries(t *testing.T) {
	tests := []struct {
		name         string
		setupEntries func(path string)
		clientID     string
		tenantID     string
		token        string
		expectError  bool
		checkEntries func(*testing.T, []map[string]interface{})
	}{
		{
			name: "creates new entries file when none exists",
			setupEntries: func(path string) {
				// Don't create file.
			},
			clientID:    "new-client-123",
			tenantID:    "tenant-456",
			token:       "federated-token-xyz",
			expectError: false,
			checkEntries: func(t *testing.T, entries []map[string]interface{}) {
				require.Len(t, entries, 1)
				assert.Equal(t, "new-client-123", entries[0]["client_id"])
				assert.Equal(t, "tenant-456", entries[0]["tenant"])
				assert.Equal(t, "federated-token-xyz", entries[0]["client_assertion"])
			},
		},
		{
			name: "updates existing entry for same client ID",
			setupEntries: func(path string) {
				entries := []map[string]interface{}{
					{
						"client_id":        "existing-client",
						"tenant":           "old-tenant",
						"client_assertion": "old-token",
					},
				}
				data, _ := json.MarshalIndent(entries, "", "  ")
				os.WriteFile(path, data, 0o600)
			},
			clientID:    "existing-client",
			tenantID:    "new-tenant",
			token:       "new-token",
			expectError: false,
			checkEntries: func(t *testing.T, entries []map[string]interface{}) {
				require.Len(t, entries, 1, "Should update existing entry, not add new one")
				assert.Equal(t, "existing-client", entries[0]["client_id"])
				assert.Equal(t, "new-tenant", entries[0]["tenant"])
				assert.Equal(t, "new-token", entries[0]["client_assertion"])
			},
		},
		{
			name: "adds new entry when different client ID",
			setupEntries: func(path string) {
				entries := []map[string]interface{}{
					{
						"client_id":        "first-client",
						"tenant":           "tenant-1",
						"client_assertion": "token-1",
					},
				}
				data, _ := json.MarshalIndent(entries, "", "  ")
				os.WriteFile(path, data, 0o600)
			},
			clientID:    "second-client",
			tenantID:    "tenant-2",
			token:       "token-2",
			expectError: false,
			checkEntries: func(t *testing.T, entries []map[string]interface{}) {
				require.Len(t, entries, 2, "Should add new entry")
				// First entry preserved.
				assert.Equal(t, "first-client", entries[0]["client_id"])
				// New entry added.
				assert.Equal(t, "second-client", entries[1]["client_id"])
				assert.Equal(t, "tenant-2", entries[1]["tenant"])
				assert.Equal(t, "token-2", entries[1]["client_assertion"])
			},
		},
		{
			name: "handles file with UTF-8 BOM",
			setupEntries: func(path string) {
				entries := []map[string]interface{}{
					{
						"client_id":        "bom-client",
						"tenant":           "bom-tenant",
						"client_assertion": "bom-token",
					},
				}
				data, _ := json.MarshalIndent(entries, "", "  ")
				// Add BOM.
				dataWithBOM := append([]byte{0xEF, 0xBB, 0xBF}, data...)
				os.WriteFile(path, dataWithBOM, 0o600)
			},
			clientID:    "new-client",
			tenantID:    "new-tenant",
			token:       "new-token",
			expectError: false,
			checkEntries: func(t *testing.T, entries []map[string]interface{}) {
				require.Len(t, entries, 2, "Should parse file with BOM and add new entry")
			},
		},
		{
			name: "handles corrupted JSON by creating fresh file",
			setupEntries: func(path string) {
				os.WriteFile(path, []byte("not valid json"), 0o600)
			},
			clientID:    "fresh-client",
			tenantID:    "fresh-tenant",
			token:       "fresh-token",
			expectError: false,
			checkEntries: func(t *testing.T, entries []map[string]interface{}) {
				require.Len(t, entries, 1, "Should create fresh entries on corrupted file")
				assert.Equal(t, "fresh-client", entries[0]["client_id"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create separate home directory for each test.
			tmpDir := t.TempDir()
			azureDir := filepath.Join(tmpDir, ".azure")
			os.MkdirAll(azureDir, 0o755)

			entriesPath := filepath.Join(azureDir, "service_principal_entries.json")
			tt.setupEntries(entriesPath)

			err := updateServicePrincipalEntries(tmpDir, tt.clientID, tt.tenantID, tt.token)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Load and verify entries.
			data, err := os.ReadFile(entriesPath)
			require.NoError(t, err)

			var entries []map[string]interface{}
			err = json.Unmarshal(data, &entries)
			require.NoError(t, err)

			if tt.checkEntries != nil {
				tt.checkEntries(t, entries)
			}
		})
	}
}

func TestSetAuthContext_OIDCFields(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now().UTC()

	tests := []struct {
		name      string
		creds     *types.AzureCredentials
		checkAuth func(*testing.T, *schema.AuthContext)
	}{
		{
			name: "sets OIDC fields for service principal",
			creds: &types.AzureCredentials{
				AccessToken:        "test-access-token",
				TokenType:          "Bearer",
				TenantID:           "tenant-123",
				SubscriptionID:     "sub-456",
				Location:           "eastus",
				Expiration:         now.Add(1 * time.Hour).Format(time.RFC3339),
				ClientID:           "oidc-client-id",
				IsServicePrincipal: true,
				TokenFilePath:      "/path/to/token",
			},
			checkAuth: func(t *testing.T, authContext *schema.AuthContext) {
				require.NotNil(t, authContext.Azure)
				assert.True(t, authContext.Azure.UseOIDC, "UseOIDC should be true for service principal")
				assert.Equal(t, "oidc-client-id", authContext.Azure.ClientID)
				assert.Equal(t, "/path/to/token", authContext.Azure.TokenFilePath)
			},
		},
		{
			name: "does not set OIDC fields for user authentication",
			creds: &types.AzureCredentials{
				AccessToken:        "test-access-token",
				TokenType:          "Bearer",
				TenantID:           "tenant-123",
				SubscriptionID:     "sub-456",
				Location:           "eastus",
				Expiration:         now.Add(1 * time.Hour).Format(time.RFC3339),
				ClientID:           "", // No client ID for user auth.
				IsServicePrincipal: false,
			},
			checkAuth: func(t *testing.T, authContext *schema.AuthContext) {
				require.NotNil(t, authContext.Azure)
				assert.False(t, authContext.Azure.UseOIDC, "UseOIDC should be false for user auth")
				assert.Empty(t, authContext.Azure.ClientID)
				assert.Empty(t, authContext.Azure.TokenFilePath)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authContext := &schema.AuthContext{}
			params := &SetAuthContextParams{
				AuthContext:  authContext,
				ProviderName: "test-provider",
				IdentityName: "test-identity",
				Credentials:  tt.creds,
				BasePath:     tmpDir,
			}

			err := SetAuthContext(params)
			require.NoError(t, err)

			if tt.checkAuth != nil {
				tt.checkAuth(t, authContext)
			}
		})
	}
}

func TestSetEnvironmentVariables_OIDC(t *testing.T) {
	tests := []struct {
		name             string
		authContext      *schema.AuthContext
		expectedContains map[string]string
		expectedMissing  []string
	}{
		{
			name: "sets OIDC environment variables for service principal",
			authContext: &schema.AuthContext{
				Azure: &schema.AzureAuthContext{
					SubscriptionID: "sub-456",
					TenantID:       "tenant-123",
					Location:       "eastus",
					UseOIDC:        true,
					ClientID:       "oidc-client-id",
					TokenFilePath:  "/path/to/token",
				},
			},
			expectedContains: map[string]string{
				"AZURE_SUBSCRIPTION_ID":      "sub-456",
				"ARM_SUBSCRIPTION_ID":        "sub-456",
				"AZURE_TENANT_ID":            "tenant-123",
				"ARM_TENANT_ID":              "tenant-123",
				"ARM_USE_OIDC":               "true",
				"ARM_CLIENT_ID":              "oidc-client-id",
				"AZURE_FEDERATED_TOKEN_FILE": "/path/to/token",
			},
			expectedMissing: []string{
				"ARM_USE_CLI", // Should NOT be set when using OIDC.
			},
		},
		{
			name: "sets CLI environment variables for user authentication",
			authContext: &schema.AuthContext{
				Azure: &schema.AzureAuthContext{
					SubscriptionID: "sub-456",
					TenantID:       "tenant-123",
					Location:       "eastus",
					UseOIDC:        false,
					ClientID:       "",
					TokenFilePath:  "",
				},
			},
			expectedContains: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "sub-456",
				"ARM_SUBSCRIPTION_ID":   "sub-456",
				"AZURE_TENANT_ID":       "tenant-123",
				"ARM_TENANT_ID":         "tenant-123",
				"ARM_USE_CLI":           "true",
			},
			expectedMissing: []string{
				"ARM_USE_OIDC", // Should NOT be set when using CLI auth.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stackInfo := &schema.ConfigAndStacksInfo{
				ComponentEnvSection: map[string]any{},
			}

			err := SetEnvironmentVariables(tt.authContext, stackInfo)
			require.NoError(t, err)

			// Check expected variables are present with correct values.
			for key, expectedValue := range tt.expectedContains {
				value, exists := stackInfo.ComponentEnvSection[key]
				require.True(t, exists, "Expected %s to exist", key)
				assert.Equal(t, expectedValue, value, "Expected %s=%s", key, expectedValue)
			}

			// Check unwanted variables are missing.
			for _, key := range tt.expectedMissing {
				_, exists := stackInfo.ComponentEnvSection[key]
				assert.False(t, exists, "Expected %s to be missing", key)
			}
		})
	}
}

func TestUpdateMSALCache_ServicePrincipal(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now().UTC()
	accessToken := createTestJWT(map[string]interface{}{"oid": "sp-oid-123"})
	expiration := now.Add(1 * time.Hour).Format(time.RFC3339)

	err := updateMSALCache(&msalCacheUpdate{
		Home:               tmpDir,
		AccessToken:        accessToken,
		Expiration:         expiration,
		UserOID:            "sp-oid-123",
		TenantID:           "tenant-123",
		ClientID:           "service-principal-client-id",
		IsServicePrincipal: true,
	})
	require.NoError(t, err)

	// Verify MSAL cache was created.
	msalCachePath := filepath.Join(tmpDir, ".azure", "msal_token_cache.json")
	data, err := os.ReadFile(msalCachePath)
	require.NoError(t, err)

	var cache map[string]interface{}
	err = json.Unmarshal(data, &cache)
	require.NoError(t, err)

	// Verify AccessToken section exists with service principal format.
	accessTokenSection, ok := cache["AccessToken"].(map[string]interface{})
	require.True(t, ok, "AccessToken section should exist")
	assert.NotEmpty(t, accessTokenSection, "AccessToken section should not be empty")

	// For service principal, there should be an entry starting with "-" (empty home_account_id).
	var foundSPToken bool
	for key := range accessTokenSection {
		// Service principal token keys start with "-" because home_account_id is empty.
		if len(key) > 0 && key[0] == '-' {
			foundSPToken = true
			break
		}
	}
	assert.True(t, foundSPToken, "Should have service principal token with key starting with '-'")

	// Verify AppMetadata section exists (required for service principal).
	appMetadata, ok := cache["AppMetadata"].(map[string]interface{})
	require.True(t, ok, "AppMetadata section should exist for service principal")
	assert.NotEmpty(t, appMetadata, "AppMetadata section should not be empty")

	// Verify Account section is empty (service principal doesn't create account entries).
	accountSection, ok := cache["Account"].(map[string]interface{})
	require.True(t, ok, "Account section should exist")
	assert.Empty(t, accountSection, "Account section should be empty for service principal")
}
