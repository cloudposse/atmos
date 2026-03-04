package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestRegisterAndNewStore(t *testing.T) {
	// Clean up registry state for this test.
	registryMu.Lock()
	oldFactories := factories
	factories = make(map[string]StoreFactory)
	registryMu.Unlock()
	defer func() {
		registryMu.Lock()
		factories = oldFactories
		registryMu.Unlock()
	}()

	called := false
	Register("test", func(opts StoreOptions) (Store, error) {
		called = true
		return nil, nil
	})

	_, err := NewStore(StoreOptions{Type: "test"})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestNewStoreNotFound(t *testing.T) {
	// Clean up registry state for this test.
	registryMu.Lock()
	oldFactories := factories
	factories = make(map[string]StoreFactory)
	registryMu.Unlock()
	defer func() {
		registryMu.Lock()
		factories = oldFactories
		registryMu.Unlock()
	}()

	_, err := NewStore(StoreOptions{Type: "nonexistent"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactStoreNotFound)
}

func TestRegisterPanicsOnEmptyType(t *testing.T) {
	assert.Panics(t, func() {
		Register("", func(opts StoreOptions) (Store, error) {
			return nil, nil
		})
	})
}

func TestRegisterPanicsOnNilFactory(t *testing.T) {
	assert.Panics(t, func() {
		Register("test", nil)
	})
}

func TestGetRegisteredTypes(t *testing.T) {
	// Clean up registry state for this test.
	registryMu.Lock()
	oldFactories := factories
	factories = make(map[string]StoreFactory)
	registryMu.Unlock()
	defer func() {
		registryMu.Lock()
		factories = oldFactories
		registryMu.Unlock()
	}()

	dummyFactory := func(opts StoreOptions) (Store, error) { return nil, nil }
	Register("s3", dummyFactory)
	Register("gcs", dummyFactory)

	types := GetRegisteredTypes()
	assert.Len(t, types, 2)
	assert.Contains(t, types, "s3")
	assert.Contains(t, types, "gcs")
}
