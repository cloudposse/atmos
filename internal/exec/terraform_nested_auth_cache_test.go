package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeChainAuthManager is a minimal auth.AuthManager whose GetChain returns a fixed chain.
// Only GetChain is exercised by buildComponentAuthCacheKey; the other methods inherit the embedded
// nil interface and would panic if called, catching any unexpected use.
type fakeChainAuthManager struct {
	auth.AuthManager
	chain []string
}

func (f *fakeChainAuthManager) GetChain() []string { return f.chain }

// TestBuildComponentAuthCacheKey verifies the shared key strategy via pairwise comparisons:
// identical sections with the same parent produce one key, while differing sections or parent chains
// produce distinct keys.
func TestBuildComponentAuthCacheKey(t *testing.T) {
	t.Parallel()

	sectionA := authSectionWithDefault()
	sectionB := map[string]any{
		"identities": map[string]any{"other-default": map[string]any{"default": true}},
	}
	coreTools := &fakeChainAuthManager{chain: []string{"core-tools/terraform"}}
	other := &fakeChainAuthManager{chain: []string{"other/identity"}}

	tests := []struct {
		name      string
		parentA   auth.AuthManager
		sectionA  map[string]any
		parentB   auth.AuthManager
		sectionB  map[string]any
		wantEqual bool
	}{
		{
			name:    "identical sections with the same (nil) parent share a key",
			parentA: nil, sectionA: sectionA,
			parentB: nil, sectionB: authSectionWithDefault(),
			wantEqual: true,
		},
		{
			name:    "different auth sections do not collide",
			parentA: nil, sectionA: sectionA,
			parentB: nil, sectionB: sectionB,
			wantEqual: false,
		},
		{
			name:    "same section, inherited vs auto-detected parent differ",
			parentA: nil, sectionA: sectionA,
			parentB: coreTools, sectionB: sectionA,
			wantEqual: false,
		},
		{
			name:    "same section, distinct parent chains differ",
			parentA: coreTools, sectionA: sectionA,
			parentB: other, sectionB: sectionA,
			wantEqual: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			keyA, okA := buildComponentAuthCacheKey(tc.parentA, tc.sectionA)
			require.True(t, okA, "a string-keyed auth section must be cacheable")
			keyB, okB := buildComponentAuthCacheKey(tc.parentB, tc.sectionB)
			require.True(t, okB, "a string-keyed auth section must be cacheable")

			if tc.wantEqual {
				assert.Equal(t, keyA, keyB)
			} else {
				assert.NotEqual(t, keyA, keyB)
			}
		})
	}
}

// TestBuildComponentAuthCacheKey_NonSerializable verifies that a section that cannot be JSON-encoded
// is reported as non-cacheable instead of panicking.
func TestBuildComponentAuthCacheKey_NonSerializable(t *testing.T) {
	t.Parallel()

	_, ok := buildComponentAuthCacheKey(nil, map[string]any{"identities": make(chan int)})
	assert.False(t, ok, "a section that cannot be JSON-serialized must be reported non-cacheable")
}

// TestBuildComponentAuthCacheKey_AuthContextWrapperNeverCaches verifies that a parent AuthManager
// propagated via authContextWrapper (used by atmos.Component() and !terraform.state to carry an
// enclosing component's AuthContext into a nested lookup) is never cached. Its GetChain method always
// returns an empty slice (it deliberately cannot prove chain identity — see its doc comment), so
// without this guard two DIFFERENT real identities wrapped this way would collapse onto the same cache
// key for any shared-shape auth section, silently reusing one identity's AuthManager (and credentials)
// for a different identity's nested lookup. See docs/fixes for the regression this guards against.
func TestBuildComponentAuthCacheKey_AuthContextWrapperNeverCaches(t *testing.T) {
	t.Parallel()

	section := authSectionWithDefault()
	wrapperA := newAuthContextWrapper(&schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "identity-a"}})
	wrapperB := newAuthContextWrapper(&schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "identity-b"}})

	_, cacheableA := buildComponentAuthCacheKey(wrapperA, section)
	_, cacheableB := buildComponentAuthCacheKey(wrapperB, section)

	assert.False(t, cacheableA, "an authContextWrapper-rooted lookup must never be cached")
	assert.False(t, cacheableB, "an authContextWrapper-rooted lookup must never be cached")
}

// sentinelAuthManager is a distinguishable AuthManager used to assert pointer identity of cached results.
type sentinelAuthManager struct{ auth.AuthManager }

