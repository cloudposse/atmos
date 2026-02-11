package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestNoopKeyring_Type(t *testing.T) {
	store := newNoopKeyringStore()

	// Noop keyring returns the noop type constant.
	storeType := store.Type()
	assert.Equal(t, types.CredentialStoreTypeNoop, storeType)
}

func TestNoopKeyring_Store(t *testing.T) {
	store := newNoopKeyringStore()

	// Store should always succeed (no-op).
	creds := &types.OIDCCredentials{Token: "test-token"}
	err := store.Store("test-alias", creds, "test-realm")
	assert.NoError(t, err)
}

func TestNoopKeyring_Retrieve(t *testing.T) {
	store := newNoopKeyringStore()

	// Retrieve should always return ErrCredentialsNotFound.
	creds, err := store.Retrieve("test-alias", "test-realm")
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, ErrCredentialsNotFound)
}

func TestNoopKeyring_Delete(t *testing.T) {
	store := newNoopKeyringStore()

	// Delete should always succeed (no-op).
	err := store.Delete("test-alias", "test-realm")
	assert.NoError(t, err)
}

func TestNoopKeyring_List(t *testing.T) {
	store := newNoopKeyringStore()

	// List should always return empty list.
	list, err := store.List("test-realm")
	assert.NoError(t, err)
	assert.Empty(t, list)
}

func TestNoopKeyring_IsExpired(t *testing.T) {
	store := newNoopKeyringStore()

	// IsExpired should always return true with ErrCredentialsNotFound.
	expired, err := store.IsExpired("test-alias", "test-realm")
	assert.True(t, expired)
	assert.ErrorIs(t, err, ErrCredentialsNotFound)
}

func TestNoopKeyring_GetAny(t *testing.T) {
	store := newNoopKeyringStore()

	// GetAny should always return ErrCredentialsNotFound.
	var dest string
	err := store.GetAny("test-key", &dest)
	assert.ErrorIs(t, err, ErrCredentialsNotFound)
}

func TestNoopKeyring_SetAny(t *testing.T) {
	store := newNoopKeyringStore()

	// SetAny should always succeed (no-op).
	err := store.SetAny("test-key", "test-value")
	assert.NoError(t, err)
}
