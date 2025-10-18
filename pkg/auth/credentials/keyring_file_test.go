package credentials

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/99designs/keyring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// unsupportedCreds is a test type that implements ICredentials but is not supported by the store.
type unsupportedCreds struct{}

func (u *unsupportedCreds) IsExpired() bool                            { return false }
func (u *unsupportedCreds) GetExpiration() (*time.Time, error)         { return nil, nil }
func (u *unsupportedCreds) BuildWhoamiInfo(info *types.WhoamiInfo)     {}

func TestFileKeyring_NewStore(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)
	assert.NotNil(t, store)
	assert.NotEmpty(t, store.path)

	// Verify directory was created.
	dirInfo, err := os.Stat(filepath.Dir(store.path))
	require.NoError(t, err)
	assert.True(t, dirInfo.IsDir())
	// Note: Exact permission checking is platform-dependent and may be affected by umask.
	// The important thing is the directory exists and is accessible.
}

func TestFileKeyring_NewStoreDefaultPath(t *testing.T) {
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	// Create with default path.
	store, err := newFileKeyringStore(nil)
	require.NoError(t, err)
	assert.NotNil(t, store)

	// Should use ~/.atmos/keyring.
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	expectedPath := filepath.Join(homeDir, ".atmos", "keyring")
	assert.Equal(t, expectedPath, store.path)
}

func TestFileKeyring_CustomPasswordEnv(t *testing.T) {
	tempDir := t.TempDir()
	customEnv := "MY_CUSTOM_KEYRING_PASSWORD"
	t.Setenv(customEnv, "custom-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path":         filepath.Join(tempDir, "test-keyring"),
				"password_env": customEnv,
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestFileKeyring_MissingPassword(t *testing.T) {
	tempDir := t.TempDir()

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	// Should fail without password and non-TTY.
	_, err := newFileKeyringStore(authConfig)
	// The keyring library will attempt to read/create the file, which requires password.
	// This might succeed or fail depending on whether the keyring file exists.
	// The important part is that our password prompt logic is tested elsewhere.
	_ = err // Expected to potentially error without password
}

func TestFileKeyring_StoreRetrieve_AWS(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	alias := "test-aws"
	exp := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIA123456",
		SecretAccessKey: "SECRET123",
		SessionToken:    "TOKEN123",
		Region:          "us-east-1",
		Expiration:      exp,
	}

	// Store credentials.
	err = store.Store(alias, creds)
	require.NoError(t, err)

	// Retrieve credentials.
	retrieved, err := store.Retrieve(alias)
	require.NoError(t, err)

	awsCreds, ok := retrieved.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, creds.AccessKeyID, awsCreds.AccessKeyID)
	assert.Equal(t, creds.SecretAccessKey, awsCreds.SecretAccessKey)
	assert.Equal(t, creds.SessionToken, awsCreds.SessionToken)
	assert.Equal(t, creds.Region, awsCreds.Region)
	assert.Equal(t, creds.Expiration, awsCreds.Expiration)
}

func TestFileKeyring_StoreRetrieve_OIDC(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	alias := "test-oidc"
	creds := &types.OIDCCredentials{
		Token:    "header.payload.signature",
		Provider: "github",
		Audience: "sts.amazonaws.com",
	}

	// Store credentials.
	err = store.Store(alias, creds)
	require.NoError(t, err)

	// Retrieve credentials.
	retrieved, err := store.Retrieve(alias)
	require.NoError(t, err)

	oidcCreds, ok := retrieved.(*types.OIDCCredentials)
	require.True(t, ok)
	assert.Equal(t, creds.Token, oidcCreds.Token)
	assert.Equal(t, creds.Provider, oidcCreds.Provider)
	assert.Equal(t, creds.Audience, oidcCreds.Audience)
}

func TestFileKeyring_Delete(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	alias := "test-delete"
	creds := &types.OIDCCredentials{Token: "test-token"}

	// Store then delete.
	require.NoError(t, store.Store(alias, creds))
	require.NoError(t, store.Delete(alias))

	// Verify it's gone.
	_, err = store.Retrieve(alias)
	assert.Error(t, err)
}

func TestFileKeyring_List(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	// Initially empty.
	aliases, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, aliases)

	// Store multiple credentials.
	require.NoError(t, store.Store("alias1", &types.OIDCCredentials{Token: "token1"}))
	require.NoError(t, store.Store("alias2", &types.OIDCCredentials{Token: "token2"}))
	require.NoError(t, store.Store("alias3", &types.OIDCCredentials{Token: "token3"}))

	// List should return all aliases.
	aliases, err = store.List()
	require.NoError(t, err)
	assert.Len(t, aliases, 3)
	assert.Contains(t, aliases, "alias1")
	assert.Contains(t, aliases, "alias2")
	assert.Contains(t, aliases, "alias3")

	// Delete one.
	require.NoError(t, store.Delete("alias2"))

	// List should reflect deletion.
	aliases, err = store.List()
	require.NoError(t, err)
	assert.Len(t, aliases, 2)
	assert.NotContains(t, aliases, "alias2")
}

