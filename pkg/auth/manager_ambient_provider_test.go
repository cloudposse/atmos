package auth

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// --- Ambient provider caching tests (issue #2695) ---
//
// Providers such as gcp/adc resolve the active principal from ambient environment state
// (gcloud's application-default credentials, GOOGLE_APPLICATION_CREDENTIALS, or the
// metadata server) on every call and mint a short-lived token from it. Persisting that
// token to the keyring made `atmos auth login` replay the previous principal even after
// the ambient credentials were changed with `gcloud auth application-default login`.
//
// These tests pin the contract: chains rooted at an ambient provider are never served
// from — nor written to — the persistent keyring, and pre-existing (poisoned) entries are
// purged so upgraded installations self-heal.

// Compile-time sentinel: the tests below configure providers by these schema fields, so
// a rename fails the build here rather than silently changing what is exercised.
var (
	_ = schema.Provider{Kind: "gcp/adc"}
	_ = schema.Identity{Kind: "gcp/service-account"}
)

// ambientTestProvider is the shared testProvider double extended with the two things
// these tests need: a controllable ambient answer, and an Authenticate that counts calls
// and returns an arbitrary ICredentials so a test can watch the "ambient environment"
// resolve to a different principal. Everything else — Kind, Name, Paths, SetRealm and the
// rest of the Provider surface — is inherited rather than restated.
type ambientTestProvider struct {
	testProvider
	ambient   bool
	resolved  types.ICredentials
	callCount atomic.Int32
}

// IsAmbient satisfies types.AmbientProvider with the test-controlled answer, so both
// directions of the opt-in can be exercised against the same double.
func (p *ambientTestProvider) IsAmbient() bool { return p.ambient }

// Authenticate records the call and returns the currently configured credentials,
// standing in for a live re-resolution of ambient environment state.
func (p *ambientTestProvider) Authenticate(_ context.Context) (types.ICredentials, error) {
	p.callCount.Add(1)
	return p.resolved, nil
}

// Compile-time assertions that the doubles satisfy the interfaces under test.
var (
	_ types.Provider        = (*ambientTestProvider)(nil)
	_ types.AmbientProvider = (*ambientTestProvider)(nil)
	_ types.Identity        = (*passthroughIdentity)(nil)
)

// newAmbientChainManager builds a manager for the chain [providerName, identityName].
func newAmbientChainManager(providerName, identityName string, provider types.Provider, identity types.Identity, store types.CredentialStore) *manager {
	return &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{
				identityName: {Kind: "gcp/service-account", Via: &schema.IdentityVia{Provider: providerName}},
			},
			Providers: map[string]schema.Provider{
				providerName: {Kind: "gcp/adc"},
			},
		},
		providers:       map[string]types.Provider{providerName: provider},
		identities:      map[string]types.Identity{identityName: identity},
		credentialStore: store,
		chain:           []string{providerName, identityName},
		realm:           realm.RealmInfo{Value: "test-realm"},
	}
}

// TestAmbientProvider_StaleKeyringEntryIsNotReplayed reproduces issue #2695: after the
// ambient ADC principal changes, a keyring entry cached from the previous principal must
// not be reused. Authentication must re-resolve through the provider.
func TestAmbientProvider_StaleKeyringEntryIsNotReplayed(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	// Keyring poisoned by an older Atmos version: account A's token, still unexpired.
	staleCreds := &types.GCPCredentials{
		AccessToken:         "token-account-a",
		TokenExpiry:         time.Now().UTC().Add(1 * time.Hour),
		ServiceAccountEmail: "account-a@example.com",
	}
	store := &testStore{data: map[string]any{
		"gcp-adc":  staleCreds,
		"deployer": staleCreds,
	}}

	// The ambient environment now resolves to account B.
	freshCreds := &types.GCPCredentials{
		AccessToken:         "token-account-b",
		TokenExpiry:         time.Now().UTC().Add(1 * time.Hour),
		ServiceAccountEmail: "account-b@example.com",
	}
	provider := &ambientTestProvider{testProvider: testProvider{name: "gcp-adc", kind: "gcp/adc"}, ambient: true, resolved: freshCreds}
	identity := &passthroughIdentity{countingIdentity: countingIdentity{provider: "gcp-adc"}}

	m := newAmbientChainManager("gcp-adc", "deployer", provider, identity, store)

	got, err := m.authenticateChain(context.Background(), "deployer")
	require.NoError(t, err)

	// The provider must have been consulted rather than the keyring replayed.
	assert.Equal(t, int32(1), provider.callCount.Load(), "ambient provider must re-resolve on every login")

	gcpCreds, ok := got.(*types.GCPCredentials)
	require.True(t, ok, "expected GCP credentials")
	assert.Equal(t, "token-account-b", gcpCreds.AccessToken, "must return the freshly resolved principal, not the cached one")
	assert.Equal(t, "account-b@example.com", gcpCreds.ServiceAccountEmail)

	// The poisoned entries must be purged rather than left to mislead a later run.
	assert.NotContains(t, store.data, "gcp-adc", "stale ambient provider entry must be purged from the keyring")
	assert.NotContains(t, store.data, "deployer", "credentials derived from an ambient provider must not be persisted")
}

