package auth

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// countingIdentity tracks how many times Authenticate is called.
type countingIdentity struct {
	provider  string
	callCount atomic.Int32
	creds     types.ICredentials
}

func (c *countingIdentity) Kind() string                     { return "aws/assume-role" }
func (c *countingIdentity) GetProviderName() (string, error) { return c.provider, nil }
func (c *countingIdentity) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
	c.callCount.Add(1)
	return c.creds, nil
}
func (c *countingIdentity) Validate() error                         { return nil }
func (c *countingIdentity) Environment() (map[string]string, error) { return nil, nil }
func (c *countingIdentity) Paths() ([]types.Path, error)            { return []types.Path{}, nil }
func (c *countingIdentity) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	return nil
}
func (c *countingIdentity) Logout(_ context.Context) error  { return nil }
func (c *countingIdentity) CredentialsExist() (bool, error) { return false, nil }
func (c *countingIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	return nil, nil
}

func (c *countingIdentity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
}
func (c *countingIdentity) SetRealm(_ string) {}

func TestProcessCredentialCache_AvoidsDuplicateAuth(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(1 * time.Hour)
	identityCreds := &testCreds{exp: &exp}
	identity := &countingIdentity{provider: "prov", creds: identityCreds}

	providerCreds := &testCreds{}
	provider := &testProvider{name: "prov", creds: providerCreds}

	store := &testStore{data: map[string]any{}}

	// Create first manager and authenticate.
	m1 := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"role": {Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "prov"}},
			},
		},
		providers:       map[string]types.Provider{"prov": provider},
		identities:      map[string]types.Identity{"role": identity},
		credentialStore: store,
		chain:           []string{"prov", "role"},
		realm:           realm.RealmInfo{Value: "test-realm"},
	}

	creds1, err := m1.authenticateChain(context.Background(), "role")
	require.NoError(t, err)
	assert.Equal(t, identityCreds, creds1)
	assert.Equal(t, int32(1), identity.callCount.Load(), "identity.Authenticate should be called once")

	// Create second manager with same chain and realm (simulates new manager for nested component).
	m2 := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"role": {Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "prov"}},
			},
		},
		providers:       map[string]types.Provider{"prov": provider},
		identities:      map[string]types.Identity{"role": identity},
		credentialStore: store,
		chain:           []string{"prov", "role"},
		realm:           realm.RealmInfo{Value: "test-realm"},
	}

	creds2, err := m2.authenticateChain(context.Background(), "role")
	require.NoError(t, err)
	assert.Equal(t, identityCreds, creds2)
	assert.Equal(t, int32(1), identity.callCount.Load(), "identity.Authenticate should NOT be called again (cache hit)")
}

func TestProcessCredentialCache_DifferentChainMisses(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(1 * time.Hour)
	identity1Creds := &testCreds{exp: &exp}
	identity1 := &countingIdentity{provider: "prov", creds: identity1Creds}

	identity2Creds := &testCreds{exp: &exp}
	identity2 := &countingIdentity{provider: "prov", creds: identity2Creds}

	providerCreds := &testCreds{}
	provider := &testProvider{name: "prov", creds: providerCreds}

	store := &testStore{data: map[string]any{}}

	// Authenticate chain ["prov", "role1"].
	m1 := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"role1": {Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "prov"}},
			},
		},
		providers:       map[string]types.Provider{"prov": provider},
		identities:      map[string]types.Identity{"role1": identity1},
		credentialStore: store,
		chain:           []string{"prov", "role1"},
		realm:           realm.RealmInfo{Value: "test-realm"},
	}

	_, err := m1.authenticateChain(context.Background(), "role1")
	require.NoError(t, err)
	assert.Equal(t, int32(1), identity1.callCount.Load())

	// Authenticate different chain ["prov", "role2"] - should NOT use cache.
	m2 := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"role2": {Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "prov"}},
			},
		},
		providers:       map[string]types.Provider{"prov": provider},
		identities:      map[string]types.Identity{"role2": identity2},
		credentialStore: store,
		chain:           []string{"prov", "role2"},
		realm:           realm.RealmInfo{Value: "test-realm"},
	}

	_, err = m2.authenticateChain(context.Background(), "role2")
	require.NoError(t, err)
	assert.Equal(t, int32(1), identity2.callCount.Load(), "different chain should authenticate independently")
}

func TestProcessCredentialCache_ExpiredCredsReauthenticate(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	// Seed the cache with expired credentials.
	expiredTime := time.Now().UTC().Add(-1 * time.Hour)
	expiredCreds := &testCreds{exp: &expiredTime}
	processCredentialCache.Store("test-realm:prov->role", &processCachedCreds{
		credentials: expiredCreds,
	})

	freshExp := time.Now().UTC().Add(1 * time.Hour)
	freshCreds := &testCreds{exp: &freshExp}
	identity := &countingIdentity{provider: "prov", creds: freshCreds}

	providerCreds := &testCreds{}
	provider := &testProvider{name: "prov", creds: providerCreds}

	store := &testStore{data: map[string]any{}}

	m := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"role": {Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "prov"}},
			},
		},
		providers:       map[string]types.Provider{"prov": provider},
		identities:      map[string]types.Identity{"role": identity},
		credentialStore: store,
		chain:           []string{"prov", "role"},
		realm:           realm.RealmInfo{Value: "test-realm"},
	}

	creds, err := m.authenticateChain(context.Background(), "role")
	require.NoError(t, err)
	assert.Equal(t, freshCreds, creds)
	assert.Equal(t, int32(1), identity.callCount.Load(), "should re-authenticate when cache is expired")
}

func TestProcessCredentialCache_DifferentRealmMisses(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(1 * time.Hour)
	identity1Creds := &testCreds{exp: &exp}
	identity1 := &countingIdentity{provider: "prov", creds: identity1Creds}

	identity2Creds := &testCreds{exp: &exp}
	identity2 := &countingIdentity{provider: "prov", creds: identity2Creds}

	providerCreds := &testCreds{}
	provider := &testProvider{name: "prov", creds: providerCreds}

	store := &testStore{data: map[string]any{}}

	// Authenticate with realm "realm-a".
	m1 := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"role": {Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "prov"}},
			},
		},
		providers:       map[string]types.Provider{"prov": provider},
		identities:      map[string]types.Identity{"role": identity1},
		credentialStore: store,
		chain:           []string{"prov", "role"},
		realm:           realm.RealmInfo{Value: "realm-a"},
	}

	_, err := m1.authenticateChain(context.Background(), "role")
	require.NoError(t, err)
	assert.Equal(t, int32(1), identity1.callCount.Load())

	// Same chain but different realm - should NOT use cache.
	m2 := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"role": {Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "prov"}},
			},
		},
		providers:       map[string]types.Provider{"prov": provider},
		identities:      map[string]types.Identity{"role": identity2},
		credentialStore: store,
		chain:           []string{"prov", "role"},
		realm:           realm.RealmInfo{Value: "realm-b"},
	}

	_, err = m2.authenticateChain(context.Background(), "role")
	require.NoError(t, err)
	assert.Equal(t, int32(1), identity2.callCount.Load(), "different realm should authenticate independently")
}

func Test_resetProcessCredentialCache(t *testing.T) {
	// Store something in the cache.
	processCredentialCache.Store("test-key", &processCachedCreds{})

	// Verify it exists.
	_, ok := processCredentialCache.Load("test-key")
	require.True(t, ok)

	// Reset.
	resetProcessCredentialCache()

	// Verify it's gone.
	_, ok = processCredentialCache.Load("test-key")
	assert.False(t, ok)
}