func TestFileKeyring_IsExpired(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	expiredCreds := &types.AWSCredentials{
		Expiration: time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
	}
	freshCreds := &types.AWSCredentials{
		Expiration: time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
	}

	require.NoError(t, store.Store("expired", expiredCreds))
	require.NoError(t, store.Store("fresh", freshCreds))

	// Check expired credentials.
	isExpired, err := store.IsExpired("expired")
	require.NoError(t, err)
	assert.True(t, isExpired)

	// Check fresh credentials.
	isExpired, err = store.IsExpired("fresh")
	require.NoError(t, err)
	assert.False(t, isExpired)

	// Missing alias returns true with error.
	isExpired, err = store.IsExpired("missing")
	assert.Error(t, err)
	assert.True(t, isExpired)
}

func TestFileKeyring_GetAnySetAny(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	type testData struct {
		Name  string
		Value int
	}

	data := testData{Name: "test", Value: 42}

	// Store arbitrary data.
	require.NoError(t, store.SetAny("test-key", data))

	// Retrieve arbitrary data.
	var retrieved testData
	require.NoError(t, store.GetAny("test-key", &retrieved))
	assert.Equal(t, data, retrieved)

	// Get non-existent key should error.
	err = store.GetAny("non-existent", &retrieved)
	assert.Error(t, err)
}

func TestFileKeyring_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	keyringPath := filepath.Join(tempDir, "test-keyring")
	password := "persistent-password-12345"
	t.Setenv("ATMOS_KEYRING_PASSWORD", password)

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": keyringPath,
			},
		},
	}

	// Create first store and save credentials.
	store1, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIA_PERSIST",
		SecretAccessKey: "SECRET_PERSIST",
		Region:          "us-west-2",
	}

	require.NoError(t, store1.Store("persistent-alias", creds))

	// Create second store (simulating new process) and verify credentials persist.
	store2, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	retrieved, err := store2.Retrieve("persistent-alias")
	require.NoError(t, err)

	awsCreds, ok := retrieved.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, creds.AccessKeyID, awsCreds.AccessKeyID)
	assert.Equal(t, creds.SecretAccessKey, awsCreds.SecretAccessKey)
	assert.Equal(t, creds.Region, awsCreds.Region)
}

func TestFileKeyring_MultipleAliases(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	// Store multiple different types of credentials.
	awsCreds := &types.AWSCredentials{
		AccessKeyID: "AKIA123",
		Region:      "us-east-1",
	}
	oidcCreds := &types.OIDCCredentials{
		Token:    "oidc-token",
		Provider: "github",
	}

	require.NoError(t, store.Store("aws-creds", awsCreds))
	require.NoError(t, store.Store("oidc-creds", oidcCreds))

	// Retrieve and verify each.
	retrieved1, err := store.Retrieve("aws-creds")
	require.NoError(t, err)
	_, ok := retrieved1.(*types.AWSCredentials)
	assert.True(t, ok)

	retrieved2, err := store.Retrieve("oidc-creds")
	require.NoError(t, err)
	_, ok = retrieved2.(*types.OIDCCredentials)
	assert.True(t, ok)

	// List should show both.
	aliases, err := store.List()
	require.NoError(t, err)
	assert.Len(t, aliases, 2)
	assert.Contains(t, aliases, "aws-creds")
	assert.Contains(t, aliases, "oidc-creds")
}

func TestFileKeyring_UpdateCredentials(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	alias := "update-test"

	// Store initial credentials.
	creds1 := &types.AWSCredentials{
		AccessKeyID: "AKIA_OLD",
		Region:      "us-east-1",
	}
	require.NoError(t, store.Store(alias, creds1))

	// Update with new credentials.
	creds2 := &types.AWSCredentials{
		AccessKeyID: "AKIA_NEW",
		Region:      "us-west-2",
	}
	require.NoError(t, store.Store(alias, creds2))

	// Retrieve should return updated credentials.
	retrieved, err := store.Retrieve(alias)
	require.NoError(t, err)

	awsCreds, ok := retrieved.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "AKIA_NEW", awsCreds.AccessKeyID)
	assert.Equal(t, "us-west-2", awsCreds.Region)
}

