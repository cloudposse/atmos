package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestManager_isCredentialValid_NilCreds is the direct unit reproducer for
// the panic in manager_chain.go:isCredentialValid when the process-level
// credential cache holds a nil ICredentials entry (the case for generic
// `ambient` identities). Before the fix, this test panicked at
// `cachedCreds.GetExpiration()` on a nil interface value.
//
// The fix short-circuits on nil and returns (false, nil), causing callers
// to treat the cache entry as invalid and fall through to re-authentication.
func TestManager_isCredentialValid_NilCreds(t *testing.T) {
	m := &manager{}

	require.NotPanics(t, func() {
		valid, expTime := m.isCredentialValid("process-cache", nil)
		assert.False(t, valid, "nil credentials must not be considered valid for cache reuse")
		assert.Nil(t, expTime, "no expiration time should be returned for nil credentials")
	})
}

// TestManager_Authenticate_AmbientStandalone_RepeatedCallsNoPanic is the
// end-to-end regression test for the panic seen when `atmos describe
// affected --upload` processed multiple components sharing the same
// standalone `ambient` identity in v1.219.0.
//
// On the first call, ambient.AuthenticateStandaloneAmbient returns
// (nil, nil), and authenticateChain stored that nil in
// processCredentialCache. The second call hit the cache and invoked
// isCredentialValid("process-cache", nil), which dereferenced the nil
// ICredentials interface → SIGSEGV.
//
// After the fix:
//   - authenticateChain no longer stores nil credentials in the cache.
//   - isCredentialValid nil-checks its input as defense-in-depth.
//
// Both guards together ensure repeated authentication of a standalone
// ambient identity is safe and never panics.
func TestManager_Authenticate_AmbientStandalone_RepeatedCallsNoPanic(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	cfg := &schema.AuthConfig{
		Realm: "test-realm",
		Identities: map[string]schema.Identity{
			"passthrough": {Kind: "ambient"},
		},
	}

	authManager, err := NewAuthManager(cfg, &testStore{data: map[string]any{}}, dummyValidator{}, nil, "")
	require.NoError(t, err)

	require.NotPanics(t, func() {
		info1, err := authManager.Authenticate(context.Background(), "passthrough")
		require.NoError(t, err)
		require.NotNil(t, info1)
		assert.Nil(t, info1.Credentials, "generic ambient identity does not manage credentials")
	})

	// Second call shares the same process-level cache key. Before the fix,
	// this call panicked when isCredentialValid dereferenced the cached
	// nil ICredentials.
	require.NotPanics(t, func() {
		info2, err := authManager.Authenticate(context.Background(), "passthrough")
		require.NoError(t, err)
		require.NotNil(t, info2)
		assert.Nil(t, info2.Credentials)
	})
}

// TestAuthenticateChain_AmbientStandalone_DoesNotCacheNil locks in the
// authenticateChain-side fix: a successful (nil, nil) result from
// AuthenticateStandaloneAmbient must NOT be written to
// processCredentialCache. Storing nil there is what set up the
// subsequent isCredentialValid panic.
//
// We assert by direct cache inspection: after a successful standalone
// ambient authentication, the cache entry for this chain must be absent.
func TestAuthenticateChain_AmbientStandalone_DoesNotCacheNil(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	cfg := &schema.AuthConfig{
		Realm: "test-realm",
		Identities: map[string]schema.Identity{
			"passthrough": {Kind: "ambient"},
		},
	}

	authManager, err := NewAuthManager(cfg, &testStore{data: map[string]any{}}, dummyValidator{}, nil, "")
	require.NoError(t, err)

	info, err := authManager.Authenticate(context.Background(), "passthrough")
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Nil(t, info.Credentials)

	// The chain for a standalone ambient is just the identity name.
	m, ok := authManager.(*manager)
	require.True(t, ok)
	cacheKey := m.chainCacheKey()

	_, present := processCredentialCache.Load(cacheKey)
	assert.False(t, present, "nil credentials from a standalone ambient identity must NOT be cached; "+
		"caching nil would re-trigger the isCredentialValid nil-deref panic on the next lookup")
}
