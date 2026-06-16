package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// withCleanRegistry swaps in an empty factory map for the duration of the test.
func withCleanRegistry(t *testing.T) {
	t.Helper()
	registryMu.Lock()
	old := factories
	factories = make(map[string]BackendFactory)
	registryMu.Unlock()
	t.Cleanup(func() {
		registryMu.Lock()
		factories = old
		registryMu.Unlock()
	})
}

func TestNewBackend_ReturnsUnwrappedBackend(t *testing.T) {
	withCleanRegistry(t)

	backend := &nonIdentityAwareBackend{}
	Register("plain", func(StoreOptions) (Backend, error) { return backend, nil })

	got, err := NewBackend(StoreOptions{Type: "plain"})
	require.NoError(t, err)
	// NewBackend returns the raw backend, not a BundledStore.
	assert.Same(t, backend, got)
}

func TestNewBackend_NotFound(t *testing.T) {
	withCleanRegistry(t)

	_, err := NewBackend(StoreOptions{Type: "nonexistent"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactStoreNotFound)
}

func TestNewBackend_InjectsResolverIntoIdentityAwareBackend(t *testing.T) {
	withCleanRegistry(t)

	recorder := &identityAwareRecorder{}
	Register("identity-aware", func(StoreOptions) (Backend, error) { return recorder, nil })

	_, err := NewBackend(StoreOptions{
		Type:     "identity-aware",
		Identity: "deploy",
		Resolver: stubResolver{},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, recorder.calls)
	assert.Equal(t, "deploy", recorder.receivedIdentity)
	assert.NotNil(t, recorder.receivedResolver)
}