// TestAmbientProvider_CredentialsNotWrittenToKeyring verifies the write side: a clean
// keyring stays clean after authenticating an ambient chain.
func TestAmbientProvider_CredentialsNotWrittenToKeyring(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	creds := &types.GCPCredentials{
		AccessToken: "token",
		TokenExpiry: time.Now().UTC().Add(1 * time.Hour),
	}
	provider := &ambientTestProvider{testProvider: testProvider{name: "gcp-adc", kind: "gcp/adc"}, ambient: true, resolved: creds}
	identity := &passthroughIdentity{countingIdentity: countingIdentity{provider: "gcp-adc"}}

	store := &testStore{data: map[string]any{}}
	m := newAmbientChainManager("gcp-adc", "deployer", provider, identity, store)

	_, err := m.authenticateChain(context.Background(), "deployer")
	require.NoError(t, err)

	assert.Empty(t, store.data, "ambient chains must not persist any credentials to the keyring")
}

// TestNonAmbientProvider_CredentialsStillCached is the negative path for the change
// above: providers that do not declare themselves ambient must keep caching as before.
func TestNonAmbientProvider_CredentialsStillCached(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	providerCreds := &testCreds{}
	provider := &ambientTestProvider{testProvider: testProvider{name: "prov", kind: "gcp/adc"}, ambient: false, resolved: providerCreds}

	identityExp := time.Now().UTC().Add(1 * time.Hour)
	identityCreds := &testCreds{exp: &identityExp}
	identity := &passthroughIdentity{countingIdentity: countingIdentity{provider: "prov"}, creds: identityCreds}

	store := &testStore{data: map[string]any{}}
	m := newAmbientChainManager("prov", "role", provider, identity, store)

	_, err := m.authenticateChain(context.Background(), "role")
	require.NoError(t, err)

	assert.Equal(t, providerCreds, store.data["prov"], "non-ambient provider credentials must still be cached")
	assert.Equal(t, identityCreds, store.data["role"], "non-ambient identity credentials must still be cached")
}

// TestBuildWhoamiInfo_AmbientChainNotCached covers the write path that runs *after*
// chain authentication. The buildWhoamiInfo helper persists the final credentials so
// later commands can reuse them; for an ambient chain that would immediately re-poison
// the entry the chain authentication had just purged.
func TestBuildWhoamiInfo_AmbientChainNotCached(t *testing.T) {
	for _, tc := range []struct {
		name         string
		ambient      bool
		expectStored bool
	}{
		{name: "ambient chain is not persisted", ambient: true, expectStored: false},
		{name: "non-ambient chain is persisted", ambient: false, expectStored: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			creds := &types.GCPCredentials{
				AccessToken: "token",
				TokenExpiry: time.Now().UTC().Add(1 * time.Hour),
			}
			// Record Store calls rather than inspecting the final map: on the success
			// path buildWhoamiInfo also prunes the legacy (pre-realm) entry, and the
			// realm-blind testStore cannot tell that delete apart from the write.
			store := &recordingStore{testStore: testStore{data: map[string]any{}}, stored: map[string]types.ICredentials{}}
			provider := &ambientTestProvider{testProvider: testProvider{name: "gcp-adc", kind: "gcp/adc"}, ambient: tc.ambient}
			m := newAmbientChainManager("gcp-adc", "deployer", provider, &passthroughIdentity{countingIdentity: countingIdentity{provider: "gcp-adc"}}, store)

			info := m.buildWhoamiInfo("deployer", creds)

			// The handle is set either way so callers can reach the in-memory credentials.
			assert.Equal(t, "deployer", info.CredentialsRef)

			if tc.expectStored {
				assert.Equal(t, creds, store.stored["deployer"], "non-ambient credentials must still be persisted")
			} else {
				assert.NotContains(t, store.stored, "deployer", "ambient credentials must not be persisted by whoami")
			}
		})
	}
}

