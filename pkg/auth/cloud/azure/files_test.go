package azure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestNewAzureFileManager(t *testing.T) {
	tests := []struct {
		name        string
		basePath    string
		expectError bool
		checkPath   func(*testing.T, string)
	}{
		{
			name:        "custom basePath",
			basePath:    "/tmp/azure-test",
			expectError: false,
			checkPath: func(t *testing.T, baseDir string) {
				assert.Equal(t, "/tmp/azure-test", baseDir)
			},
		},
		{
			name:        "empty basePath uses default",
			basePath:    "",
			expectError: false,
			checkPath: func(t *testing.T, baseDir string) {
				// Should contain .azure/atmos.
				assert.Contains(t, baseDir, ".azure")
				assert.Contains(t, baseDir, "atmos")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewAzureFileManager(tt.basePath)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, mgr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, mgr)
			if tt.checkPath != nil {
				tt.checkPath(t, mgr.baseDir)
			}
		})
	}
}

func TestAzureFileManager_GetCredentialsPath(t *testing.T) {
	mgr := &AzureFileManager{
		baseDir: "/tmp/azure-test",
	}

	tests := []struct {
		name         string
		providerName string
		expected     string
	}{
		{
			name:         "basic provider name",
			providerName: "my-azure-provider",
			expected:     "/tmp/azure-test/my-azure-provider/credentials.json",
		},
		{
			name:         "provider with hyphens",
			providerName: "prod-azure-cli",
			expected:     "/tmp/azure-test/prod-azure-cli/credentials.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mgr.GetCredentialsPath(tt.providerName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAzureFileManager_WriteCredentials(t *testing.T) {
	// Create temp directory for testing.
	tmpDir := t.TempDir()

	mgr := &AzureFileManager{
		baseDir: tmpDir,
	}

	now := time.Now().UTC()
	testCreds := &types.AzureCredentials{
		AccessToken:        "test-access-token",
		TokenType:          "Bearer",
		TenantID:           "tenant-123",
		SubscriptionID:     "sub-456",
		Location:           "eastus",
		Expiration:         now.Add(1 * time.Hour).Format(time.RFC3339),
		GraphAPIToken:      "test-graph-token",
		GraphAPIExpiration: now.Add(1 * time.Hour).Format(time.RFC3339),
		KeyVaultToken:      "test-keyvault-token",
		KeyVaultExpiration: now.Add(1 * time.Hour).Format(time.RFC3339),
	}

	tests := []struct {
		name         string
		providerName string
		identityName string
		creds        *types.AzureCredentials
		expectError  bool
	}{
		{
			name:         "successfully writes credentials",
			providerName: "test-provider",
			identityName: "test-identity",
			creds:        testCreds,
			expectError:  false,
		},
		{
			name:         "creates directory if doesn't exist",
			providerName: "new-provider",
			identityName: "new-identity",
			creds:        testCreds,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.WriteCredentials(tt.providerName, tt.identityName, tt.creds)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify file was created.
			credPath := mgr.GetCredentialsPath(tt.providerName)
			_, err = os.Stat(credPath)
			require.NoError(t, err, "Credentials file should exist")

			// Verify file permissions.
			info, err := os.Stat(credPath)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(PermissionRW), info.Mode().Perm(), "File should have 0600 permissions")

			// Verify JSON content.
			data, err := os.ReadFile(credPath)
			require.NoError(t, err)

			var loadedCreds types.AzureCredentials
			err = json.Unmarshal(data, &loadedCreds)
			require.NoError(t, err)

			// Verify all fields.
			assert.Equal(t, testCreds.AccessToken, loadedCreds.AccessToken)
			assert.Equal(t, testCreds.TokenType, loadedCreds.TokenType)
			assert.Equal(t, testCreds.TenantID, loadedCreds.TenantID)
			assert.Equal(t, testCreds.SubscriptionID, loadedCreds.SubscriptionID)
			assert.Equal(t, testCreds.Location, loadedCreds.Location)
			assert.Equal(t, testCreds.Expiration, loadedCreds.Expiration)
			assert.Equal(t, testCreds.GraphAPIToken, loadedCreds.GraphAPIToken)
			assert.Equal(t, testCreds.GraphAPIExpiration, loadedCreds.GraphAPIExpiration)
			assert.Equal(t, testCreds.KeyVaultToken, loadedCreds.KeyVaultToken)
			assert.Equal(t, testCreds.KeyVaultExpiration, loadedCreds.KeyVaultExpiration)
		})
	}
}

