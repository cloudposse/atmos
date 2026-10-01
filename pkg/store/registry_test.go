package store

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	log "github.com/cloudposse/atmos/pkg/logger"
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
			// The unregistered type is skipped with a warning, not an error; we only care
			// that the map read is race-free.
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
	registry, err := NewStoreRegistry(&StoresConfig{"s": StoreConfig{Type: "reset-type"}})
	assert.NoError(t, err)
	assert.Contains(t, registry, "s")

	Reset()

	// After Reset the type no longer resolves: the store is skipped (warned), not fatal.
	registry, err = NewStoreRegistry(&StoresConfig{"s": StoreConfig{Type: "reset-type"}})
	assert.NoError(t, err)
	assert.NotContains(t, registry, "s")
}

// TestNewStoreRegistry_UnknownTypeIsSkippedWithoutError verifies that an unresolvable store
// kind is omitted from the registry with a warning instead of failing the whole build. See
// https://github.com/cloudposse/atmos/issues/2930.
func TestNewStoreRegistry_UnknownTypeIsSkippedWithoutError(t *testing.T) {
	t.Cleanup(Reset)
	Reset()

	registry, err := NewStoreRegistry(&StoresConfig{"s": StoreConfig{Type: "no-such-type"}})
	assert.NoError(t, err, "an unresolvable store must not fail the whole registry build")
	assert.NotContains(t, registry, "s")
}

// TestNewStoreRegistry_FactoryErrorIsSkippedWithoutError verifies that a factory error for one
// store is warned and skipped rather than aborting the whole registry build.
func TestNewStoreRegistry_FactoryErrorIsSkippedWithoutError(t *testing.T) {
	t.Cleanup(Reset)

	sentinel := errors.New("factory boom")
	Register("err-type", func(_ string, _ StoreConfig) (Store, error) {
		return nil, sentinel
	})

	registry, err := NewStoreRegistry(&StoresConfig{"s": StoreConfig{Type: "err-type"}})
	assert.NoError(t, err, "a factory error for one store must not fail the whole registry build")
	assert.NotContains(t, registry, "s")
}

// TestNewStoreRegistry_SecretIncapableIsSkippedWithoutError verifies that marking a
// non-encrypting backend `secret: true` is warned and skipped, not fatal to the build.
func TestNewStoreRegistry_SecretIncapableIsSkippedWithoutError(t *testing.T) {
	t.Cleanup(Reset)
	Reset()

	Register(KindRedis, noopFactory)

	registry, err := NewStoreRegistry(&StoresConfig{"s": StoreConfig{Type: KindRedis, Secret: true}})
	assert.NoError(t, err)
	assert.NotContains(t, registry, "s")
}

// TestNewStoreRegistry_OneBadStoreDoesNotBlockOthers verifies the blast-radius hardening: a
// single misconfigured store (unresolvable kind) must not prevent the other, valid stores in
// the same config from being built. Before this fix, one bad store — even one nothing in any
// stack referenced — failed the entire atmos.yaml config load. See
// https://github.com/cloudposse/atmos/issues/2930.
func TestNewStoreRegistry_OneBadStoreDoesNotBlockOthers(t *testing.T) {
	t.Cleanup(Reset)
	Reset()

	Register("good-type", noopFactory)

	registry, err := NewStoreRegistry(&StoresConfig{
		"bad":  StoreConfig{Type: "no-such-type"},
		"good": StoreConfig{Type: "good-type"},
	})

	assert.NoError(t, err)
	assert.NotContains(t, registry, "bad")
	assert.Contains(t, registry, "good")
}

// TestNewStoreRegistry_UnresolvedKindWarningNamesStore verifies the diagnosability hardening:
// the warning logged for an unresolvable store kind names the specific store, not just the
// kind, so a config with several stores can be triaged from the log alone. See
// https://github.com/cloudposse/atmos/issues/2930.
func TestNewStoreRegistry_UnresolvedKindWarningNamesStore(t *testing.T) {
	t.Cleanup(Reset)
	Reset()

	originalLogger := log.Default()
	buffer := &bytes.Buffer{}
	testLogger := log.New()
	testLogger.SetOutput(buffer)
	testLogger.SetLevel(log.WarnLevel)
	testLogger.SetReportTimestamp(false)
	log.SetDefault(testLogger)
	t.Cleanup(func() { log.SetDefault(originalLogger) })

	_, err := NewStoreRegistry(&StoresConfig{"my-broken-store": StoreConfig{Type: "no-such-type"}})
	assert.NoError(t, err)

	logged := buffer.String()
	assert.Contains(t, logged, "my-broken-store", "the warning must name the specific store, not just its kind")
}