// TestFindFirstValidCachedCredentials_AmbientChain verifies the read side directly:
// a valid cached entry mid-chain is ignored when the chain root is ambient, and honored
// when it is not.
func TestFindFirstValidCachedCredentials_AmbientChain(t *testing.T) {
	exp := time.Now().UTC().Add(1 * time.Hour)
	cached := &testCreds{exp: &exp}

	newManager := func(ambient bool) *manager {
		return &manager{
			config:          &schema.AuthConfig{Identities: map[string]schema.Identity{}},
			providers:       map[string]types.Provider{"prov": &ambientTestProvider{testProvider: testProvider{name: "prov", kind: "gcp/adc"}, ambient: ambient}},
			identities:      map[string]types.Identity{},
			credentialStore: &testStore{data: map[string]any{"mid": cached}},
			// Three steps so the "mid" entry is not the target, which is skipped for
			// unrelated reasons (see findFirstValidCachedCredentials).
			chain: []string{"prov", "mid", "target"},
			realm: realm.RealmInfo{Value: "test-realm"},
		}
	}

	assert.Equal(t, -1, newManager(true).findFirstValidCachedCredentials(),
		"ambient chains must ignore cached credentials and re-authenticate from the provider")
	assert.Equal(t, 1, newManager(false).findFirstValidCachedCredentials(),
		"non-ambient chains must still resume from valid cached credentials")
}

// TestLogout_AmbientChainDeletesKeyringWithoutFlag verifies that logging out of an
// ambient chain clears the keyring entry without requiring --keychain. The entry holds
// nothing durable, so preserving it only keeps stale data around.
func TestLogout_AmbientChainDeletesKeyringWithoutFlag(t *testing.T) {
	for _, tc := range []struct {
		name          string
		ambient       bool
		expectDeleted bool
	}{
		{name: "ambient chain purges without --keychain", ambient: true, expectDeleted: true},
		{name: "non-ambient chain preserves without --keychain", ambient: false, expectDeleted: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &ambientTestProvider{testProvider: testProvider{name: "gcp-adc", kind: "gcp/adc"}, ambient: tc.ambient}
			store := &testStore{data: map[string]any{
				"gcp-adc":  &testCreds{},
				"deployer": &testCreds{},
			}}
			m := newAmbientChainManager("gcp-adc", "deployer", provider, &passthroughIdentity{countingIdentity: countingIdentity{provider: "gcp-adc"}}, store)

			// deleteKeychain=false: the flag the user did NOT pass.
			require.NoError(t, m.Logout(context.Background(), "deployer", false))

			if tc.expectDeleted {
				assert.NotContains(t, store.data, "deployer", "ambient identity entry should be purged without --keychain")
			} else {
				assert.Contains(t, store.data, "deployer", "non-ambient identity entry must be preserved without --keychain")
			}
		})
	}
}

