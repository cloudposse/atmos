package auth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// standaloneRootIdentity is a standalone identity (no provider step) used as the root of a chain.
type standaloneRootIdentity struct {
	stubIdentity
	standaloneCalls atomic.Int32
	authenticateErr error
	creds           types.ICredentials
	loadErr         error
}

func (s *standaloneRootIdentity) Kind() string { return "aws/credential-process" }

func (s *standaloneRootIdentity) IsStandalone() bool { return true }

func (s *standaloneRootIdentity) AuthenticateStandalone(_ context.Context) (types.ICredentials, error) {
	s.standaloneCalls.Add(1)
	if s.authenticateErr != nil {
		return nil, s.authenticateErr
	}
	return s.creds, nil
}

func (s *standaloneRootIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	return nil, s.loadErr
}

// recordingChildIdentity records the credentials it receives from the previous chain step.
type recordingChildIdentity struct {
	stubIdentity
	received types.ICredentials
	calls    atomic.Int32
	creds    types.ICredentials
}

func (r *recordingChildIdentity) Kind() string { return "aws/assume-role" }

func (r *recordingChildIdentity) Authenticate(_ context.Context, in types.ICredentials) (types.ICredentials, error) {
	r.calls.Add(1)
	r.received = in
	return r.creds, nil
}

// plainRootIdentity is a non-standalone identity that is not a registered provider.
type plainRootIdentity struct {
	stubIdentity
}

func newStandaloneRootChainManager(root, child types.Identity, providers map[string]types.Provider) *manager {
	return &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"root":  {Kind: "aws/credential-process"},
				"child": {Kind: "aws/assume-role", Via: &schema.IdentityVia{Identity: "root"}},
			},
		},
		providers:       providers,
		identities:      map[string]types.Identity{"root": root, "child": child},
		credentialStore: &testStore{data: map[string]any{}},
		chain:           []string{"root", "child"},
		realm:           realm.RealmInfo{Value: "test-realm"},
	}
}

// TestAuthenticateChain_StandaloneRoot_AuthenticatesViaStandalonePath is the repro for the
// "provider not registered" bug: a chain rooted at a standalone identity with no valid cached
// credentials used to be authenticated as if the root were a provider.
func TestAuthenticateChain_StandaloneRoot_AuthenticatesViaStandalonePath(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(time.Hour)
	rootCreds := &testCreds{exp: &exp}
	childCreds := &testCreds{exp: &exp}

	root := &standaloneRootIdentity{creds: rootCreds, loadErr: errors.New("no credential files")}
	child := &recordingChildIdentity{creds: childCreds}
	m := newStandaloneRootChainManager(root, child, map[string]types.Provider{})

	got, err := m.authenticateChain(context.Background(), "child")
	require.NoError(t, err)

	assert.Equal(t, childCreds, got)
	assert.Equal(t, int32(1), root.standaloneCalls.Load(), "root AuthenticateStandalone must be called once")
	assert.Equal(t, int32(1), child.calls.Load(), "child Authenticate must be called once")
	assert.Same(t, rootCreds, child.received, "child must receive the standalone root credentials")

	// Helper credentials are never written to the keyring.
	store, ok := m.credentialStore.(*testStore)
	require.True(t, ok)
	_, rootStored := store.data["root"]
	assert.False(t, rootStored, "standalone root credentials must not be written to the keyring")
}

// TestAuthenticateChain_StandaloneRoot_ErrorPropagates verifies a failing standalone root
// fails the chain without ever invoking the child.
func TestAuthenticateChain_StandaloneRoot_ErrorPropagates(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	rootErr := errors.New("helper failed")
	root := &standaloneRootIdentity{authenticateErr: rootErr, loadErr: errors.New("no credential files")}
	child := &recordingChildIdentity{}
	m := newStandaloneRootChainManager(root, child, map[string]types.Provider{})

	_, err := m.authenticateChain(context.Background(), "child")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.ErrorIs(t, err, rootErr)
	assert.Equal(t, int32(0), child.calls.Load(), "child must not run when the root fails")
}

// TestAuthenticateChain_ProviderRoot_DoesNotUseStandalonePath is the negative test: a chain
// rooted at a registered provider still authenticates through the provider, even when the
// identity registered under the same name happens to be standalone-capable.
func TestAuthenticateChain_ProviderRoot_DoesNotUseStandalonePath(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(time.Hour)
	providerCreds := &testCreds{exp: &exp}
	provider := &testProvider{name: "root", creds: providerCreds}

	root := &standaloneRootIdentity{creds: &testCreds{exp: &exp}, loadErr: errors.New("no credential files")}
	child := &recordingChildIdentity{creds: &testCreds{exp: &exp}}
	m := newStandaloneRootChainManager(root, child, map[string]types.Provider{"root": provider})

	_, err := m.authenticateChain(context.Background(), "child")
	require.NoError(t, err)

	assert.Equal(t, int32(0), root.standaloneCalls.Load(), "standalone path must not run for a registered provider root")
	assert.Same(t, providerCreds, child.received, "child must receive the provider credentials")
}

