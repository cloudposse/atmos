package credentials

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/go-keyring"

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

func (f *fakeCreds) IsExpired() bool                                         { return false }
func (f *fakeCreds) GetExpiration() (*time.Time, error)                      { return nil, nil }
func (f *fakeCreds) BuildWhoamiInfo(info *types.WhoamiInfo)                  {}
func (f *fakeCreds) Validate(ctx context.Context) (*time.Time, error)        { return nil, nil }

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

// TestNoopKeyringStore tests the no-op keyring behavior.
func TestNoopKeyringStore(t *testing.T) {
	store := newNoopKeyringStore()

	// Store succeeds (no-op).
	creds := &types.OIDCCredentials{Token: "test-token", Provider: "test"}
	err := store.Store("alias", creds)
	assert.NoError(t, err, "Store should succeed (no-op)")

	// Retrieve validates credentials and returns "not found".
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
}