// TestLogoutProvider_AmbientDeletesKeyringWithoutFlag verifies the provider-scoped
// counterpart of the identity logout behavior above, including the negative path: a
// non-ambient provider must still honor the default "preserve credentials" contract.
func TestLogoutProvider_AmbientDeletesKeyringWithoutFlag(t *testing.T) {
	for _, tc := range []struct {
		name          string
		ambient       bool
		expectDeleted bool
	}{
		{name: "ambient provider purges without --keychain", ambient: true, expectDeleted: true},
		{name: "non-ambient provider preserves without --keychain", ambient: false, expectDeleted: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &ambientTestProvider{testProvider: testProvider{name: "gcp-adc", kind: "gcp/adc"}, ambient: tc.ambient}
			store := &testStore{data: map[string]any{
				"gcp-adc":  &testCreds{},
				"deployer": &testCreds{},
			}}
			m := newAmbientChainManager("gcp-adc", "deployer", provider, &passthroughIdentity{countingIdentity: countingIdentity{provider: "gcp-adc"}}, store)

			require.NoError(t, m.LogoutProvider(context.Background(), "gcp-adc", false))

			if tc.expectDeleted {
				assert.NotContains(t, store.data, "gcp-adc", "ambient provider entry should be purged without --keychain")
				assert.NotContains(t, store.data, "deployer", "identities under an ambient provider should be purged too")
			} else {
				assert.Contains(t, store.data, "gcp-adc", "non-ambient provider entry must be preserved without --keychain")
				assert.Contains(t, store.data, "deployer", "identities under a non-ambient provider must be preserved too")
			}
		})
	}
}

// TestLogoutAll_AmbientProviderPurgedWithoutFlag covers the third logout entry point.
// LogoutAll decides per provider, so an ambient provider must be cleared even when
// non-ambient providers in the same config are preserved.
func TestLogoutAll_AmbientProviderPurgedWithoutFlag(t *testing.T) {
	store := newRealmStore()
	store.put("gcp-adc", "test-realm", &testCreds{})
	store.put("aws-sso", "test-realm", &testCreds{})

	m := &manager{
		config: &schema.AuthConfig{Identities: map[string]schema.Identity{}},
		providers: map[string]types.Provider{
			"gcp-adc": &ambientTestProvider{testProvider: testProvider{name: "gcp-adc", kind: "gcp/adc"}, ambient: true},
			"aws-sso": &ambientTestProvider{testProvider: testProvider{name: "aws-sso", kind: "gcp/adc"}, ambient: false},
		},
		identities:      map[string]types.Identity{},
		credentialStore: store,
		realm:           realm.RealmInfo{Value: "test-realm"},
	}

	require.NoError(t, m.LogoutAll(context.Background(), false))

	assert.False(t, store.has("gcp-adc", "test-realm"), "ambient provider entry should be purged without --keychain")
	assert.True(t, store.has("aws-sso", "test-realm"), "non-ambient provider entry must be preserved without --keychain")
}

// TestPurgeCachedCredentials_RemovesRealmAndLegacyEntries pins the self-healing behavior
// for installations upgrading from the pre-realm keyring layout: both the realm-scoped
// entry and the legacy unscoped one must go, or the stale principal survives the upgrade.
// The realm-blind testStore cannot distinguish these, hence the realm-aware double.
func TestPurgeCachedCredentials_RemovesRealmAndLegacyEntries(t *testing.T) {
	store := newRealmStore()
	store.put("gcp-adc", "test-realm", &testCreds{})
	store.put("gcp-adc", "", &testCreds{}) // Legacy pre-realm entry.
	store.put("other", "test-realm", &testCreds{})

	m := &manager{credentialStore: store, realm: realm.RealmInfo{Value: "test-realm"}}
	m.purgeCachedCredentials("gcp-adc")

	assert.False(t, store.has("gcp-adc", "test-realm"), "realm-scoped entry must be purged")
	assert.False(t, store.has("gcp-adc", ""), "legacy pre-realm entry must be purged")
	assert.True(t, store.has("other", "test-realm"), "unrelated entries must be left alone")
}

// TestPurgeCachedCredentials_EmptyRealmSkipsLegacyDelete verifies the guard around the
// legacy delete: with no realm configured the realm-scoped key IS the legacy key, so a
// second delete would be redundant.
func TestPurgeCachedCredentials_EmptyRealmSkipsLegacyDelete(t *testing.T) {
	store := newRealmStore()
	store.put("gcp-adc", "", &testCreds{})

	m := &manager{credentialStore: store, realm: realm.RealmInfo{Value: ""}}
	m.purgeCachedCredentials("gcp-adc")

	assert.False(t, store.has("gcp-adc", ""), "entry must be purged")
	assert.Equal(t, 1, store.deleteCalls, "exactly one delete when no realm is configured")
}

