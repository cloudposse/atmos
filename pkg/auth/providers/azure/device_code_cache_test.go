package azure

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockCacheStorage is a mock implementation of CacheStorage for testing.
type mockCacheStorage struct {
	files          map[string][]byte
	readFileError  error
	writeFileError error
	removeError    error
	mkdirAllError  error
	xdgCacheDir    string
}

func newMockCacheStorage() *mockCacheStorage {
	return &mockCacheStorage{
		files:       make(map[string][]byte),
		xdgCacheDir: "/mock/cache",
	}
}

func (m *mockCacheStorage) ReadFile(path string) ([]byte, error) {
	if m.readFileError != nil {
		return nil, m.readFileError
	}
	data, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (m *mockCacheStorage) WriteFile(path string, data []byte, perm os.FileMode) error {
	if m.writeFileError != nil {
		return m.writeFileError
	}
	m.files[path] = data
	return nil
}

func (m *mockCacheStorage) Remove(path string) error {
	if m.removeError != nil {
		return m.removeError
	}
	delete(m.files, path)
	return nil
}

func (m *mockCacheStorage) MkdirAll(path string, perm os.FileMode) error {
	if m.mkdirAllError != nil {
		return m.mkdirAllError
	}
	return nil
}

func (m *mockCacheStorage) GetXDGCacheDir(subdir string, perm os.FileMode) (string, error) {
	if m.xdgCacheDir == "" {
		return "", errors.New("mock xdg cache dir not set")
	}
	return filepath.Join(m.xdgCacheDir, subdir), nil
}

func TestDeviceCodeProvider_getTokenCachePath(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		xdgCacheDir  string
		xdgError     bool
		expectError  bool
	}{
		{
			name:         "successful path generation",
			providerName: "my-azure-provider",
			xdgCacheDir:  "/home/user/.cache/atmos",
			expectError:  false,
		},
		{
			name:         "xdg cache dir error",
			providerName: "test-provider",
			xdgCacheDir:  "",
			xdgError:     true,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorage := newMockCacheStorage()
			if tt.xdgError {
				mockStorage.xdgCacheDir = ""
			} else {
				mockStorage.xdgCacheDir = tt.xdgCacheDir
			}

			provider := &deviceCodeProvider{
				name:         tt.providerName,
				cacheStorage: mockStorage,
			}

			path, err := provider.getTokenCachePath()

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Contains(t, path, tt.providerName)
			assert.Contains(t, path, "token.json")
		})
	}
}

func TestDeviceCodeProvider_loadCachedToken(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name               string
		setupCache         func(*mockCacheStorage, string)
		providerTenantID   string
		expectedToken      string
		expectedGraphToken string
		shouldReturnToken  bool
	}{
		{
			name: "cache miss - file doesn't exist",
			setupCache: func(m *mockCacheStorage, path string) {
				// Don't create file.
			},
			shouldReturnToken: false,
		},
		{
			name: "invalid JSON",
			setupCache: func(m *mockCacheStorage, path string) {
				m.files[path] = []byte("invalid json")
			},
			shouldReturnToken: false,
		},
		{
			name: "expired token",
			setupCache: func(m *mockCacheStorage, path string) {
				cache := deviceCodeTokenCache{
					AccessToken: "expired-token",
					ExpiresAt:   now.Add(-1 * time.Hour),
					TenantID:    "tenant-123",
				}
				data, _ := json.Marshal(cache)
				m.files[path] = data
			},
			shouldReturnToken: false,
		},
		{
			name: "tenant ID mismatch",
			setupCache: func(m *mockCacheStorage, path string) {
				cache := deviceCodeTokenCache{
					AccessToken: "valid-token",
					ExpiresAt:   now.Add(1 * time.Hour),
					TenantID:    "different-tenant",
				}
				data, _ := json.Marshal(cache)
				m.files[path] = data
			},
			providerTenantID:  "tenant-123",
			shouldReturnToken: false,
		},
		{
			name: "valid cached token",
			setupCache: func(m *mockCacheStorage, path string) {
				cache := deviceCodeTokenCache{
					AccessToken:       "valid-token",
					TokenType:         "Bearer",
					ExpiresAt:         now.Add(1 * time.Hour),
					TenantID:          "tenant-123",
					GraphAPIToken:     "valid-graph-token",
					GraphAPIExpiresAt: now.Add(2 * time.Hour),
				}
				data, _ := json.Marshal(cache)
				m.files[path] = data
			},
			providerTenantID:   "tenant-123",
			expectedToken:      "valid-token",
			expectedGraphToken: "valid-graph-token",
			shouldReturnToken:  true,
		},
		{
			name: "token expires within 5 minutes",
			setupCache: func(m *mockCacheStorage, path string) {
				cache := deviceCodeTokenCache{
					AccessToken: "about-to-expire-token",
					ExpiresAt:   now.Add(3 * time.Minute),
					TenantID:    "tenant-123",
				}
				data, _ := json.Marshal(cache)
				m.files[path] = data
			},
			providerTenantID:  "tenant-123",
			shouldReturnToken: false, // Should not return token within 5 min buffer.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorage := newMockCacheStorage()
			provider := &deviceCodeProvider{
				name:         "test-provider",
				tenantID:     tt.providerTenantID,
				cacheStorage: mockStorage,
			}

			cachePath := filepath.Join(mockStorage.xdgCacheDir, deviceCodeTokenCacheSubdir, provider.name, deviceCodeTokenCacheFilename)
			tt.setupCache(mockStorage, cachePath)

			result := provider.loadCachedToken()

			if tt.shouldReturnToken {
				assert.Equal(t, tt.expectedToken, result.AccessToken)
				assert.False(t, result.ExpiresAt.IsZero())
				assert.Equal(t, tt.expectedGraphToken, result.GraphAPIToken)
				assert.False(t, result.GraphAPIExpiresAt.IsZero())
			} else {
				assert.Empty(t, result.AccessToken)
				assert.True(t, result.ExpiresAt.IsZero())
				assert.Empty(t, result.GraphAPIToken)
				assert.True(t, result.GraphAPIExpiresAt.IsZero())
			}
		})
	}
}