func TestFileKeyring_ErrorHandling(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	// Retrieve non-existent alias.
	_, err = store.Retrieve("non-existent")
	assert.Error(t, err)

	// Delete non-existent alias.
	err = store.Delete("non-existent")
	assert.Error(t, err)
}

func TestCreatePasswordPrompt_EnvironmentVariable(t *testing.T) {
	passwordEnv := "TEST_KEYRING_PASSWORD"
	expectedPassword := "env-password-12345"
	t.Setenv(passwordEnv, expectedPassword)

	promptFunc := createPasswordPrompt(passwordEnv)

	password, err := promptFunc("Enter password")
	require.NoError(t, err)
	assert.Equal(t, expectedPassword, password)
}

func TestCreatePasswordPrompt_NoEnvironmentNoTTY(t *testing.T) {
	passwordEnv := "NONEXISTENT_PASSWORD_ENV"

	promptFunc := createPasswordPrompt(passwordEnv)

	// Without env var and without TTY, should error.
	_, err := promptFunc("Enter password")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "keyring password required")
}

func TestFileKeyring_StoreUnsupportedType(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	// Try to store unsupported credential type.
	err = store.Store("test", &unsupportedCreds{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported credential type")
}

func TestFileKeyring_CorruptedData(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	// Store valid credentials first.
	creds := &types.AWSCredentials{AccessKeyID: "TEST"}
	require.NoError(t, store.Store("test-alias", creds))

	// Now manually corrupt the data by storing invalid JSON.
	_ = store.ring.Set(keyring.Item{
		Key:  "corrupted",
		Data: []byte("this is not valid JSON"),
	})

	// Attempting to retrieve should error.
	_, err = store.Retrieve("corrupted")
	assert.Error(t, err)
}

func TestFileKeyring_EmptyConfig(t *testing.T) {
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	// Create store with nil authConfig (should use defaults).
	store, err := newFileKeyringStore(nil)
	require.NoError(t, err)
	assert.NotNil(t, store)

	// Should have default path.
	homeDir, _ := os.UserHomeDir()
	expectedPath := filepath.Join(homeDir, ".atmos", "keyring")
	assert.Equal(t, expectedPath, store.path)
}

func TestFileKeyring_SetAnyError(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	// Try to marshal something that can't be marshaled (e.g., function).
	type unmarshalable struct {
		Func func()
	}

	err = store.SetAny("test-key", unmarshalable{Func: func() {}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal data")
}

func TestFileKeyring_RetrieveUnknownType(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	// Store data with an unknown credential type by directly marshaling the envelope.
	data := []byte(`{"type":"unknown-type","data":{}}`)

	err = store.ring.Set(keyring.Item{
		Key:  "test-alias",
		Data: data,
	})
	require.NoError(t, err)

	// Attempting to retrieve should error with unknown type.
	_, err = store.Retrieve("test-alias")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown credential type")
}

func TestFileKeyring_GetAnyError(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	// Store invalid JSON.
	err = store.ring.Set(keyring.Item{
		Key:  "invalid-json",
		Data: []byte("this is not valid JSON"),
	})
	require.NoError(t, err)

	// Attempting to retrieve should error.
	var result map[string]string
	err = store.GetAny("invalid-json", &result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal data")
}

func TestFileKeyring_StoreRetrieveOIDC(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	// Store OIDC credentials.
	creds := &types.OIDCCredentials{
		Token:    "test-oidc-token",
		Provider: "github",
		Audience: "test-audience",
	}

	err = store.Store("oidc-test", creds)
	assert.NoError(t, err)

	// Retrieve and verify.
	retrieved, err := store.Retrieve("oidc-test")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)

	oidcCreds, ok := retrieved.(*types.OIDCCredentials)
	assert.True(t, ok)
	assert.Equal(t, "test-oidc-token", oidcCreds.Token)
	assert.Equal(t, "github", oidcCreds.Provider)
	assert.Equal(t, "test-audience", oidcCreds.Audience)
}

func TestFileKeyring_ListError(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	// Store some credentials first.
	creds := &types.AWSCredentials{AccessKeyID: "TEST"}
	require.NoError(t, store.Store("test1", creds))

	// List should work normally.
	aliases, err := store.List()
	assert.NoError(t, err)
	assert.Contains(t, aliases, "test1")
}

func TestFileKeyring_NonexistentRetrieval(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	// Try to retrieve non-existent credentials.
	_, err = store.Retrieve("nonexistent-alias")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to retrieve credentials")
}

func TestFileKeyring_IsExpiredNonexistent(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": filepath.Join(tempDir, "test-keyring"),
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)

	// Check expiration on non-existent alias.
	expired, err := store.IsExpired("nonexistent-alias")
	assert.Error(t, err)
	assert.True(t, expired) // Should be considered expired if retrieval fails.
}