// TestPurgeCachedCredentials_NilStore verifies the nil-store guard. Managers are
// constructed without a credential store in several unit-test and embedded code paths,
// and purging must not panic there.
func TestPurgeCachedCredentials_NilStore(t *testing.T) {
	m := &manager{realm: realm.RealmInfo{Value: "test-realm"}}
	assert.NotPanics(t, func() { m.purgeCachedCredentials("gcp-adc") })
}

// TestChainRootIsAmbient_Guards covers the two ways the chain root can fail to resolve to
// a registered provider. Both must report "not ambient" so normal caching is unaffected.
func TestChainRootIsAmbient_Guards(t *testing.T) {
	tests := []struct {
		name      string
		chain     []string
		providers map[string]types.Provider
	}{
		{
			name:      "empty chain",
			chain:     nil,
			providers: map[string]types.Provider{"prov": &ambientTestProvider{ambient: true}},
		},
		{
			name:  "chain root is not a registered provider",
			chain: []string{"standalone-identity"},
			// A standalone identity (aws/user, ambient) forms a single-element chain whose
			// head is an identity name, so the provider lookup misses by design.
			providers: map[string]types.Provider{"prov": &ambientTestProvider{ambient: true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manager{chain: tt.chain, providers: tt.providers}
			assert.False(t, m.chainRootIsAmbient())
		})
	}
}

// TestIdentityChainRootIsAmbient_MultiHopChain verifies the ambient check follows a
// multi-hop `via` chain. An impersonation chain two identities deep is still rooted at
// ADC, and its intermediate credentials are just as non-durable as the provider's token.
func TestIdentityChainRootIsAmbient_MultiHopChain(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{Identities: map[string]schema.Identity{
			"base-sa":   {Kind: "gcp/service-account", Via: &schema.IdentityVia{Provider: "gcp-adc"}},
			"target-sa": {Kind: "gcp/service-account", Via: &schema.IdentityVia{Identity: "base-sa"}},
			"aws-role":  {Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "aws-sso"}},
		}},
		providers: map[string]types.Provider{
			"gcp-adc": &ambientTestProvider{testProvider: testProvider{name: "gcp-adc", kind: "gcp/adc"}, ambient: true},
			"aws-sso": &ambientTestProvider{testProvider: testProvider{name: "aws-sso", kind: "gcp/adc"}, ambient: false},
		},
	}

	assert.True(t, m.identityChainRootIsAmbient("target-sa"), "two hops from an ambient provider is still ambient")
	assert.True(t, m.identityChainRootIsAmbient("base-sa"))
	assert.False(t, m.identityChainRootIsAmbient("aws-role"), "a chain rooted at a non-ambient provider must cache normally")
	assert.False(t, m.identityChainRootIsAmbient("unknown"), "an unresolvable identity is not ambient")
}

// TestIsAmbientProvider_SatisfiesReporter covers the optional-interface wrapper the
// logout dry-run preview depends on. Without it the preview cannot know that ambient
// keyring entries are removed even when --keychain was not passed, and under-reports.
//
// The manager is bound to types.AmbientProviderReporter (not types.AuthManager) because
// the reporter is deliberately kept off the wide manager interface.
func TestIsAmbientProvider_SatisfiesReporter(t *testing.T) {
	var m types.AmbientProviderReporter = &manager{
		config: &schema.AuthConfig{Identities: map[string]schema.Identity{}},
		providers: map[string]types.Provider{
			"gcp-adc": &ambientTestProvider{testProvider: testProvider{name: "gcp-adc", kind: "gcp/adc"}, ambient: true},
			"aws-sso": &ambientTestProvider{testProvider: testProvider{name: "aws-sso", kind: "gcp/adc"}, ambient: false},
		},
	}

	assert.True(t, m.IsAmbientProvider("gcp-adc"))
	assert.False(t, m.IsAmbientProvider("aws-sso"))
	assert.False(t, m.IsAmbientProvider("does-not-exist"), "an unknown provider name is not ambient")

	// The helper the cmd layer actually calls must agree when handed the real manager.
	assert.True(t, types.ManagerReportsAmbient(m, "gcp-adc"))
	assert.False(t, types.ManagerReportsAmbient(m, "aws-sso"))
}