// TestResolveCachedComponentAuthManager_DedupesByIdentity verifies that components sharing an auth
// section (same identity) resolve the nested AuthManager once per process, while a distinct section or
// a distinct parent chain resolves independently.
func TestResolveCachedComponentAuthManager_DedupesByIdentity(t *testing.T) {
	// Not parallel: mutates the package-level nestedAuthManagerCache.
	ResetNestedAuthManagerCache()
	t.Cleanup(ResetNestedAuthManagerCache)

	mgr := &sentinelAuthManager{}
	spy := &componentAuthResolverSpy{returnMgr: mgr}
	resolve := spy.resolver()
	atmosCfg := &schema.AtmosConfiguration{}
	sectionA := authSectionWithDefault()

	// First component: cache miss, resolver runs once.
	got1, err := resolveCachedComponentAuthManager(atmosCfg, componentSectionWithAuth(sectionA), "comp-a", "stack-1", nil, sectionA, resolve)
	require.NoError(t, err)
	assert.Same(t, mgr, got1, "first resolution returns the resolver's manager")
	require.EqualValues(t, 1, spy.calls.Load(), "first component must invoke the resolver")

	// Second component, SAME section (fresh copy), different component/stack: cache hit.
	got2, err := resolveCachedComponentAuthManager(atmosCfg, componentSectionWithAuth(sectionA), "comp-b", "stack-2", nil, authSectionWithDefault(), resolve)
	require.NoError(t, err)
	assert.Same(t, mgr, got2, "cache hit must return the same manager instance")
	require.EqualValues(t, 1, spy.calls.Load(), "identical auth section must reuse the cached manager")

	// Distinct section: cache miss, resolver runs again.
	sectionB := map[string]any{"identities": map[string]any{"other-default": map[string]any{"default": true}}}
	_, err = resolveCachedComponentAuthManager(atmosCfg, componentSectionWithAuth(sectionB), "comp-c", "stack-3", nil, sectionB, resolve)
	require.NoError(t, err)
	require.EqualValues(t, 2, spy.calls.Load(), "a distinct auth section must miss the cache")

	// Same section A but a non-nil parent chain: distinct key → resolver runs again.
	parent := &fakeChainAuthManager{chain: []string{"core-tools/terraform"}}
	_, err = resolveCachedComponentAuthManager(atmosCfg, componentSectionWithAuth(sectionA), "comp-d", "stack-4", parent, sectionA, resolve)
	require.NoError(t, err)
	require.EqualValues(t, 3, spy.calls.Load(), "a different parent chain must miss the cache")
}

// TestResolveCachedComponentAuthManager_DoesNotCacheErrors verifies a failed resolution is not
// memoized: a subsequent call with the same key re-invokes the resolver, so a transient auth failure
// does not poison the cache for the rest of the process.
func TestResolveCachedComponentAuthManager_DoesNotCacheErrors(t *testing.T) {
	ResetNestedAuthManagerCache()
	t.Cleanup(ResetNestedAuthManagerCache)

	spy := &componentAuthResolverSpy{returnMgr: nil, returnErr: assert.AnError}
	resolve := spy.resolver()
	section := authSectionWithDefault()

	_, err := resolveCachedComponentAuthManager(&schema.AtmosConfiguration{}, componentSectionWithAuth(section), "comp-a", "stack-1", nil, section, resolve)
	require.ErrorIs(t, err, assert.AnError)
	require.EqualValues(t, 1, spy.calls.Load())

	// Same key again: must re-run because the error path was not cached.
	_, err = resolveCachedComponentAuthManager(&schema.AtmosConfiguration{}, componentSectionWithAuth(section), "comp-a", "stack-1", nil, authSectionWithDefault(), resolve)
	require.ErrorIs(t, err, assert.AnError)
	require.EqualValues(t, 2, spy.calls.Load(), "an errored resolution must not be cached")
}

// TestResolveCachedComponentAuthManager_UnserializableSectionAlwaysResolves verifies that a section
// that cannot be fingerprinted is never cached: every call resolves anew rather than sharing a manager
// under a bogus key.
func TestResolveCachedComponentAuthManager_UnserializableSectionAlwaysResolves(t *testing.T) {
	ResetNestedAuthManagerCache()
	t.Cleanup(ResetNestedAuthManagerCache)

	spy := &componentAuthResolverSpy{returnMgr: &sentinelAuthManager{}}
	resolve := spy.resolver()
	bad := map[string]any{"identities": make(chan int)}

	for range 2 {
		_, err := resolveCachedComponentAuthManager(&schema.AtmosConfiguration{}, nil, "comp", "stack", nil, bad, resolve)
		require.NoError(t, err)
	}
	require.EqualValues(t, 2, spy.calls.Load(), "an unserializable section must resolve every time (never cached)")
}

// TestResetStateCacheClearsNestedAuthCache verifies the documented coupling: clearing the state cache
// also drops memoized nested AuthManagers, so a JIT re-read does not reuse a stale auth manager.
func TestResetStateCacheClearsNestedAuthCache(t *testing.T) {
	ResetNestedAuthManagerCache()
	t.Cleanup(ResetNestedAuthManagerCache)

	spy := &componentAuthResolverSpy{returnMgr: &sentinelAuthManager{}}
	resolve := spy.resolver()
	section := authSectionWithDefault()

	_, err := resolveCachedComponentAuthManager(&schema.AtmosConfiguration{}, componentSectionWithAuth(section), "comp", "stack", nil, section, resolve)
	require.NoError(t, err)
	require.EqualValues(t, 1, spy.calls.Load())

	ResetStateCache() // Must also clear nestedAuthManagerCache.

	_, err = resolveCachedComponentAuthManager(&schema.AtmosConfiguration{}, componentSectionWithAuth(section), "comp", "stack", nil, authSectionWithDefault(), resolve)
	require.NoError(t, err)
	require.EqualValues(t, 2, spy.calls.Load(), "ResetStateCache must clear the nested auth cache, forcing re-resolution")
}
