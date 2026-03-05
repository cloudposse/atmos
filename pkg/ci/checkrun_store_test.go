package ci

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSyncMapCheckRunStore_StoreAndLoadAndDelete(t *testing.T) {
	store := &syncMapCheckRunStore{}

	// Store a value.
	store.Store("dev/vpc/plan", int64(42))

	// LoadAndDelete should return the value.
	id, ok := store.LoadAndDelete("dev/vpc/plan")
	assert.True(t, ok)
	assert.Equal(t, int64(42), id)

	// Second LoadAndDelete should return false.
	_, ok = store.LoadAndDelete("dev/vpc/plan")
	assert.False(t, ok)
}

func TestSyncMapCheckRunStore_LoadAndDelete_NotFound(t *testing.T) {
	store := &syncMapCheckRunStore{}

	id, ok := store.LoadAndDelete("nonexistent")
	assert.False(t, ok)
	assert.Equal(t, int64(0), id)
}

func TestSyncMapCheckRunStore_ConcurrentAccess(t *testing.T) {
	store := &syncMapCheckRunStore{}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "key-" + string(rune('A'+i%26))
			store.Store(key, int64(i))
			store.LoadAndDelete(key)
		}(i)
	}
	wg.Wait()
}
