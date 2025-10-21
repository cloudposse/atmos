package credentials

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestMemoryKeyring_StoreRetrieve(t *testing.T) {
	store := newMemoryKeyringStore()

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

func TestMemoryKeyring_Delete(t *testing.T) {
	store := newMemoryKeyringStore()

	alias := "test-delete"
	creds := &types.OIDCCredentials{Token: "test-token", Provider: "github"}

	// Store then delete.
	require.NoError(t, store.Store(alias, creds))
	require.NoError(t, store.Delete(alias))

	// Verify it's gone.
	_, err := store.Retrieve(alias)
	assert.Error(t, err)

	// Delete non-existent should error.
	err = store.Delete("non-existent")
	assert.Error(t, err)
}

func TestMemoryKeyring_List(t *testing.T) {
	store := newMemoryKeyringStore()

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

//nolint:dupl // Test code intentionally duplicates interface behavior testing.
func TestMemoryKeyring_IsExpired(t *testing.T) {
	store := newMemoryKeyringStore()

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

func TestMemoryKeyring_GetAnySetAny(t *testing.T) {
	store := newMemoryKeyringStore()

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
	err := store.GetAny("non-existent", &retrieved)
	assert.Error(t, err)
}

func TestMemoryKeyring_ConcurrentAccess(t *testing.T) {
	store := newMemoryKeyringStore()

	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	// Concurrent writes.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				alias := fmt.Sprintf("alias-%d-%d", id, j)
				creds := &types.OIDCCredentials{Token: alias}
				store.Store(alias, creds)
			}
		}(i)
	}

	wg.Wait()

	// Verify all credentials stored.
	aliases, err := store.List()
	require.NoError(t, err)
	assert.Equal(t, numGoroutines*numOperations, len(aliases))

	// Concurrent reads.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				alias := fmt.Sprintf("alias-%d-%d", id, j)
				_, err := store.Retrieve(alias)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()
}

func TestMemoryKeyring_Isolation(t *testing.T) {
	// Create two independent memory stores.
	store1 := newMemoryKeyringStore()

	store2 := newMemoryKeyringStore()

	// Store in store1.
	creds := &types.OIDCCredentials{Token: "test-token"}
	require.NoError(t, store1.Store("test-alias", creds))

	// Should exist in store1.
	_, err := store1.Retrieve("test-alias")
	require.NoError(t, err)

	// Should NOT exist in store2 (isolated).
	_, err = store2.Retrieve("test-alias")
	assert.Error(t, err)

	// Lists should be independent.
	list1, err := store1.List()
	require.NoError(t, err)
	assert.Len(t, list1, 1)

	list2, err := store2.List()
	require.NoError(t, err)
	assert.Empty(t, list2)
}