func TestDeviceCodeProvider_saveCachedToken(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name              string
		accessToken       string
		tokenType         string
		expiresAt         time.Time
		graphToken        string
		graphExpiresAt    time.Time
		subscriptionID    string
		location          string
		expectFileWritten bool
	}{
		{
			name:              "successfully saves token",
			accessToken:       "test-access-token",
			tokenType:         "Bearer",
			expiresAt:         now.Add(1 * time.Hour),
			graphToken:        "test-graph-token",
			graphExpiresAt:    now.Add(2 * time.Hour),
			subscriptionID:    "sub-456",
			location:          "eastus",
			expectFileWritten: true,
		},
		{
			name:              "saves token without graph token",
			accessToken:       "test-access-token",
			tokenType:         "Bearer",
			expiresAt:         now.Add(1 * time.Hour),
			graphToken:        "",
			graphExpiresAt:    time.Time{},
			expectFileWritten: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorage := newMockCacheStorage()
			provider := &deviceCodeProvider{
				name:           "test-provider",
				tenantID:       "tenant-123",
				subscriptionID: tt.subscriptionID,
				location:       tt.location,
				cacheStorage:   mockStorage,
			}

			err := provider.saveCachedToken(tt.accessToken, tt.tokenType, tt.expiresAt, tt.graphToken, tt.graphExpiresAt)
			require.NoError(t, err) // saveCachedToken never returns errors.

			if tt.expectFileWritten {
				cachePath := filepath.Join(mockStorage.xdgCacheDir, deviceCodeTokenCacheSubdir, provider.name, deviceCodeTokenCacheFilename)
				data, exists := mockStorage.files[cachePath]
				require.True(t, exists, "Cache file should be written")

				var cache deviceCodeTokenCache
				err := json.Unmarshal(data, &cache)
				require.NoError(t, err)

				assert.Equal(t, tt.accessToken, cache.AccessToken)
				assert.Equal(t, tt.tokenType, cache.TokenType)
				assert.WithinDuration(t, tt.expiresAt, cache.ExpiresAt, time.Second)
				assert.Equal(t, "tenant-123", cache.TenantID)
				assert.Equal(t, tt.subscriptionID, cache.SubscriptionID)
				assert.Equal(t, tt.location, cache.Location)
				assert.Equal(t, tt.graphToken, cache.GraphAPIToken)
				if !tt.graphExpiresAt.IsZero() {
					assert.WithinDuration(t, tt.graphExpiresAt, cache.GraphAPIExpiresAt, time.Second)
				}
			}
		})
	}
}

