package credentials

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

// StoreFactory creates a credential store for testing.
type StoreFactory func(t *testing.T) types.CredentialStore

// RunCredentialStoreTests runs the full credential store test suite against any implementation.
func RunCredentialStoreTests(t *testing.T, factory StoreFactory) {
	t.Run("StoreAndRetrieve", func(t *testing.T) {
		testStoreAndRetrieve(t, factory)
	})

	t.Run("IsExpired", func(t *testing.T) {
		testIsExpired(t, factory)
	})

	t.Run("Delete", func(t *testing.T) {
		testDelete(t, factory)
	})
}

// testStoreAndRetrieve tests basic store and retrieve operations.
func testStoreAndRetrieve(t *testing.T, factory StoreFactory) {
	store := factory(t)

	alias := "test-aws"
	exp := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIA123",
		SecretAccessKey: "SECRET",
		SessionToken:    "TOKEN",
		Region:          "us-east-1",
		Expiration:      exp,
	}

	// Store credentials.
	err := store.Store(alias, creds)
	require.NoError(t, err)

	// Retrieve credentials.
	retrieved, err := store.Retrieve(alias)
	require.NoError(t, err)

	awsCreds, ok := retrieved.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, creds.AccessKeyID, awsCreds.AccessKeyID)
	assert.Equal(t, creds.SecretAccessKey, awsCreds.SecretAccessKey)
	assert.Equal(t, creds.Region, awsCreds.Region)
}

// testIsExpired tests credential expiration checking.
func testIsExpired(t *testing.T, factory StoreFactory) {
	store := factory(t)

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

// testDelete tests credential deletion.
func testDelete(t *testing.T, factory StoreFactory) {
	store := factory(t)

	alias := "test-delete"
	creds := &types.OIDCCredentials{Token: "test-token", Provider: "github"}

	// Store then delete.
	require.NoError(t, store.Store(alias, creds))
	require.NoError(t, store.Delete(alias))

	// Verify it's gone.
	_, err := store.Retrieve(alias)
	assert.Error(t, err)

	// Delete non-existent should succeed (idempotent).
	err = store.Delete("non-existent")
	assert.NoError(t, err)
}