// TestResolveProviderForIdentity_NilConfig covers the nil-config guard. The
// buildWhoamiInfo helper now calls into this one, and it runs on managers built without
// config in several narrow paths.
func TestResolveProviderForIdentity_NilConfig(t *testing.T) {
	m := &manager{}
	assert.Empty(t, m.resolveProviderForIdentity("anything"))
}

// TestAmbientChain_MidChainCredentialsPurged verifies the intermediate steps of a longer
// ambient chain, not just its root and target: an impersonated token cached at step 1
// would otherwise let the chain resume from a stale principal without re-resolving ADC.
func TestAmbientChain_MidChainCredentialsPurged(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(1 * time.Hour)
	store := newRealmStore()
	store.put("gcp-adc", "test-realm", &testCreds{exp: &exp})
	store.put("base-sa", "test-realm", &testCreds{exp: &exp})
	store.put("target-sa", "test-realm", &testCreds{exp: &exp})

	provider := &ambientTestProvider{testProvider: testProvider{name: "gcp-adc", kind: "gcp/adc"}, ambient: true, resolved: &testCreds{exp: &exp}}
	m := &manager{
		config: &schema.AuthConfig{Identities: map[string]schema.Identity{
			"base-sa":   {Kind: "gcp/service-account", Via: &schema.IdentityVia{Provider: "gcp-adc"}},
			"target-sa": {Kind: "gcp/service-account", Via: &schema.IdentityVia{Identity: "base-sa"}},
		}},
		providers: map[string]types.Provider{"gcp-adc": provider},
		identities: map[string]types.Identity{
			"base-sa":   &passthroughIdentity{countingIdentity: countingIdentity{provider: "gcp-adc"}},
			"target-sa": &passthroughIdentity{countingIdentity: countingIdentity{provider: "gcp-adc"}},
		},
		credentialStore: store,
		chain:           []string{"gcp-adc", "base-sa", "target-sa"},
		realm:           realm.RealmInfo{Value: "test-realm"},
	}

	_, err := m.authenticateChain(context.Background(), "target-sa")
	require.NoError(t, err)

	assert.Equal(t, int32(1), provider.callCount.Load(), "the chain must re-resolve from the ambient provider")
	for _, name := range []string{"gcp-adc", "base-sa", "target-sa"} {
		assert.False(t, store.has(name, "test-realm"), "stale entry for %q must be purged", name)
	}
}

// TestSharedTestProviderIsNotAmbient guards the other tests in this package: they use the
// shared testProvider double for non-ambient scenarios, so it must never accidentally
// satisfy types.AmbientProvider. Exhaustive coverage of the helper itself lives beside it
// in pkg/auth/types.
func TestSharedTestProviderIsNotAmbient(t *testing.T) {
	assert.False(t, types.ProviderIsAmbient(&testProvider{name: "plain"}),
		"testProvider must stay non-ambient or the non-ambient assertions here are vacuous")
}

// --- Test doubles ---

// realmStore is a credential store double that keys entries by (alias, realm) instead of
// alias alone. The shared testStore is realm-blind, which makes it unable to distinguish
// a realm-scoped entry from the legacy pre-realm one — exactly the distinction the
// ambient purge depends on.
type realmStore struct {
	data        map[string]types.ICredentials
	deleteCalls int
}

// newRealmStore returns an empty realm-aware store.
func newRealmStore() *realmStore {
	return &realmStore{data: map[string]types.ICredentials{}}
}

// realmKey composes the (alias, realm) pair into a single map key. The NUL separator
// cannot appear in either component, so distinct pairs can never collide.
func realmKey(alias, realm string) string { return realm + "\x00" + alias }

// put seeds an entry directly, bypassing Store, so a test can stage pre-existing
// keyring state without counting it as a write.
func (s *realmStore) put(alias, realm string, creds types.ICredentials) {
	s.data[realmKey(alias, realm)] = creds
}

// has reports whether an entry exists for the exact (alias, realm) pair.
func (s *realmStore) has(alias, realm string) bool {
	_, ok := s.data[realmKey(alias, realm)]
	return ok
}

// Store writes the credentials under the realm-scoped key.
func (s *realmStore) Store(alias string, creds types.ICredentials, realm string) error {
	s.put(alias, realm, creds)
	return nil
}

