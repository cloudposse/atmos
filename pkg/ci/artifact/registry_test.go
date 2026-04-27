package artifact

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestRegisterAndNewStore(t *testing.T) {
	// Clean up registry state for this test.
	registryMu.Lock()
	oldFactories := factories
	factories = make(map[string]BackendFactory)
	registryMu.Unlock()
	defer func() {
		registryMu.Lock()
		factories = oldFactories
		registryMu.Unlock()
	}()

	called := false
	Register("test", func(opts StoreOptions) (Backend, error) {
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
	factories = make(map[string]BackendFactory)
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
		Register("", func(opts StoreOptions) (Backend, error) {
			return nil, nil
		})
	})
}

func TestRegisterPanicsOnNilFactory(t *testing.T) {
	assert.Panics(t, func() {
		Register("test", nil)
	})
}

// identityAwareRecorder is a Backend + IdentityAwareBackend test double that
// records what the registry injects via SetAuthContext.
type identityAwareRecorder struct {
	calls            int
	receivedResolver AuthContextResolver
	receivedIdentity string
}

func (r *identityAwareRecorder) Name() string { return "test-identity-aware" }
func (r *identityAwareRecorder) Upload(_ context.Context, _ string, _ io.Reader, _ int64, _ *Metadata) error {
	return nil
}

func (r *identityAwareRecorder) Download(_ context.Context, _ string) (io.ReadCloser, *Metadata, error) {
	return nil, nil, nil
}
func (r *identityAwareRecorder) Delete(_ context.Context, _ string) error { return nil }
func (r *identityAwareRecorder) List(_ context.Context, _ Query) ([]ArtifactInfo, error) {
	return nil, nil
}
func (r *identityAwareRecorder) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (r *identityAwareRecorder) GetMetadata(_ context.Context, _ string) (*Metadata, error) {
	return nil, nil
}

func (r *identityAwareRecorder) SetAuthContext(resolver AuthContextResolver, identityName string) {
	r.calls++
	r.receivedResolver = resolver
	r.receivedIdentity = identityName
}

// nonIdentityAwareBackend is a Backend that does not implement IdentityAwareBackend.
type nonIdentityAwareBackend struct{}

func (b *nonIdentityAwareBackend) Name() string { return "test-plain" }
func (b *nonIdentityAwareBackend) Upload(_ context.Context, _ string, _ io.Reader, _ int64, _ *Metadata) error {
	return nil
}

func (b *nonIdentityAwareBackend) Download(_ context.Context, _ string) (io.ReadCloser, *Metadata, error) {
	return nil, nil, nil
}
func (b *nonIdentityAwareBackend) Delete(_ context.Context, _ string) error { return nil }
func (b *nonIdentityAwareBackend) List(_ context.Context, _ Query) ([]ArtifactInfo, error) {
	return nil, nil
}
func (b *nonIdentityAwareBackend) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (b *nonIdentityAwareBackend) GetMetadata(_ context.Context, _ string) (*Metadata, error) {
	return nil, nil
}

// stubResolver is a minimal AuthContextResolver used as an opaque value.
type stubResolver struct{}

func (stubResolver) ResolveAWSAuthContext(_ context.Context, _ string) (*AWSAuthConfig, error) {
	return nil, nil
}

func (stubResolver) ResolveAzureAuthContext(_ context.Context, _ string) (*store.AzureAuthConfig, error) {
	return nil, nil
}

func (stubResolver) ResolveGCPAuthContext(_ context.Context, _ string) (*store.GCPAuthConfig, error) {
	return nil, nil
}

func TestNewStore_InjectsResolverIntoIdentityAwareBackend(t *testing.T) {
	registryMu.Lock()
	oldFactories := factories
	factories = make(map[string]BackendFactory)
	registryMu.Unlock()
	defer func() {
		registryMu.Lock()
		factories = oldFactories
		registryMu.Unlock()
	}()

	recorder := &identityAwareRecorder{}
	Register("identity-aware", func(_ StoreOptions) (Backend, error) {
		return recorder, nil
	})

	resolver := stubResolver{}
	_, err := NewStore(StoreOptions{
		Type:     "identity-aware",
		Identity: "deploy",
		Resolver: resolver,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, recorder.calls, "registry must call SetAuthContext exactly once")
	assert.Equal(t, "deploy", recorder.receivedIdentity, "identity must propagate to backend")
	assert.NotNil(t, recorder.receivedResolver, "resolver must propagate to backend")
}

func TestNewStore_NoInjectionWhenResolverNil(t *testing.T) {
	registryMu.Lock()
	oldFactories := factories
	factories = make(map[string]BackendFactory)
	registryMu.Unlock()
	defer func() {
		registryMu.Lock()
		factories = oldFactories
		registryMu.Unlock()
	}()

	recorder := &identityAwareRecorder{}
	Register("identity-aware", func(_ StoreOptions) (Backend, error) {
		return recorder, nil
	})

	_, err := NewStore(StoreOptions{
		Type:     "identity-aware",
		Identity: "deploy",
		// Resolver intentionally nil
	})
	require.NoError(t, err)

	assert.Equal(t, 0, recorder.calls, "SetAuthContext must not be called without a resolver")
}

func TestNewStore_IgnoresResolverOnNonIdentityAwareBackend(t *testing.T) {
	registryMu.Lock()
	oldFactories := factories
	factories = make(map[string]BackendFactory)
	registryMu.Unlock()
	defer func() {
		registryMu.Lock()
		factories = oldFactories
		registryMu.Unlock()
	}()

	Register("plain", func(_ StoreOptions) (Backend, error) {
		return &nonIdentityAwareBackend{}, nil
	})

	storeResult, err := NewStore(StoreOptions{
		Type:     "plain",
		Identity: "deploy",
		Resolver: stubResolver{},
	})
	require.NoError(t, err)
	assert.NotNil(t, storeResult)
}

func TestGetRegisteredTypes(t *testing.T) {
	// Clean up registry state for this test.
	registryMu.Lock()
	oldFactories := factories
	factories = make(map[string]BackendFactory)
	registryMu.Unlock()
	defer func() {
		registryMu.Lock()
		factories = oldFactories
		registryMu.Unlock()
	}()

	dummyFactory := func(opts StoreOptions) (Backend, error) { return nil, nil }
	Register("s3", dummyFactory)
	Register("gcs", dummyFactory)

	types := GetRegisteredTypes()
	assert.Len(t, types, 2)
	assert.Contains(t, types, "s3")
	assert.Contains(t, types, "gcs")
}
