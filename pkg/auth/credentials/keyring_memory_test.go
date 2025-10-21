package credentials

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

// TestMemoryKeyring_Suite runs the shared credential store test suite.
func TestMemoryKeyring_Suite(t *testing.T) {
	factory := func(t *testing.T) types.CredentialStore {
		return newMemoryKeyringStore()
	}

	RunCredentialStoreTests(t, factory)
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

// TestMemoryKeyring_GetAnySetAny tests arbitrary data storage (memory-specific helper method).
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

// TestMemoryKeyring_ConcurrentAccess tests thread-safe concurrent operations (memory-specific).
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