// Retrieve returns the entry for the exact (alias, realm) pair, or ErrCredentialsNotFound.
func (s *realmStore) Retrieve(alias string, realm string) (types.ICredentials, error) {
	creds, ok := s.data[realmKey(alias, realm)]
	if !ok {
		return nil, credentials.ErrCredentialsNotFound
	}
	return creds, nil
}

// Delete removes the realm-scoped entry and counts the call, so tests can assert how
// many deletes the purge path issued — the realm and legacy keys are separate deletes.
func (s *realmStore) Delete(alias string, realm string) error {
	s.deleteCalls++
	key := realmKey(alias, realm)
	if _, ok := s.data[key]; !ok {
		return credentials.ErrCredentialsNotFound
	}
	delete(s.data, key)
	return nil
}

// List is unused by these tests and returns nothing.
func (s *realmStore) List(_ string) ([]string, error) { return nil, nil }

// IsExpired reports expiry for the realm-scoped entry.
func (s *realmStore) IsExpired(alias string, realm string) (bool, error) {
	creds, ok := s.data[realmKey(alias, realm)]
	if !ok {
		return false, credentials.ErrCredentialsNotFound
	}
	return creds.IsExpired(), nil
}

// Type identifies this double in diagnostics.
func (s *realmStore) Type() string { return "test-realm-aware" }

var _ types.CredentialStore = (*realmStore)(nil)

// recordingStore is a testStore that additionally records every Store call, so tests can
// assert on what was written independently of later deletes.
type recordingStore struct {
	testStore
	stored map[string]types.ICredentials
}

// Store records the write before delegating, so a test can assert on what was persisted
// independently of any later delete.
func (s *recordingStore) Store(alias string, creds types.ICredentials, realm string) error {
	s.stored[alias] = creds
	return s.testStore.Store(alias, creds, realm)
}

// passthroughIdentity is the shared countingIdentity double reshaped for GCP chains: it
// reports the gcp/service-account kind and, when no credentials are configured, echoes
// whatever the previous chain step produced. The rest of the Identity surface is
// inherited rather than restated.
type passthroughIdentity struct {
	countingIdentity
	creds types.ICredentials
}

// Kind reports gcp/service-account so chains built from this double look like the
// ADC -> impersonation shape the ambient tests exercise.
func (i *passthroughIdentity) Kind() string { return "gcp/service-account" }

// Authenticate returns the configured credentials, or passes the upstream step's
// credentials straight through when none are set — the behavior that lets a test observe
// exactly what the provider resolved.
func (i *passthroughIdentity) Authenticate(_ context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	if i.creds != nil {
		return i.creds, nil
	}
	return baseCreds, nil
}

// TestLogout_AmbientChainWithNoKeyringEntry covers the steady state, which every other
// logout test here misses by pre-populating the store first.
//
// Because ambient chains never write to the keyring, the normal case at logout time is
// that there is nothing to delete. Forcing the delete must not turn that expected miss
// into ErrPartialLogout — otherwise `atmos auth logout <gcp-adc-identity>` reports a
// failure on a perfectly clean system. The realm-aware double is used deliberately:
// testStore.Delete always returns nil and so cannot reproduce this.
func TestLogout_AmbientChainWithNoKeyringEntry(t *testing.T) {
	provider := &ambientTestProvider{testProvider: testProvider{name: "gcp-adc", kind: "gcp/adc"}, ambient: true}
	store := newRealmStore() // Deliberately empty.
	m := newAmbientChainManager("gcp-adc", "deployer", provider, &passthroughIdentity{countingIdentity: countingIdentity{provider: "gcp-adc"}}, store)

	err := m.Logout(context.Background(), "deployer", false)

	require.NoError(t, err, "a missing keyring entry is the expected state for an ambient chain, not a partial-logout failure")
}

// TestLogoutProvider_AmbientWithNoKeyringEntry is the provider-scoped counterpart.
func TestLogoutProvider_AmbientWithNoKeyringEntry(t *testing.T) {
	provider := &ambientTestProvider{testProvider: testProvider{name: "gcp-adc", kind: "gcp/adc"}, ambient: true}
	store := newRealmStore()
	m := newAmbientChainManager("gcp-adc", "deployer", provider, &passthroughIdentity{countingIdentity: countingIdentity{provider: "gcp-adc"}}, store)

	require.NoError(t, m.LogoutProvider(context.Background(), "gcp-adc", false))
}