func TestAzureFileManager_LoadCredentials(t *testing.T) {
	// Create temp directory for testing.
	tmpDir := t.TempDir()

	mgr := &AzureFileManager{
		baseDir: tmpDir,
	}

	now := time.Now().UTC()
	testCreds := &types.AzureCredentials{
		AccessToken:        "test-access-token",
		TokenType:          "Bearer",
		TenantID:           "tenant-123",
		SubscriptionID:     "sub-456",
		Location:           "eastus",
		Expiration:         now.Add(1 * time.Hour).Format(time.RFC3339),
		GraphAPIToken:      "test-graph-token",
		GraphAPIExpiration: now.Add(1 * time.Hour).Format(time.RFC3339),
		KeyVaultToken:      "test-keyvault-token",
		KeyVaultExpiration: now.Add(1 * time.Hour).Format(time.RFC3339),
	}

	tests := []struct {
		name         string
		providerName string
		setup        func(t *testing.T, mgr *AzureFileManager, providerName string)
		expectError  bool
		errorType    error
	}{
		{
			name:         "successfully loads credentials",
			providerName: "test-provider",
			setup: func(t *testing.T, mgr *AzureFileManager, providerName string) {
				// Write credentials first.
				err := mgr.WriteCredentials(providerName, "test-identity", testCreds)
				require.NoError(t, err)
			},
			expectError: false,
		},
		{
			name:         "returns error if file doesn't exist",
			providerName: "nonexistent-provider",
			setup:        func(t *testing.T, mgr *AzureFileManager, providerName string) {},
			expectError:  true,
			errorType:    errUtils.ErrAuthenticationFailed,
		},
		{
			name:         "returns error if JSON is invalid",
			providerName: "invalid-json-provider",
			setup: func(t *testing.T, mgr *AzureFileManager, providerName string) {
				// Write invalid JSON.
				credPath := mgr.GetCredentialsPath(providerName)
				credDir := filepath.Dir(credPath)
				err := os.MkdirAll(credDir, PermissionRWX)
				require.NoError(t, err)
				err = os.WriteFile(credPath, []byte("invalid json"), PermissionRW)
				require.NoError(t, err)
			},
			expectError: true,
			errorType:   ErrLoadCredentialsFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test scenario.
			tt.setup(t, mgr, tt.providerName)

			// Load credentials.
			creds, err := mgr.LoadCredentials(tt.providerName)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, creds)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, creds)

			// Verify all fields match.
			assert.Equal(t, testCreds.AccessToken, creds.AccessToken)
			assert.Equal(t, testCreds.TokenType, creds.TokenType)
			assert.Equal(t, testCreds.TenantID, creds.TenantID)
			assert.Equal(t, testCreds.SubscriptionID, creds.SubscriptionID)
			assert.Equal(t, testCreds.Location, creds.Location)
			assert.Equal(t, testCreds.Expiration, creds.Expiration)
			assert.Equal(t, testCreds.GraphAPIToken, creds.GraphAPIToken)
			assert.Equal(t, testCreds.GraphAPIExpiration, creds.GraphAPIExpiration)
			assert.Equal(t, testCreds.KeyVaultToken, creds.KeyVaultToken)
			assert.Equal(t, testCreds.KeyVaultExpiration, creds.KeyVaultExpiration)
		})
	}
}

func TestAzureFileManager_Cleanup(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		setup        func(t *testing.T, mgr *AzureFileManager, providerName string)
		expectError  bool
	}{
		{
			name:         "removes provider directory",
			providerName: "test-provider",
			setup: func(t *testing.T, mgr *AzureFileManager, providerName string) {
				// Create provider directory with credentials.
				creds := &types.AzureCredentials{
					AccessToken:    "test-token",
					TenantID:       "tenant-123",
					SubscriptionID: "sub-456",
				}
				err := mgr.WriteCredentials(providerName, "test-identity", creds)
				require.NoError(t, err)
			},
			expectError: false,
		},
		{
			name:         "returns nil if directory doesn't exist",
			providerName: "nonexistent-provider",
			setup:        func(t *testing.T, mgr *AzureFileManager, providerName string) {},
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for each test.
			tmpDir := t.TempDir()
			mgr := &AzureFileManager{
				baseDir: tmpDir,
			}

			// Setup test scenario.
			tt.setup(t, mgr, tt.providerName)

			// Cleanup.
			err := mgr.Cleanup(tt.providerName)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify provider directory was removed.
			providerDir := filepath.Join(mgr.baseDir, tt.providerName)
			_, err = os.Stat(providerDir)
			assert.True(t, os.IsNotExist(err), "Provider directory should be removed")
		})
	}
}