func TestDeviceCodeProvider_deleteCachedToken(t *testing.T) {
	tests := []struct {
		name        string
		setupCache  func(*mockCacheStorage, string)
		expectError bool
	}{
		{
			name: "successfully deletes token",
			setupCache: func(m *mockCacheStorage, path string) {
				m.files[path] = []byte(`{"accessToken": "test"}`)
			},
			expectError: false,
		},
		{
			name: "token doesn't exist",
			setupCache: func(m *mockCacheStorage, path string) {
				// Don't create file.
			},
			expectError: false, // Non-fatal.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorage := newMockCacheStorage()
			provider := &deviceCodeProvider{
				name:         "test-provider",
				cacheStorage: mockStorage,
			}

			cachePath := filepath.Join(mockStorage.xdgCacheDir, deviceCodeTokenCacheSubdir, provider.name, deviceCodeTokenCacheFilename)
			tt.setupCache(mockStorage, cachePath)

			err := provider.deleteCachedToken()

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify file was removed.
			_, exists := mockStorage.files[cachePath]
			assert.False(t, exists, "Cache file should be deleted")
		})
	}
}

func TestExtractJWTClaims(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		expectError bool
		checkClaims func(*testing.T, map[string]interface{})
	}{
		{
			name: "valid JWT with standard claims",
			token: func() string {
				// Create JWT with header.payload.signature format.
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
		{
			name:        "invalid JWT format - not base64",
			token:       "header.!!!invalid-base64!!!.signature",
			expectError: true,
		},
		{
			name: "invalid JSON in payload",
			token: func() string {
				header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
				payload := base64.RawURLEncoding.EncodeToString([]byte(`not valid json`))
				return header + "." + payload + ".signature"
			}(),
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

func TestExtractOIDFromToken(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		expectedOID string
		expectError bool
	}{
		{
			name: "valid token with OID",
			token: func() string {
				header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
				payload := base64.RawURLEncoding.EncodeToString([]byte(`{"oid":"abc123-def456"}`))
				return header + "." + payload + ".signature"
			}(),
			expectedOID: "abc123-def456",
			expectError: false,
		},
		{
			name: "token without OID claim",
			token: func() string {
				header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
				payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user123"}`))
				return header + "." + payload + ".signature"
			}(),
			expectError: true,
		},
		{
			name:        "invalid token format",
			token:       "invalid-token",
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

func TestExtractUsernameFromToken(t *testing.T) {
	tests := []struct {
		name             string
		token            string
		expectedUsername string
		expectError      bool
	}{
		{
			name: "token with upn claim",
			token: func() string {
				header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
				payload := base64.RawURLEncoding.EncodeToString([]byte(`{"upn":"user@example.com"}`))
				return header + "." + payload + ".signature"
			}(),
			expectedUsername: "user@example.com",
			expectError:      false,
		},
		{
			name: "token with unique_name claim",
			token: func() string {
				header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
				payload := base64.RawURLEncoding.EncodeToString([]byte(`{"unique_name":"john.doe@example.com"}`))
				return header + "." + payload + ".signature"
			}(),
			expectedUsername: "john.doe@example.com",
			expectError:      false,
		},
		{
			name: "token with email claim",
			token: func() string {
				header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
				payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"jane@example.com"}`))
				return header + "." + payload + ".signature"
			}(),
			expectedUsername: "jane@example.com",
			expectError:      false,
		},
		{
			name: "token with upn preferred over unique_name",
			token: func() string {
				header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
				payload := base64.RawURLEncoding.EncodeToString([]byte(`{"upn":"upn@example.com","unique_name":"unique@example.com"}`))
				return header + "." + payload + ".signature"
			}(),
			expectedUsername: "upn@example.com",
			expectError:      false,
		},
		{
			name: "token without username claims",
			token: func() string {
				header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
				payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user123"}`))
				return header + "." + payload + ".signature"
			}(),
			expectError: true,
		},
		{
			name:        "invalid token format",
			token:       "invalid-token",
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

func TestDeviceCodeProvider_updateAzureProfile(t *testing.T) {
	// Create temp directory for testing.
	tmpDir := t.TempDir()

	tests := []struct {
		name           string
		setupProfile   func(string)
		subscriptionID string
		tenantID       string
		username       string
		checkProfile   func(*testing.T, map[string]interface{})
	}{
		{
			name: "creates new profile with subscription",
			setupProfile: func(home string) {
				// Don't create profile file.
			},
			subscriptionID: "sub-123",
			tenantID:       "tenant-456",
			username:       "user@example.com",
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
				// Create existing profile.
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
			subscriptionID: "sub-123",
			tenantID:       "new-tenant",
			username:       "user@example.com",
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
			name: "marks other subscriptions as not default",
			setupProfile: func(home string) {
				// Create profile with multiple subscriptions.
				profile := map[string]interface{}{
					"subscriptions": []interface{}{
						map[string]interface{}{
							"id":        "sub-old",
							"tenantId":  "tenant-old",
							"isDefault": true,
						},
					},
				}
				data, _ := json.MarshalIndent(profile, "", "  ")
				azureDir := filepath.Join(home, ".azure")
				os.MkdirAll(azureDir, 0o700)
				os.WriteFile(filepath.Join(azureDir, "azureProfile.json"), data, 0o600)
			},
			subscriptionID: "sub-new",
			tenantID:       "tenant-new",
			username:       "user@example.com",
			checkProfile: func(t *testing.T, profile map[string]interface{}) {
				subs, ok := profile["subscriptions"].([]interface{})
				require.True(t, ok)
				require.Len(t, subs, 2)

				// Old subscription should not be default.
				oldSub := subs[0].(map[string]interface{})
				assert.Equal(t, "sub-old", oldSub["id"])
				assert.False(t, oldSub["isDefault"].(bool))

				// New subscription should be default.
				newSub := subs[1].(map[string]interface{})
				assert.Equal(t, "sub-new", newSub["id"])
				assert.True(t, newSub["isDefault"].(bool))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create separate home directory for each test.
			testHome := filepath.Join(tmpDir, tt.name)
			os.MkdirAll(filepath.Join(testHome, ".azure"), 0o755)

			tt.setupProfile(testHome)

			provider := &deviceCodeProvider{
				subscriptionID: tt.subscriptionID,
				tenantID:       tt.tenantID,
			}

			err := provider.updateAzureProfile(testHome, tt.username)
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

func TestDeviceCodeProvider_updateAzureCLICache_Integration(t *testing.T) {
	// This is an integration test for updateAzureCLICache.
	// We can't fully test it without real JWT tokens, but we can test the structure.

	// Create isolated temp directory for test.
	tmpHome := t.TempDir()

	// Sandbox home directory to prevent writes to real user home.
	t.Setenv("HOME", tmpHome)             // Unix/Linux/macOS
	t.Setenv("USERPROFILE", tmpHome)      // Windows
	t.Setenv("AZURE_CONFIG_DIR", tmpHome) // Azure CLI config override

	// Create a valid test JWT with OID and UPN claims.
	createTestJWT := func(oid, upn string) string {
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"RS256"}`))
		payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"oid":"%s","upn":"%s"}`, oid, upn)))
		return header + "." + payload + ".testsignature"
	}

	now := time.Now().UTC()
	accessToken := createTestJWT("user-oid-123", "user@example.com")
	graphToken := createTestJWT("user-oid-123", "user@example.com")
	keyVaultToken := createTestJWT("user-oid-123", "user@example.com")

	provider := &deviceCodeProvider{
		name:           "test-provider",
		tenantID:       "tenant-123",
		subscriptionID: "sub-456",
		config: &schema.Provider{
			Kind: "azure/device-code",
			Spec: map[string]interface{}{
				"tenant_id": "tenant-123",
			},
		},
	}

	err := provider.updateAzureCLICache(tokenCacheUpdate{
		AccessToken:       accessToken,
		ExpiresAt:         now.Add(1 * time.Hour),
		GraphToken:        graphToken,
		GraphExpiresAt:    now.Add(2 * time.Hour),
		KeyVaultToken:     keyVaultToken,
		KeyVaultExpiresAt: now.Add(3 * time.Hour),
	})

	// The function should succeed without errors in the sandboxed environment.
	assert.NoError(t, err)
}

func TestLoadAndInitializeCLICache(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		setup      func(path string)
		checkCache func(*testing.T, map[string]interface{}, map[string]interface{}, map[string]interface{})
	}{
		{
			name: "load existing cache with all sections",
			setup: func(path string) {
				cache := map[string]interface{}{
					azureCloud.FieldAccessToken: map[string]interface{}{
						"key1": "token1",
					},
					"Account": map[string]interface{}{
						"key2": "account2",
					},
				}
				data, _ := json.Marshal(cache)
				os.WriteFile(path, data, 0o600)
			},
			checkCache: func(t *testing.T, cache, accessTokenSection, accountSection map[string]interface{}) {
				require.NotNil(t, cache)
				require.NotNil(t, accessTokenSection)
				require.NotNil(t, accountSection)
				assert.Contains(t, accessTokenSection, "key1")
				assert.Contains(t, accountSection, "key2")
			},
		},
		{
			name: "create new cache when file does not exist",
			setup: func(path string) {
				// Don't create file.
			},
			checkCache: func(t *testing.T, cache, accessTokenSection, accountSection map[string]interface{}) {
				require.NotNil(t, cache)
				require.NotNil(t, accessTokenSection)
				require.NotNil(t, accountSection)
				assert.Empty(t, accessTokenSection)
				assert.Empty(t, accountSection)
			},
		},
		{
			name: "initialize missing AccessToken section",
			setup: func(path string) {
				cache := map[string]interface{}{
					"Account": map[string]interface{}{
						"key2": "account2",
					},
				}
				data, _ := json.Marshal(cache)
				os.WriteFile(path, data, 0o600)
			},
			checkCache: func(t *testing.T, cache, accessTokenSection, accountSection map[string]interface{}) {
				require.NotNil(t, cache)
				require.NotNil(t, accessTokenSection)
				require.NotNil(t, accountSection)
				assert.Empty(t, accessTokenSection)
				assert.Contains(t, accountSection, "key2")
				// Verify AccessToken section was added to cache.
				assert.Contains(t, cache, azureCloud.FieldAccessToken)
			},
		},
		{
			name: "initialize missing Account section",
			setup: func(path string) {
				cache := map[string]interface{}{
					azureCloud.FieldAccessToken: map[string]interface{}{
						"key1": "token1",
					},
				}
				data, _ := json.Marshal(cache)
				os.WriteFile(path, data, 0o600)
			},
			checkCache: func(t *testing.T, cache, accessTokenSection, accountSection map[string]interface{}) {
				require.NotNil(t, cache)
				require.NotNil(t, accessTokenSection)
				require.NotNil(t, accountSection)
				assert.Contains(t, accessTokenSection, "key1")
				assert.Empty(t, accountSection)
				// Verify Account section was added to cache.
				assert.Contains(t, cache, "Account")
			},
		},
		{
			name: "handle invalid JSON gracefully",
			setup: func(path string) {
				os.WriteFile(path, []byte("not valid json"), 0o600)
			},
			checkCache: func(t *testing.T, cache, accessTokenSection, accountSection map[string]interface{}) {
				// Should create new empty cache on JSON parse error.
				require.NotNil(t, cache)
				require.NotNil(t, accessTokenSection)
				require.NotNil(t, accountSection)
				assert.Empty(t, accessTokenSection)
				assert.Empty(t, accountSection)
			},
		},
		{
			name: "handle wrong type for sections",
			setup: func(path string) {
				cache := map[string]interface{}{
					azureCloud.FieldAccessToken: "not a map",            // Wrong type.
					"Account":                   []string{"also wrong"}, // Wrong type.
				}
				data, _ := json.Marshal(cache)
				os.WriteFile(path, data, 0o600)
			},
			checkCache: func(t *testing.T, cache, accessTokenSection, accountSection map[string]interface{}) {
				require.NotNil(t, cache)
				require.NotNil(t, accessTokenSection)
				require.NotNil(t, accountSection)
				// Should create new sections when existing ones have wrong type.
				assert.Empty(t, accessTokenSection)
				assert.Empty(t, accountSection)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &deviceCodeProvider{
				name:     "test-provider",
				tenantID: "tenant-123",
			}

			cachePath := filepath.Join(tmpDir, tt.name, "msal_token_cache.json")
			os.MkdirAll(filepath.Dir(cachePath), 0o755)

			tt.setup(cachePath)

			cache, accessTokenSection, accountSection := provider.loadAndInitializeCLICache(cachePath)

			if tt.checkCache != nil {
				tt.checkCache(t, cache, accessTokenSection, accountSection)
			}
		})
	}
}
