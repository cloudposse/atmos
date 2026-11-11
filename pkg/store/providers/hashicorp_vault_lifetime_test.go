package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/store"
)

func scopedVault(t *testing.T, cleanup error) (*VaultStore, *fakeVaultKV, *int) {
	t.Helper()
	fake := newFakeVaultKV()
	fake.data["key"] = map[string]any{"apiToken": "synthetic-selected", "value": "synthetic-legacy"}
	s := newTestVaultStore(fake)
	s.identityLifetime = "operation"
	resolver := store.NewMockScopedEnvironmentAuthContextResolver(gomock.NewController(t))
	releases := new(int)
	resolver.EXPECT().WithEnvironmentAuthContext(gomock.Any(), "openbao", gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ string, operation func(context.Context, *store.EnvironmentAuthConfig) error) error {
			err := operation(ctx, &store.EnvironmentAuthConfig{Environment: map[string]string{"VAULT_TOKEN": "synthetic-identity"}})
			*releases++
			return errors.Join(err, cleanup)
		}).AnyTimes()
	s.SetAuthContext(resolver, "openbao")
	return s, fake, releases
}

func TestVaultOperationLifetimeReleasesBeforeReturningField(t *testing.T) {
	s, fake, releases := scopedVault(t, nil)
	for range 2 {
		value, err := s.GetKeyField("key", "apiToken")
		require.NoError(t, err)
		assert.Equal(t, "synthetic-selected", value)
		assert.Empty(t, fake.token, "the client must discard its leased token")
	}
	assert.Equal(t, 2, *releases)
	_, err := s.GetKeyField("key", "missing")
	require.ErrorIs(t, err, store.ErrFieldNotFound)
	assert.Equal(t, 3, *releases, "missing fields still release the identity")
	fake.getErr = assert.AnError
	_, err = s.GetKey("key")
	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, 4, *releases, "failed reads still release the identity")
}

func TestVaultOperationLifetimeSuppressesResultsOnCleanupFailure(t *testing.T) {
	s, fake, releases := scopedVault(t, assert.AnError)
	value, err := s.GetKeyField("key", "apiToken")
	require.ErrorIs(t, err, assert.AnError)
	assert.Nil(t, value, "a completed read cannot escape failed identity cleanup")
	assert.Equal(t, 1, *releases)
	assert.Empty(t, fake.token)
	found, err := s.Has("", "", "key")
	require.ErrorIs(t, err, assert.AnError)
	assert.False(t, found)
	keys, err := s.Keys("", "")
	require.ErrorIs(t, err, assert.AnError)
	assert.Nil(t, keys)
}

func TestVaultOperationLifetimeOwnsWritesAndDeletesToo(t *testing.T) {
	s, fake, releases := scopedVault(t, nil)
	require.NoError(t, s.Set("", "", "key", "synthetic-value"))
	require.NoError(t, s.Delete("", "", "key"))
	assert.Equal(t, 2, *releases)
	assert.Empty(t, fake.token)
}

func TestVaultOperationLifetimeNeverFallsBack(t *testing.T) {
	s, fake, _ := scopedVault(t, nil)
	s.ambientToken = "synthetic-ambient"
	s.SetAuthContext(store.NewMockEnvironmentAuthContextResolver(gomock.NewController(t)), "openbao")
	_, err := s.GetKey("key")
	require.ErrorIs(t, err, store.ErrAuthContextNotAvailable)
	assert.Zero(t, fake.getCalls)
	s.ResetAuthContext()
	_, err = s.GetKey("key")
	require.ErrorIs(t, err, store.ErrIdentityNotConfigured)
	assert.Zero(t, fake.getCalls)
}

func TestVaultLifetimeConfiguration(t *testing.T) {
	_, err := NewVaultStore(&VaultStoreOptions{Mount: "platform", IdentityLifetime: "unknown"}, "openbao")
	require.ErrorIs(t, err, store.ErrVaultIdentityLifetime)
	_, err = NewVaultStore(&VaultStoreOptions{Mount: "platform", IdentityLifetime: "operation"}, "")
	require.ErrorIs(t, err, store.ErrIdentityNotConfigured)
}

type cancellationVault struct {
	VaultKVClient
	entered chan struct{}
}

func (v cancellationVault) Get(ctx context.Context, _ string) (map[string]any, error) {
	close(v.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestVaultContextCancellationReleasesBeforeReturning(t *testing.T) {
	s, fake, releases := scopedVault(t, nil)
	entered := make(chan struct{})
	s.client = cancellationVault{VaultKVClient: fake, entered: entered}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := s.GetKeyFieldContext(ctx, "key", "apiToken")
		finished <- err
	}()
	<-entered
	cancel()
	require.ErrorIs(t, <-finished, context.Canceled)
	assert.Equal(t, 1, *releases)
	assert.Empty(t, fake.token)
}