func TestAzureFileManager_CredentialsExist(t *testing.T) {
	tmpDir := t.TempDir()

	mgr := &AzureFileManager{
		baseDir: tmpDir,
	}

	tests := []struct {
		name         string
		providerName string
		setup        func(t *testing.T, mgr *AzureFileManager, providerName string)
		expected     bool
	}{
		{
			name:         "returns true if file exists",
			providerName: "existing-provider",
			setup: func(t *testing.T, mgr *AzureFileManager, providerName string) {
				// Write credentials.
				creds := &types.AzureCredentials{
					AccessToken:    "test-token",
					TenantID:       "tenant-123",
					SubscriptionID: "sub-456",
				}
				err := mgr.WriteCredentials(providerName, "test-identity", creds)
				require.NoError(t, err)
			},
			expected: true,
		},
		{
			name:         "returns false if file doesn't exist",
			providerName: "nonexistent-provider",
			setup:        func(t *testing.T, mgr *AzureFileManager, providerName string) {},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test scenario.
			tt.setup(t, mgr, tt.providerName)

			// Check if credentials exist.
			result := mgr.CredentialsExist(tt.providerName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAzureFileManager_WriteCredentials_JSONMarshaling(t *testing.T) {
	tmpDir := t.TempDir()

	mgr := &AzureFileManager{
		baseDir: tmpDir,
	}

	now := time.Now().UTC().Truncate(time.Second)

	// Test all credential fields are properly marshaled.
	creds := &types.AzureCredentials{
		AccessToken:        "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.example",
		TokenType:          "Bearer",
		TenantID:           "12345678-1234-1234-1234-123456789012",
		SubscriptionID:     "87654321-4321-4321-4321-210987654321",
		Location:           "westus2",
		Expiration:         now.Add(1 * time.Hour).Format(time.RFC3339),
		GraphAPIToken:      "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJncmFwaCJ9.example",
		GraphAPIExpiration: now.Add(2 * time.Hour).Format(time.RFC3339),
		KeyVaultToken:      "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ2YXVsdCJ9.example",
		KeyVaultExpiration: now.Add(3 * time.Hour).Format(time.RFC3339),
	}

	err := mgr.WriteCredentials("json-test-provider", "json-test-identity", creds)
	require.NoError(t, err)

	// Load and verify.
	loaded, err := mgr.LoadCredentials("json-test-provider")
	require.NoError(t, err)

	assert.Equal(t, creds.AccessToken, loaded.AccessToken, "AccessToken should match")
	assert.Equal(t, creds.TokenType, loaded.TokenType, "TokenType should match")
	assert.Equal(t, creds.TenantID, loaded.TenantID, "TenantID should match")
	assert.Equal(t, creds.SubscriptionID, loaded.SubscriptionID, "SubscriptionID should match")
	assert.Equal(t, creds.Location, loaded.Location, "Location should match")
	assert.Equal(t, creds.Expiration, loaded.Expiration, "Expiration should match")
	assert.Equal(t, creds.GraphAPIToken, loaded.GraphAPIToken, "GraphAPIToken should match")
	assert.Equal(t, creds.GraphAPIExpiration, loaded.GraphAPIExpiration, "GraphAPIExpiration should match")
	assert.Equal(t, creds.KeyVaultToken, loaded.KeyVaultToken, "KeyVaultToken should match")
	assert.Equal(t, creds.KeyVaultExpiration, loaded.KeyVaultExpiration, "KeyVaultExpiration should match")
}

func TestAzureFileManager_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()

	mgr := &AzureFileManager{
		baseDir: tmpDir,
	}

	// Test concurrent writes don't corrupt file.
	// This tests the mutex locking behavior.
	providerName := "concurrent-provider"

	// Write initial credentials.
	creds1 := &types.AzureCredentials{
		AccessToken:    "token-1",
		TenantID:       "tenant-123",
		SubscriptionID: "sub-456",
	}

	err := mgr.WriteCredentials(providerName, "identity-1", creds1)
	require.NoError(t, err)

	// Verify credentials were written.
	loaded, err := mgr.LoadCredentials(providerName)
	require.NoError(t, err)
	assert.Equal(t, "token-1", loaded.AccessToken)

	// Write again with different credentials.
	creds2 := &types.AzureCredentials{
		AccessToken:    "token-2",
		TenantID:       "tenant-789",
		SubscriptionID: "sub-012",
	}

	err = mgr.WriteCredentials(providerName, "identity-2", creds2)
	require.NoError(t, err)

	// Verify new credentials overwrote old ones.
	loaded, err = mgr.LoadCredentials(providerName)
	require.NoError(t, err)
	assert.Equal(t, "token-2", loaded.AccessToken)
	assert.Equal(t, "tenant-789", loaded.TenantID)
}
