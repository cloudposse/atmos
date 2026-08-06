package store

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRegisterConcurrentWithNewStoreRegistry verifies that the storeFactories map
// is safe for concurrent access. Register (a write) and NewStoreRegistry (a read)
// run from many goroutines at once; without the guarding mutex this would trip the
// Go runtime's concurrent map read/write detector under `go test -race`.
func TestRegisterConcurrentWithNewStoreRegistry(t *testing.T) {
	const goroutines = 50

	t.Cleanup(Reset)

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

// noopFactory is a throwaway factory whose body is never invoked.
func noopFactory(_ string, _ StoreConfig) (Store, error) {
	return nil, nil //nolint:nilnil // Dummy factory.
}

// TestRegister_DuplicatePanics verifies that registering the same store type
// twice panics, surfacing the programming error of two factories claiming a type.
func TestRegister_DuplicatePanics(t *testing.T) {
	t.Cleanup(Reset)

	Register("dup-type", noopFactory)

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected a panic on duplicate registration")
	}()

	Register("dup-type", noopFactory) // Second registration must panic.
}

// TestReset_ClearsFactories verifies that Reset removes all registered factories.
func TestReset_ClearsFactories(t *testing.T) {
	t.Cleanup(Reset)

	Register("reset-type", noopFactory)

	// Sanity check: the type resolves before Reset.
	_, err := NewStoreRegistry(&StoresConfig{"s": StoreConfig{Type: "reset-type"}})
	assert.NoError(t, err)

	Reset()

	// After Reset the type is gone.
	_, err = NewStoreRegistry(&StoresConfig{"s": StoreConfig{Type: "reset-type"}})
	assert.ErrorIs(t, err, ErrStoreTypeNotFound)
}

// TestNewStoreRegistry_UnknownType verifies the not-found error for unregistered types.
func TestNewStoreRegistry_UnknownType(t *testing.T) {
	t.Cleanup(Reset)
	Reset()

	_, err := NewStoreRegistry(&StoresConfig{"s": StoreConfig{Type: "no-such-type"}})
	assert.ErrorIs(t, err, ErrStoreTypeNotFound)
}

// TestNewStoreRegistry_FactoryError verifies that a factory error propagates.
func TestNewStoreRegistry_FactoryError(t *testing.T) {
	t.Cleanup(Reset)

	sentinel := errors.New("factory boom")
	Register("err-type", func(_ string, _ StoreConfig) (Store, error) {
		return nil, sentinel
	})

	_, err := NewStoreRegistry(&StoresConfig{"s": StoreConfig{Type: "err-type"}})
	assert.ErrorIs(t, err, sentinel)
}
