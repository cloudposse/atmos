package store

import (
	"fmt"
	"sync"
	"testing"
)

// TestRegisterConcurrentWithNewStoreRegistry verifies that the storeFactories map
// is safe for concurrent access. Register (a write) and NewStoreRegistry (a read)
// run from many goroutines at once; without the guarding mutex this would trip the
// Go runtime's concurrent map read/write detector under `go test -race`.
func TestRegisterConcurrentWithNewStoreRegistry(t *testing.T) {
	const goroutines = 50

	factory := func(_ string, _ StoreConfig) (Store, error) {
		return nil, nil //nolint:nilnil // Dummy factory; never invoked in this test.
	}

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		// Writer: register a unique throwaway type to avoid the duplicate panic.
		go func(n int) {
			defer wg.Done()
			Register(fmt.Sprintf("concurrent-test-type-%d", n), factory)
		}(i)

		// Reader: NewStoreRegistry reads storeFactories while writers mutate it.
		go func() {
			defer wg.Done()
			config := &StoresConfig{
				"probe": StoreConfig{Type: "definitely-not-registered"},
			}
			// Returns ErrStoreTypeNotFound; we only care that the map read is race-free.
			_, _ = NewStoreRegistry(config)
		}()
	}

	wg.Wait()
}