// TestAuthenticateChain_NonStandaloneUnregisteredRoot_StillFails verifies the original
// "provider not registered" error is preserved when the root is neither a provider nor a
// standalone identity.
func TestAuthenticateChain_NonStandaloneUnregisteredRoot_StillFails(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	root := &plainRootIdentity{}
	child := &recordingChildIdentity{}
	m := newStandaloneRootChainManager(root, child, map[string]types.Provider{})

	_, err := m.authenticateChain(context.Background(), "child")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	assert.Contains(t, err.Error(), "not registered")
	assert.Equal(t, int32(0), child.calls.Load())
}

// TestManager_AWSCredentialProcessRoot_ChainAndProvider verifies the real wiring: an
// aws/credential-process identity is a standalone chain root, chained identities build the
// chain [root, child], and the root reports its synthetic provider name.
func TestManager_AWSCredentialProcessRoot_ChainAndProvider(t *testing.T) {
	cfg := &schema.AuthConfig{
		Realm: "test-realm",
		Identities: map[string]schema.Identity{
			"corp-base": {
				Kind:        types.IdentityKindAWSCredentialProcess,
				Credentials: map[string]any{"credential_process": "okta-aws-cli web"},
			},
			"prod-admin": {
				Kind:      "aws/assume-role",
				Via:       &schema.IdentityVia{Identity: "corp-base"},
				Principal: map[string]any{"assume_role": "arn:aws:iam::111111111111:role/Admin"},
			},
		},
	}

	authManager, err := NewAuthManager(cfg, &testStore{data: map[string]any{}}, dummyValidator{}, nil, "")
	require.NoError(t, err)

	m, ok := authManager.(*manager)
	require.True(t, ok)

	chain, err := m.buildAuthenticationChain("prod-admin")
	require.NoError(t, err)
	assert.Equal(t, []string{"corp-base", "prod-admin"}, chain)

	chain, err = m.buildAuthenticationChain("corp-base")
	require.NoError(t, err)
	assert.Equal(t, []string{"corp-base"}, chain)

	assert.Equal(t, "aws-credential-process", m.GetProviderForIdentity("corp-base"))

	standalone, ok := m.identities["corp-base"].(types.StandaloneIdentity)
	require.True(t, ok, "the constructed identity must be dispatched through StandaloneIdentity")
	assert.True(t, standalone.IsStandalone())
}

// newStandaloneRootThreeStepManager builds a [root, mid, leaf] chain whose root is a standalone
// identity (not a registered provider).
func newStandaloneRootThreeStepManager(root *standaloneRootIdentity, mid, leaf types.Identity, store *testStore) *manager {
	return &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"root": {Kind: "aws/credential-process"},
				"mid":  {Kind: "aws/assume-role", Via: &schema.IdentityVia{Identity: "root"}},
				"leaf": {Kind: "aws/assume-role", Via: &schema.IdentityVia{Identity: "mid"}},
			},
		},
		providers:       map[string]types.Provider{},
		identities:      map[string]types.Identity{"root": root, "mid": mid, "leaf": leaf},
		credentialStore: store,
		chain:           []string{"root", "mid", "leaf"},
		realm:           realm.RealmInfo{Value: "test-realm"},
	}
}

// TestAuthenticateChain_StandaloneRoot_IgnoresCachedLongLivedRootCredentials is the repro for the
// aws/user MFA bypass: long-lived, non-expiring credentials for the standalone root sit in the
// credential store, so the cache scan used to treat the root as a valid cached starting point and
// fed the raw keys to the child, skipping the root's own session/MFA logic. The root must
// authenticate through AuthenticateStandalone and the child must receive ITS credentials.
func TestAuthenticateChain_StandaloneRoot_IgnoresCachedLongLivedRootCredentials(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(time.Hour)
	longLived := &testCreds{} // No expiration: valid forever from the cache scan's point of view.
	sessionCreds := &testCreds{exp: &exp}
	childCreds := &testCreds{exp: &exp}

	root := &standaloneRootIdentity{creds: sessionCreds, loadErr: errors.New("no credential files")}
	child := &recordingChildIdentity{creds: childCreds}
	m := newStandaloneRootChainManager(root, child, map[string]types.Provider{})
	m.credentialStore = &testStore{data: map[string]any{"root": longLived}}

	assert.Equal(t, -1, m.findFirstValidCachedCredentials(),
		"cached credentials of a standalone chain root must never be a reusable starting point")

	got, err := m.authenticateChain(context.Background(), "child")
	require.NoError(t, err)

	assert.Equal(t, childCreds, got)
	assert.Equal(t, int32(1), root.standaloneCalls.Load(), "root AuthenticateStandalone must run")
	assert.Same(t, sessionCreds, child.received, "child must receive the credentials returned by the root, not the stored long-lived ones")
	assert.NotSame(t, longLived, child.received)
}

