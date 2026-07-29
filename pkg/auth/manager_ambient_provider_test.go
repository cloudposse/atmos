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

// ambientTestProvider is a minimal provider whose ambient-ness and resolved credentials
// are both controlled by the test, and which counts Authenticate calls so tests can
// assert that the ambient environment was actually re-consulted.
type ambientTestProvider struct {
	name      string
	ambient   bool
	resolved  types.ICredentials
	callCount atomic.Int32
}

func (p *ambientTestProvider) IsAmbient() bool { return p.ambient }

func (p *ambientTestProvider) Kind() string                              { return "gcp/adc" }
func (p *ambientTestProvider) Name() string                              { return p.name }
func (p *ambientTestProvider) PreAuthenticate(_ types.AuthManager) error { return nil }
func (p *ambientTestProvider) Authenticate(_ context.Context) (types.ICredentials, error) {
	p.callCount.Add(1)
	return p.resolved, nil
}
func (p *ambientTestProvider) Validate() error                         { return nil }
func (p *ambientTestProvider) Environment() (map[string]string, error) { return nil, nil }
func (p *ambientTestProvider) Paths() ([]types.Path, error)            { return []types.Path{}, nil }

func (p *ambientTestProvider) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
}
func (p *ambientTestProvider) Logout(_ context.Context) error { return nil }
func (p *ambientTestProvider) GetFilesDisplayPath() string    { return "" }
func (p *ambientTestProvider) SetRealm(_ string)              {}

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
	provider := &ambientTestProvider{name: "gcp-adc", ambient: true, resolved: freshCreds}
	identity := &passthroughIdentity{provider: "gcp-adc"}

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
	provider := &ambientTestProvider{name: "gcp-adc", ambient: true, resolved: creds}
	identity := &passthroughIdentity{provider: "gcp-adc"}

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
	provider := &ambientTestProvider{name: "prov", ambient: false, resolved: providerCreds}

	identityExp := time.Now().UTC().Add(1 * time.Hour)
	identityCreds := &testCreds{exp: &identityExp}
	identity := &passthroughIdentity{provider: "prov", creds: identityCreds}

	store := &testStore{data: map[string]any{}}
	m := newAmbientChainManager("prov", "role", provider, identity, store)

	_, err := m.authenticateChain(context.Background(), "role")
	require.NoError(t, err)

	assert.Equal(t, providerCreds, store.data["prov"], "non-ambient provider credentials must still be cached")
	assert.Equal(t, identityCreds, store.data["role"], "non-ambient identity credentials must still be cached")
}

// TestBuildWhoamiInfo_AmbientChainNotCached covers the write path that runs *after*
// chain authentication. buildWhoamiInfo persists the final credentials so later commands
// can reuse them; for an ambient chain that would immediately re-poison the entry the
// chain authentication had just purged.
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
			provider := &ambientTestProvider{name: "gcp-adc", ambient: tc.ambient}
			m := newAmbientChainManager("gcp-adc", "deployer", provider, &passthroughIdentity{provider: "gcp-adc"}, store)

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
			providers:       map[string]types.Provider{"prov": &ambientTestProvider{name: "prov", ambient: ambient}},
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
			provider := &ambientTestProvider{name: "gcp-adc", ambient: tc.ambient}
			store := &testStore{data: map[string]any{
				"gcp-adc":  &testCreds{},
				"deployer": &testCreds{},
			}}
			m := newAmbientChainManager("gcp-adc", "deployer", provider, &passthroughIdentity{provider: "gcp-adc"}, store)

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
// counterpart of the identity logout behavior above.
func TestLogoutProvider_AmbientDeletesKeyringWithoutFlag(t *testing.T) {
	provider := &ambientTestProvider{name: "gcp-adc", ambient: true}
	store := &testStore{data: map[string]any{
		"gcp-adc":  &testCreds{},
		"deployer": &testCreds{},
	}}
	m := newAmbientChainManager("gcp-adc", "deployer", provider, &passthroughIdentity{provider: "gcp-adc"}, store)

	require.NoError(t, m.LogoutProvider(context.Background(), "gcp-adc", false))

	assert.NotContains(t, store.data, "gcp-adc", "ambient provider entry should be purged without --keychain")
	assert.NotContains(t, store.data, "deployer", "identities under an ambient provider should be purged too")
}

// TestProviderIsAmbient covers the helper's handling of providers that do not implement
// the optional interface, and of nil.
func TestProviderIsAmbient(t *testing.T) {
	assert.False(t, types.ProviderIsAmbient(nil), "nil provider is not ambient")
	assert.False(t, types.ProviderIsAmbient(&testProvider{name: "plain"}),
		"provider that does not implement AmbientProvider is not ambient")
	assert.True(t, types.ProviderIsAmbient(&ambientTestProvider{ambient: true}))
	assert.False(t, types.ProviderIsAmbient(&ambientTestProvider{ambient: false}),
		"implementing the interface but returning false must not opt into ambient handling")
}

// --- Test doubles ---

// recordingStore is a testStore that additionally records every Store call, so tests can
// assert on what was written independently of later deletes.
type recordingStore struct {
	testStore
	stored map[string]types.ICredentials
}

func (s *recordingStore) Store(alias string, creds types.ICredentials, realm string) error {
	s.stored[alias] = creds
	return s.testStore.Store(alias, creds, realm)
}

// passthroughIdentity returns fixed credentials, or echoes the base credentials when
// none are configured. It stands in for a gcp/service-account impersonation step.
type passthroughIdentity struct {
	provider string
	creds    types.ICredentials
}

func (i *passthroughIdentity) Kind() string                     { return "gcp/service-account" }
func (i *passthroughIdentity) GetProviderName() (string, error) { return i.provider, nil }
func (i *passthroughIdentity) Authenticate(_ context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	if i.creds != nil {
		return i.creds, nil
	}
	return baseCreds, nil
}
func (i *passthroughIdentity) Validate() error                         { return nil }
func (i *passthroughIdentity) Environment() (map[string]string, error) { return nil, nil }
func (i *passthroughIdentity) Paths() ([]types.Path, error)            { return []types.Path{}, nil }

func (i *passthroughIdentity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
}

func (i *passthroughIdentity) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	return nil
}
func (i *passthroughIdentity) Logout(_ context.Context) error  { return nil }
func (i *passthroughIdentity) CredentialsExist() (bool, error) { return false, nil }
func (i *passthroughIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	return nil, nil
}
func (i *passthroughIdentity) SetRealm(_ string) {}
