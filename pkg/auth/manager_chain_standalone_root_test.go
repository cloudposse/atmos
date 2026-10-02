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
