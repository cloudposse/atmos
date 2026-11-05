package registry

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/providers/mock"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	_ = schema.Provider{Kind: "", StartURL: "", Session: nil}
	_ = schema.Identity{Kind: "", Via: nil, Credentials: nil}
)

func TestRegistryPreservesFullConfiguration(t *testing.T) {
	f := NewRegistry()
	providerConfig := &schema.Provider{Kind: "custom/provider", StartURL: "https://example.com/start", Region: "us-east-1", Session: &schema.SessionConfig{Duration: "1h"}}
	identityConfig := &schema.Identity{Kind: "custom/identity", Via: &schema.IdentityVia{Provider: "custom"}, Credentials: map[string]any{"profile": "prod"}}
	require.NoError(t, f.RegisterProviderConfig(providerConfig.Kind, func(name string, config *schema.Provider) (types.Provider, error) {
		assert.Same(t, providerConfig, config)
		assert.Equal(t, "custom", name)
		// Constructor invocation must not hold the registry lock.
		assert.True(t, f.HasProvider("mock"))
		return mock.NewProvider(name, config), nil
	}))
	require.NoError(t, f.RegisterIdentityConfig(identityConfig.Kind, func(name string, config *schema.Identity) (types.Identity, error) {
		assert.Same(t, identityConfig, config)
		assert.Equal(t, "identity", name)
		assert.True(t, f.HasIdentity("mock"))
		return mock.NewIdentity(name, config), nil
	}))
	provider, err := f.NewProvider("custom", providerConfig)
	require.NoError(t, err)
	require.NotNil(t, provider)
	identity, err := f.NewIdentity("identity", identityConfig)
	require.NoError(t, err)
	require.NotNil(t, identity)
}

func TestRegistryRejectsDuplicatesAndInvalidConstructors(t *testing.T) {
	f := NewRegistry()
	provider := func(name string, config *schema.Provider) (types.Provider, error) {
		return mock.NewProvider(name, config), nil
	}
	identity := func(name string, config *schema.Identity) (types.Identity, error) {
		return mock.NewIdentity(name, config), nil
	}
	require.ErrorIs(t, f.RegisterProviderConfig("mock", provider), errUtils.ErrDuplicateAuthRegistration)
	require.ErrorIs(t, f.RegisterIdentityConfig("mock", identity), errUtils.ErrDuplicateAuthRegistration)
	require.ErrorIs(t, f.RegisterProviderConfig("", provider), errUtils.ErrInvalidAuthRegistration)
	require.ErrorIs(t, f.RegisterIdentityConfig("", identity), errUtils.ErrInvalidAuthRegistration)
	require.ErrorIs(t, f.RegisterProviderConfig("custom", nil), errUtils.ErrInvalidAuthRegistration)
	require.ErrorIs(t, f.RegisterIdentityConfig("custom", nil), errUtils.ErrInvalidAuthRegistration)
	require.ErrorIs(t, f.RegisterProvider("custom", nil), errUtils.ErrInvalidAuthRegistration)
	require.ErrorIs(t, f.RegisterIdentity("custom", nil), errUtils.ErrInvalidAuthRegistration)
	require.ErrorIs(t, RegisterGCPProviders(f), errUtils.ErrDuplicateAuthRegistration)
	require.ErrorIs(t, RegisterGCPIdentities(f), errUtils.ErrDuplicateAuthRegistration)
	require.ErrorIs(t, RegisterEmulatorIdentities(f), errUtils.ErrDuplicateAuthRegistration)
	// A duplicate registration must leave the original constructor usable.
	original, err := f.NewProvider("original", &schema.Provider{Kind: "mock"})
	require.NoError(t, err)
	assert.Equal(t, "original", original.Name())
}

func TestRegistryConcurrentRegistration(t *testing.T) {
	var f Registry
	var wg sync.WaitGroup
	results := make(chan error, 16)
	for range 16 {
		wg.Go(func() {
			results <- f.RegisterProvider("custom", func(name string, spec map[string]any) (types.Provider, error) {
				return mock.NewProvider(name, &schema.Provider{Kind: "custom", Spec: spec}), nil
			})
		})
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, errUtils.ErrDuplicateAuthRegistration)
		}
	}
	assert.Equal(t, 1, successes)
	provider, err := f.CreateProvider("custom", "winner", nil)
	require.NoError(t, err)
	assert.Equal(t, "winner", provider.Name())
}

func TestBuiltinLookupDoesNotInventMissingKinds(t *testing.T) {
	require.True(t, HasProvider("aws/iam-identity-center"))
	require.True(t, HasIdentity("aws/assume-role"))
	require.False(t, HasProvider("example/missing-provider"))
	require.False(t, HasIdentity("example/missing-identity"))
}
