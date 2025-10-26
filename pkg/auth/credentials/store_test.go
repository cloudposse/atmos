package credentials

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/go-keyring"

	"github.com/cloudposse/atmos/pkg/auth/providers/mock"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// Ensure the keyring uses an in-memory mock backend for tests.
func init() {
	keyring.MockInit()
}

func TestNewCredentialStore(t *testing.T) {
	s := NewCredentialStore()
	assert.NotNil(t, s)
}

func TestStoreRetrieve_AWS(t *testing.T) {
	s := NewCredentialStore()
	alias := "aws-1"
	exp := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	in := &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "SECRET", SessionToken: "TOKEN", Region: "us-east-1", Expiration: exp}
	assert.NoError(t, s.Store(alias, in))

	got, err := s.Retrieve(alias)
	assert.NoError(t, err)
	out, ok := got.(*types.AWSCredentials)
	if assert.True(t, ok) {
		assert.Equal(t, in.AccessKeyID, out.AccessKeyID)
		assert.Equal(t, in.Region, out.Region)
		assert.Equal(t, in.Expiration, out.Expiration)
	}
}

func TestStoreRetrieve_OIDC(t *testing.T) {
	s := NewCredentialStore()
	alias := "oidc-1"
	in := &types.OIDCCredentials{Token: "hdr.payload.", Provider: "github", Audience: "sts"}
	assert.NoError(t, s.Store(alias, in))

	got, err := s.Retrieve(alias)
	assert.NoError(t, err)
	out, ok := got.(*types.OIDCCredentials)
	if assert.True(t, ok) {
		assert.Equal(t, in.Token, out.Token)
		assert.Equal(t, in.Provider, out.Provider)
		assert.Equal(t, in.Audience, out.Audience)
	}
}

// fakeCreds implements types.ICredentials but is not a supported concrete type.
type fakeCreds struct{}

func (f *fakeCreds) IsExpired() bool                                  { return false }
func (f *fakeCreds) GetExpiration() (*time.Time, error)               { return nil, nil }
func (f *fakeCreds) BuildWhoamiInfo(info *types.WhoamiInfo)           {}
func (f *fakeCreds) Validate(ctx context.Context) (*time.Time, error) { return nil, nil }

func TestStore_UnsupportedType(t *testing.T) {
	s := NewCredentialStore()
	err := s.Store("alias", &fakeCreds{})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrCredentialStore))
}

func TestDelete_Flow(t *testing.T) {
	s := NewCredentialStore()
	alias := "to-delete"
	// Delete non-existent -> success (treated as already deleted).
	assert.NoError(t, s.Delete(alias))

	// Store then delete -> ok.
	assert.NoError(t, s.Store(alias, &types.OIDCCredentials{Token: "hdr.payload."}))
	assert.NoError(t, s.Delete(alias))
	// Retrieve after delete -> error.
	_, err := s.Retrieve(alias)
	assert.Error(t, err)

	// Delete again -> success (idempotent).
	assert.NoError(t, s.Delete(alias))
}

func TestList_NotSupported(t *testing.T) {
	s := NewCredentialStore()
	_, err := s.List()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrCredentialStore))
}

// TestDefaultStore_Suite runs the shared test suite against the default credential store.
func TestDefaultStore_Suite(t *testing.T) {
	factory := func(t *testing.T) types.CredentialStore {
		return NewCredentialStore()
	}

	RunCredentialStoreTests(t, factory)
}

// TestNewCredentialStoreWithConfig_NoopFallback tests that credential store uses no-op keyring when system keyring is unavailable.
func TestNewCredentialStoreWithConfig_NoopFallback(t *testing.T) {
	tests := []struct {
		name           string
		keyringType    string
		expectNoop     bool
		expectRetrieve bool
	}{
		{
			name:           "memory keyring always works",
			keyringType:    "memory",
			expectNoop:     false,
			expectRetrieve: true,
		},
		{
			name:           "file keyring always works",
			keyringType:    "file",
			expectNoop:     false,
			expectRetrieve: true,
		},
		{
			name:           "system keyring works in test environment",
			keyringType:    "system",
			expectNoop:     false,
			expectRetrieve: true, // In test environment with MockInit, system keyring works
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_KEYRING_TYPE", tt.keyringType)
			// File keyring requires password.
			if tt.keyringType == "file" {
				t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password")
			}

			store := NewCredentialStore()
			assert.NotNil(t, store, "store should always be created")

			// Verify store is functional by storing and retrieving.
			alias := "test-fallback"
			creds := &types.OIDCCredentials{Token: "test-token", Provider: "test"}
			err := store.Store(alias, creds)
			assert.NoError(t, err, "store should be able to store credentials")

			if tt.expectRetrieve {
				retrieved, err := store.Retrieve(alias)
				assert.NoError(t, err, "store should be able to retrieve credentials")
				assert.NotNil(t, retrieved, "retrieved credentials should not be nil")
			}
		})
	}
}

