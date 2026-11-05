package registry

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/providers/mock"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func batchProvider(name string, config *schema.Provider) (types.Provider, error) {
	return mock.NewProvider(name, config), nil
}

func batchIdentity(name string, config *schema.Identity) (types.Identity, error) {
	return mock.NewIdentity(name, config), nil
}

func TestBatchRegistrationIsAtomicAndCleanupIsOwned(t *testing.T) {
	t.Parallel()
	var registry Registry
	batch := Registration{Providers: map[string]ProviderConfigConstructor{"custom/provider": batchProvider}, Identities: map[string]IdentityConfigConstructor{"custom/identity": batchIdentity}}
	cleanup, err := registry.RegisterBatch(batch)
	require.NoError(t, err)
	provider, err := registry.NewProvider("login", &schema.Provider{Kind: "custom/provider"})
	require.NoError(t, err)
	require.Equal(t, "login", provider.Name())
	identity, err := registry.NewIdentity("deploy", &schema.Identity{Kind: "custom/identity"})
	require.NoError(t, err)
	require.IsType(t, &mock.Identity{}, identity)
	// Caller mutations cannot change which registrations this cleanup owns.
	batch.Providers["unrelated/provider"] = batchProvider
	delete(batch.Providers, "custom/provider")
	require.NoError(t, registry.RegisterProviderConfig("unrelated/provider", batchProvider))
	require.NoError(t, registry.RegisterIdentityConfig("unrelated/identity", batchIdentity))
	cleanup()
	require.False(t, registry.HasProvider("custom/provider"))
	require.False(t, registry.HasIdentity("custom/identity"))
	require.True(t, registry.HasProvider("unrelated/provider"))
	require.True(t, registry.HasIdentity("unrelated/identity"))
	// Reusing a released kind must not let an old cleanup delete a new owner.
	replacement, err := registry.RegisterBatch(Registration{Providers: map[string]ProviderConfigConstructor{"custom/provider": batchProvider}})
	require.NoError(t, err)
	defer replacement()
	cleanup()
	require.True(t, registry.HasProvider("custom/provider"))
}

func TestInvalidBatchDoesNotPublishPartialRegistrations(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		batch    Registration
		expected error
	}{
		{"provider collision", Registration{Providers: map[string]ProviderConfigConstructor{"mock": batchProvider}, Identities: map[string]IdentityConfigConstructor{"custom/identity": batchIdentity}}, errUtils.ErrDuplicateAuthRegistration},
		{"identity collision", Registration{Providers: map[string]ProviderConfigConstructor{"custom/provider": batchProvider}, Identities: map[string]IdentityConfigConstructor{"mock": batchIdentity}}, errUtils.ErrDuplicateAuthRegistration},
		{"empty provider kind", Registration{Providers: map[string]ProviderConfigConstructor{"": batchProvider}}, errUtils.ErrInvalidAuthRegistration},
		{"nil identity constructor", Registration{Providers: map[string]ProviderConfigConstructor{"custom/provider": batchProvider}, Identities: map[string]IdentityConfigConstructor{"custom/identity": nil}}, errUtils.ErrInvalidAuthRegistration},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			registry := NewRegistry()
			cleanup, err := registry.RegisterBatch(test.batch)
			require.ErrorIs(t, err, test.expected)
			require.Nil(t, cleanup)
			require.False(t, registry.HasProvider("custom/provider"))
			require.False(t, registry.HasIdentity("custom/identity"))
			value, err := registry.NewProvider("original", &schema.Provider{Kind: "mock"})
			require.NoError(t, err)
			require.Equal(t, "original", value.Name())
		})
	}
}

func TestProcessWideBatchRegistrationIsVisibleToConstructors(t *testing.T) {
	cleanup, err := RegisterBatch(Registration{Providers: map[string]ProviderConfigConstructor{"test/batch-provider": batchProvider}, Identities: map[string]IdentityConfigConstructor{"test/batch-identity": batchIdentity}})
	require.NoError(t, err)
	t.Cleanup(cleanup)
	provider, err := NewProvider("login", &schema.Provider{Kind: "test/batch-provider"})
	require.NoError(t, err)
	require.Equal(t, "login", provider.Name())
	identity, err := NewIdentity("deploy", &schema.Identity{Kind: "test/batch-identity"})
	require.NoError(t, err)
	require.IsType(t, &mock.Identity{}, identity)
	require.ErrorIs(t, RegisterProvider("test/batch-provider", batchProvider), errUtils.ErrDuplicateAuthRegistration)
	require.ErrorIs(t, RegisterIdentity("test/batch-identity", batchIdentity), errUtils.ErrDuplicateAuthRegistration)
}