// TestLogoutAll_AmbientWithNoKeyringEntry is the --all counterpart.
func TestLogoutAll_AmbientWithNoKeyringEntry(t *testing.T) {
	store := newRealmStore()
	m := &manager{
		config:          &schema.AuthConfig{Identities: map[string]schema.Identity{}},
		providers:       map[string]types.Provider{"gcp-adc": &ambientTestProvider{testProvider: testProvider{name: "gcp-adc", kind: "gcp/adc"}, ambient: true}},
		identities:      map[string]types.Identity{},
		credentialStore: store,
		realm:           realm.RealmInfo{Value: "test-realm"},
	}

	require.NoError(t, m.LogoutAll(context.Background(), false))
}

// TestLogout_ExplicitKeychainStillReportsDeletionFailure is the negative path: when the
// user explicitly asks for --keychain, a failed delete must still surface, so the
// ambient tolerance above cannot silently swallow real keyring errors.
func TestLogout_ExplicitKeychainStillReportsDeletionFailure(t *testing.T) {
	provider := &ambientTestProvider{testProvider: testProvider{name: "aws-sso", kind: "aws/iam-identity-center"}, ambient: false}
	store := newRealmStore() // Empty, so the delete fails.
	m := newAmbientChainManager("aws-sso", "role", provider, &passthroughIdentity{countingIdentity: countingIdentity{provider: "aws-sso"}}, store)

	err := m.Logout(context.Background(), "role", true /*deleteKeychain*/)

	require.Error(t, err, "an explicit --keychain delete that fails must still be reported")
	assert.ErrorIs(t, err, errUtils.ErrPartialLogout)
}

// TestPurgeCachedCredentials_ToleratesMissingEntry covers the branch taken when the
// realm-scoped delete finds nothing. That is the ordinary case once the ambient fix is in
// place — nothing was ever written — so the purge must stay silent rather than surface an
// error the way an explicitly requested deletion would. The realm-aware double is required
// here: testStore.Delete always returns nil and so never reaches this branch.
func TestPurgeCachedCredentials_ToleratesMissingEntry(t *testing.T) {
	store := newRealmStore() // Empty: every delete misses.
	m := &manager{credentialStore: store, realm: realm.RealmInfo{Value: "test-realm"}}

	assert.NotPanics(t, func() { m.purgeCachedCredentials("gcp-adc") })

	// Both the realm-scoped and legacy keys are attempted even though neither exists.
	assert.Equal(t, 2, store.deleteCalls, "purge must still attempt both keys when the entry is absent")
	assert.False(t, store.has("gcp-adc", "test-realm"))
}

// TestAuthenticateWithProvider_SessionTokenStillSkipsKeyring guards the pre-existing
// session-token branch, which now sits alongside the ambient branch in the same switch.
// A non-ambient provider that mints session tokens must still bypass the keyring, so the
// restructuring cannot have silently folded that case into the caching default.
func TestAuthenticateWithProvider_SessionTokenStillSkipsKeyring(t *testing.T) {
	sessionCreds := &types.AWSCredentials{
		AccessKeyID:     "ASIA_SESSION",
		SecretAccessKey: "secret",
		SessionToken:    "session-token", // What makes isSessionToken true.
	}
	provider := &ambientTestProvider{
		testProvider: testProvider{name: "aws-sso", kind: "aws/iam-identity-center"},
		ambient:      false, // Non-ambient: the session-token branch must be the one taken.
		resolved:     sessionCreds,
	}
	store := &testStore{data: map[string]any{}}
	m := &manager{
		config:          &schema.AuthConfig{Identities: map[string]schema.Identity{}},
		providers:       map[string]types.Provider{"aws-sso": provider},
		identities:      map[string]types.Identity{},
		credentialStore: store,
		chain:           []string{"aws-sso"},
		realm:           realm.RealmInfo{Value: "test-realm"},
	}

	got, err := m.authenticateWithProvider(context.Background(), "aws-sso")
	require.NoError(t, err)
	assert.Equal(t, sessionCreds, got)

	assert.Empty(t, store.data, "session tokens must not be written to the keyring, ambient or not")
}