// TestNewKeyringAuthStore tests the deprecated backward-compatible function.
func TestNewKeyringAuthStore(t *testing.T) {
	store := NewKeyringAuthStore()
	assert.NotNil(t, store, "NewKeyringAuthStore should return a store")

	// Should be able to perform basic operations.
	alias := "deprecated-test"
	creds := &types.OIDCCredentials{Token: "test-token", Provider: "test"}
	err := store.Store(alias, creds)
	assert.NoError(t, err, "should be able to store credentials")

	// Clean up.
	store.Delete(alias)
}

// TestNoopKeyringStore tests the no-op keyring behavior.
func TestNoopKeyringStore(t *testing.T) {
	store := newNoopKeyringStore()

	// Store succeeds (no-op).
	creds := &types.OIDCCredentials{Token: "test-token", Provider: "test"}
	err := store.Store("alias", creds)
	assert.NoError(t, err, "Store should succeed (no-op)")

	// Retrieve validates credentials and returns error.
	// Note: In test environment without AWS credentials, this will fail validation.
	retrieved, err := store.Retrieve("alias")
	assert.Error(t, err, "Retrieve should return error (no AWS credentials in test)")
	assert.Nil(t, retrieved, "Retrieved credentials should be nil")

	// Delete succeeds (no-op).
	err = store.Delete("alias")
	assert.NoError(t, err, "Delete should succeed (no-op)")

	// List returns empty.
	list, err := store.List()
	assert.NoError(t, err, "List should succeed")
	assert.Empty(t, list, "List should be empty")

	// IsExpired returns true with error.
	expired, err := store.IsExpired("alias")
	assert.True(t, expired, "IsExpired should return true")
	assert.ErrorIs(t, err, ErrCredentialsNotFound, "Should return credentials not found error")

	// GetAny returns credentials not found.
	var dest string
	err = store.GetAny("key", &dest)
	assert.ErrorIs(t, err, ErrCredentialsNotFound, "GetAny should return credentials not found")

	// SetAny succeeds (no-op).
	err = store.SetAny("key", "value")
	assert.NoError(t, err, "SetAny should succeed (no-op)")
}

// TestNoopKeyringStore_CacheBehavior tests the caching logic.
func TestNoopKeyringStore_CacheBehavior(t *testing.T) {
	store := newNoopKeyringStore()

	// First call attempts validation (may succeed or fail depending on environment).
	_, err1 := store.Retrieve("test-alias")

	// If validation failed, cache should be empty.
	// If validation succeeded, cache should have entry and error should be ErrCredentialsNotFound.
	if errors.Is(err1, ErrCredentialsNotFound) {
		// Validation succeeded - cache should be populated.
		assert.NotEmpty(t, store.cache, "Cache should be populated after successful validation")
		assert.Contains(t, store.cache, "test-alias", "Cache should contain the alias")
	} else {
		// Validation failed - cache should be empty.
		assert.Error(t, err1, "Retrieve should return error when validation fails")
		assert.Empty(t, store.cache, "Cache should be empty after failed validation")
	}
}

// TestNoopKeyringStore_ExpiredCache tests expired cache handling.
func TestNoopKeyringStore_ExpiredCache(t *testing.T) {
	store := newNoopKeyringStore()

	// Manually populate cache with expired credentials.
	expiredTime := time.Now().Add(-1 * time.Hour)
	store.cache["test-alias"] = cachedCredential{
		creds:       nil,
		expiration:  &expiredTime,
		validatedAt: time.Now().Add(-6 * time.Minute), // Older than 5-min cache
	}

	// Retrieve should attempt revalidation (will fail without AWS creds).
	_, err := store.Retrieve("test-alias")
	assert.Error(t, err, "Should attempt revalidation after cache expiry")
}

// TestNoopKeyringStore_ValidCache tests returning early when cache is valid.
func TestNoopKeyringStore_ValidCache(t *testing.T) {
	store := newNoopKeyringStore()

	// Manually populate cache with valid, non-expired credentials.
	futureTime := time.Now().Add(1 * time.Hour)
	store.cache["test-alias"] = cachedCredential{
		creds:       nil,
		expiration:  &futureTime,
		validatedAt: time.Now(), // Recent validation (< 5 min)
	}

	// Retrieve should return ErrCredentialsNotFound without revalidation.
	creds, err := store.Retrieve("test-alias")
	assert.Nil(t, creds, "Should return nil credentials")
	assert.ErrorIs(t, err, ErrCredentialsNotFound, "Should return credentials not found")
}

