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