// TestAuthenticateChain_ProviderRoot_ReusesCachedProviderCredentials is the negative test: a chain
// rooted at a registered provider keeps reusing valid cached provider credentials without
// re-authenticating the provider.
func TestAuthenticateChain_ProviderRoot_ReusesCachedProviderCredentials(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(time.Hour)
	cachedProviderCreds := &testCreds{exp: &exp}
	freshProviderCreds := &testCreds{exp: &exp}
	provider := &testProvider{name: "root", creds: freshProviderCreds}

	// The identity registered under the provider's name is standalone-capable to prove the
	// provider check wins.
	root := &standaloneRootIdentity{creds: &testCreds{exp: &exp}, loadErr: errors.New("no credential files")}
	child := &recordingChildIdentity{creds: &testCreds{exp: &exp}}
	m := newStandaloneRootChainManager(root, child, map[string]types.Provider{"root": provider})
	m.credentialStore = &testStore{data: map[string]any{"root": cachedProviderCreds}}

	assert.Equal(t, 0, m.findFirstValidCachedCredentials(), "valid cached provider credentials are a reusable starting point")

	_, err := m.authenticateChain(context.Background(), "child")
	require.NoError(t, err)

	assert.Same(t, cachedProviderCreds, child.received, "child must receive the cached provider credentials")
	assert.Equal(t, int32(0), root.standaloneCalls.Load())
}

// TestAuthenticateChain_StandaloneRoot_ReusesCachedCredentialsAtLaterStep verifies the fix is
// scoped to the root: valid cached credentials of a later step (index >= 1) are still reused and
// the standalone root is not re-authenticated.
func TestAuthenticateChain_StandaloneRoot_ReusesCachedCredentialsAtLaterStep(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(time.Hour)
	cachedMid := &testCreds{exp: &exp}
	leafCreds := &testCreds{exp: &exp}

	root := &standaloneRootIdentity{creds: &testCreds{exp: &exp}, loadErr: errors.New("no credential files")}
	mid := &recordingChildIdentity{creds: &testCreds{exp: &exp}}
	leaf := &recordingChildIdentity{creds: leafCreds}
	store := &testStore{data: map[string]any{
		"root": &testCreds{}, // Long-lived root credentials must be ignored.
		"mid":  cachedMid,
	}}
	m := newStandaloneRootThreeStepManager(root, mid, leaf, store)

	assert.Equal(t, 1, m.findFirstValidCachedCredentials(), "cached credentials at index 1 are reusable")

	got, err := m.authenticateChain(context.Background(), "leaf")
	require.NoError(t, err)

	assert.Equal(t, leafCreds, got)
	assert.Equal(t, int32(0), root.standaloneCalls.Load(), "root must not be re-authenticated when a later step is cached")
	assert.Equal(t, int32(0), mid.calls.Load(), "mid must not run when its cached credentials are reused")
	assert.Equal(t, int32(1), leaf.calls.Load())
	assert.Same(t, cachedMid, leaf.received, "leaf must receive the cached mid credentials")
}

// TestAuthenticateChain_StandaloneRoot_ExpiredLaterStepReauthenticatesFromRoot verifies the
// negative path: when the later step's cache is expired, the chain re-authenticates from the
// standalone root instead of reusing the stored root keys.
func TestAuthenticateChain_StandaloneRoot_ExpiredLaterStepReauthenticatesFromRoot(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(time.Hour)
	expired := time.Now().UTC().Add(-time.Hour)
	sessionCreds := &testCreds{exp: &exp}

	root := &standaloneRootIdentity{creds: sessionCreds, loadErr: errors.New("no credential files")}
	mid := &recordingChildIdentity{creds: &testCreds{exp: &exp}}
	leaf := &recordingChildIdentity{creds: &testCreds{exp: &exp}}
	store := &testStore{data: map[string]any{
		"root": &testCreds{},
		"mid":  &testCreds{exp: &expired},
	}}
	m := newStandaloneRootThreeStepManager(root, mid, leaf, store)

	assert.Equal(t, -1, m.findFirstValidCachedCredentials())

	_, err := m.authenticateChain(context.Background(), "leaf")
	require.NoError(t, err)

	assert.Equal(t, int32(1), root.standaloneCalls.Load())
	assert.Same(t, sessionCreds, mid.received, "mid must receive the credentials returned by the root")
}

// TestAuthenticateChain_StandaloneSingleElementChain_Unchanged verifies a single-element chain of a
// standalone identity still authenticates through AuthenticateStandalone, regardless of what is
// cached for it.
func TestAuthenticateChain_StandaloneSingleElementChain_Unchanged(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	exp := time.Now().UTC().Add(time.Hour)
	rootCreds := &testCreds{exp: &exp}
	root := &standaloneRootIdentity{creds: rootCreds, loadErr: errors.New("no credential files")}
	m := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{"root": {Kind: "aws/credential-process"}},
		},
		providers:       map[string]types.Provider{},
		identities:      map[string]types.Identity{"root": root},
		credentialStore: &testStore{data: map[string]any{"root": &testCreds{}}},
		chain:           []string{"root"},
		realm:           realm.RealmInfo{Value: "test-realm"},
	}

	assert.Equal(t, -1, m.findFirstValidCachedCredentials())

	got, err := m.authenticateChain(context.Background(), "root")
	require.NoError(t, err)

	assert.Same(t, rootCreds, got)
	assert.Equal(t, int32(1), root.standaloneCalls.Load())
}