// TestNoopKeyringStore_ExpiredInCache tests returning error for expired cached credentials.
func TestNoopKeyringStore_ExpiredInCache(t *testing.T) {
	store := newNoopKeyringStore()

	// Manually populate cache with expired credentials but recent validation.
	expiredTime := time.Now().Add(-1 * time.Hour)
	store.cache["test-alias"] = cachedCredential{
		creds:       nil,
		expiration:  &expiredTime,
		validatedAt: time.Now(), // Recent validation (< 5 min)
	}

	// Retrieve should return expired error without revalidation.
	creds, err := store.Retrieve("test-alias")
	assert.Nil(t, creds, "Should return nil credentials")
	assert.Error(t, err, "Should return error for expired credentials")
	assert.Contains(t, err.Error(), "credentials expired", "Should mention expired credentials")
}

// TestNoopKeyringStore_StoreWithMockCredentials tests storing mock credentials and validating them.
func TestNoopKeyringStore_StoreWithMockCredentials(t *testing.T) {
	store := newNoopKeyringStore()

	// Create mock credentials with future expiration.
	futureTime := time.Now().Add(1 * time.Hour)
	mockCreds := &mock.Credentials{
		AccessKeyID:     "AKIATEST",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Region:          "us-east-1",
		Expiration:      futureTime,
	}

	// Store should succeed (no-op).
	err := store.Store("mock-alias", mockCreds)
	assert.NoError(t, err, "Store should succeed for mock credentials")

	// Manually populate cache to simulate successful validation.
	store.cache["mock-alias"] = cachedCredential{
		creds:       mockCreds,
		expiration:  &futureTime,
		validatedAt: time.Now(),
	}

	// Retrieve should return credentials not found (signals to use SDK).
	creds, err := store.Retrieve("mock-alias")
	assert.Nil(t, creds, "Should return nil credentials")
	assert.ErrorIs(t, err, ErrCredentialsNotFound, "Should return credentials not found")
}

// TestNoopKeyringStore_ExpirationWarning tests warning for expiring credentials.
func TestNoopKeyringStore_ExpirationWarning(t *testing.T) {
	store := newNoopKeyringStore()

	// Create mock credentials expiring in 10 minutes (< 15 min threshold).
	expiringTime := time.Now().Add(10 * time.Minute)
	mockCreds := &mock.Credentials{
		AccessKeyID:     "AKIATEST",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Region:          "us-east-1",
		Expiration:      expiringTime,
	}

	// Manually populate cache with expiring credentials.
	store.cache["expiring-alias"] = cachedCredential{
		creds:       mockCreds,
		expiration:  &expiringTime,
		validatedAt: time.Now(),
	}

	// Retrieve should warn about expiring credentials but still succeed.
	creds, err := store.Retrieve("expiring-alias")
	assert.Nil(t, creds, "Should return nil credentials")
	assert.ErrorIs(t, err, ErrCredentialsNotFound, "Should return credentials not found")
}

// TestSystemKeyringStore_GetAny tests retrieving arbitrary data from keyring.
func TestSystemKeyringStore_GetAny(t *testing.T) {
	store, err := newSystemKeyringStore()
	assert.NoError(t, err, "Should create system keyring store")

	// Store some data using SetAny.
	testData := map[string]string{"key": "value", "foo": "bar"}
	err = store.SetAny("test-data", testData)
	assert.NoError(t, err, "SetAny should succeed")

	// Retrieve the data using GetAny.
	var retrieved map[string]string
	err = store.GetAny("test-data", &retrieved)
	assert.NoError(t, err, "GetAny should succeed")
	assert.Equal(t, testData, retrieved, "Retrieved data should match stored data")

	// Clean up.
	keyring.Delete("test-data", KeyringUser)
}

// TestSystemKeyringStore_GetAny_NotFound tests GetAny with non-existent key.
func TestSystemKeyringStore_GetAny_NotFound(t *testing.T) {
	store, err := newSystemKeyringStore()
	assert.NoError(t, err, "Should create system keyring store")

	var dest string
	err = store.GetAny("non-existent-key", &dest)
	assert.Error(t, err, "GetAny should return error for non-existent key")
	assert.True(t, errors.Is(err, ErrCredentialStore), "Should return ErrCredentialStore")
}

// TestSystemKeyringStore_SetAny tests storing arbitrary data in keyring.
func TestSystemKeyringStore_SetAny(t *testing.T) {
	store, err := newSystemKeyringStore()
	assert.NoError(t, err, "Should create system keyring store")

	// Test with different data types.
	testCases := []struct {
		name string
		key  string
		data interface{}
	}{
		{"string", "test-string", "hello"},
		{"number", "test-number", 42},
		{"struct", "test-struct", struct{ Name string }{"test"}},
		{"map", "test-map", map[string]int{"a": 1, "b": 2}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := store.SetAny(tc.key, tc.data)
			assert.NoError(t, err, "SetAny should succeed for %s", tc.name)

			// Clean up.
			keyring.Delete(tc.key, KeyringUser)
		})
	}
}
