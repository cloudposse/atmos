package auth

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the schema field the fixtures below rely on.
var _ = schema.Identity{Kind: "aws/credential-process"}

// nonPersistentIdentity models an identity whose credentials are owned by an external helper
// (aws/credential-process): the manager must never write them to the keyring.
type nonPersistentIdentity struct {
	stubIdentity
	fileCreds types.ICredentials
}

func (n *nonPersistentIdentity) Kind() string              { return "aws/credential-process" }
func (n *nonPersistentIdentity) PersistsCredentials() bool { return false }
func (n *nonPersistentIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	return n.fileCreds, nil
}

var (
	_ types.Identity              = (*nonPersistentIdentity)(nil)
	_ types.CredentialPersistence = (*nonPersistentIdentity)(nil)
)

// longLivedCreds are helper output with no session token and no expiration: exactly the shape
// the keyring cache accepts for aws/user, and the shape that used to leak into the keyring.
func longLivedCreds() *types.AWSCredentials {
	return &types.AWSCredentials{AccessKeyID: "AKIAHELPERLONGLIVED", SecretAccessKey: "secret", Region: "us-east-1"}
}

// writeLogStore records which aliases were written, because testStore.Delete ignores the realm
// and would erase a just-written entry when the manager cleans up the legacy pre-realm one.
type writeLogStore struct {
	testStore
	stored []string
}

func (r *writeLogStore) Store(alias string, creds types.ICredentials, realm string) error {
	r.stored = append(r.stored, alias)
	return r.testStore.Store(alias, creds, realm)
}

func newPersistenceTestManager(store types.CredentialStore, identities map[string]types.Identity) *manager {
	cfg := &schema.AuthConfig{Identities: map[string]schema.Identity{}}
	for name := range identities {
		cfg.Identities[name] = schema.Identity{Kind: identities[name].Kind()}
	}
	return &manager{
		config:          cfg,
		providers:       map[string]types.Provider{},
		identities:      identities,
		credentialStore: store,
		realm:           realm.RealmInfo{Value: "test-realm"},
	}
}

// Regression: `auth whoami -i <credential-process identity>` stored the helper's long-lived
// output in the keyring, contradicting the documented "never stored in the keyring" contract.
func TestCacheWhoamiCredentials_NonPersistentIdentityNotWrittenToKeyring(t *testing.T) {
	store := &writeLogStore{testStore: testStore{data: map[string]any{}}}
	m := newPersistenceTestManager(store, map[string]types.Identity{
		"cp":     &nonPersistentIdentity{},
		"normal": stubIdentity{provider: "p"},
	})

	creds := longLivedCreds()
	info := &types.WhoamiInfo{}
	m.cacheWhoamiCredentials("cp", creds, info)

	assert.NotContains(t, store.stored, "cp", "helper-owned credentials must never reach the keyring")
	assert.Equal(t, "cp", info.CredentialsRef, "the in-memory reference is still set")

	// Control: an identity without the opt-out is persisted as before, so the assertion above
	// is sensitive to the policy and not to an unrelated early return.
	m.cacheWhoamiCredentials("normal", longLivedCreds(), &types.WhoamiInfo{})
	assert.Contains(t, store.stored, "normal")
}

func TestCacheWhoamiCredentials_NonPersistentIdentityPurgesStaleEntry(t *testing.T) {
	store := &testStore{data: map[string]any{"cp": longLivedCreds()}}
	m := newPersistenceTestManager(store, map[string]types.Identity{"cp": &nonPersistentIdentity{}})

	m.cacheWhoamiCredentials("cp", longLivedCreds(), &types.WhoamiInfo{})

	assert.NotContains(t, store.data, "cp", "an entry written by an older version must self-heal")
}

func TestLoadCredentialsWithFallback_NonPersistentIdentityIgnoresKeyring(t *testing.T) {
	stale := &types.AWSCredentials{AccessKeyID: "AKIASTALEKEYRING", SecretAccessKey: "stale"}
	fresh := &types.AWSCredentials{AccessKeyID: "AKIAFRESHFILES", SecretAccessKey: "fresh"}
	store := &testStore{data: map[string]any{"cp": stale}}
	m := newPersistenceTestManager(store, map[string]types.Identity{"cp": &nonPersistentIdentity{fileCreds: fresh}})

	got, err := m.loadCredentialsWithFallback(context.Background(), "cp")

	require.NoError(t, err)
	gotAWS, ok := got.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "AKIAFRESHFILES", gotAWS.AccessKeyID, "a stale keyring entry must not be served")
}

func TestAuthenticateIdentityChain_NonPersistentStepNotWrittenToKeyring(t *testing.T) {
	store := &testStore{data: map[string]any{}}
	step := &chainStepIdentity{creds: longLivedCreds()}
	m := newPersistenceTestManager(store, map[string]types.Identity{"cp": step})
	m.chain = []string{"provider", "cp"}

	_, err := m.authenticateIdentityChain(context.Background(), 1, longLivedCreds())

	require.NoError(t, err)
	assert.NotContains(t, store.data, "cp")
}

// chainStepIdentity is a non-persistent identity that returns fixed credentials when authenticated.
type chainStepIdentity struct {
	nonPersistentIdentity
	creds types.ICredentials
}

func (c *chainStepIdentity) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
	return c.creds, nil
}

// Regression: `auth logout -i <credential-process identity>` left the keyring entry in place
// (deletedKeychain=false) and chains kept reusing it. Logout must remove it without --keychain.
func TestLogout_NonPersistentIdentityRemovesKeyringEntryWithoutKeychainFlag(t *testing.T) {
	store := &testStore{data: map[string]any{"cp": longLivedCreds(), "normal": longLivedCreds()}}
	m := newPersistenceTestManager(store, map[string]types.Identity{
		"cp":     &nonPersistentIdentity{},
		"normal": stubIdentity{provider: "p"},
	})

	require.NoError(t, m.Logout(context.Background(), "cp", false))
	assert.NotContains(t, store.data, "cp", "logout must really log out")

	// Negative path: a regular identity keeps its (possibly long-lived) keyring credentials
	// unless --keychain is requested, as documented.
	require.NoError(t, m.Logout(context.Background(), "normal", false))
	assert.Contains(t, store.data, "normal")
}

// countingStandaloneRoot is a standalone chain root that counts how often it authenticates.
type countingStandaloneRoot struct {
	stubIdentity
	calls atomic.Int32
}

func (c *countingStandaloneRoot) Kind() string       { return "aws/credential-process" }
func (c *countingStandaloneRoot) IsStandalone() bool { return true }
func (c *countingStandaloneRoot) AuthenticateStandalone(_ context.Context) (types.ICredentials, error) {
	c.calls.Add(1)
	return longLivedCreds(), nil
}

// invalidChildIdentity fails configuration validation.
type invalidChildIdentity struct{ stubIdentity }

func (invalidChildIdentity) Validate() error { return errUtils.ErrInvalidIdentityConfig }

// Regression: a misconfigured identity later in the chain was only reported after the earlier
// steps had already authenticated, so an upstream credential_process helper ran (and could prompt
// or open a browser) before the configuration error surfaced.
func TestAuthenticateChain_ValidatesEveryIdentityBeforeAuthenticatingAnyStep(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	root := &countingStandaloneRoot{}
	store := &testStore{data: map[string]any{}}
	m := newPersistenceTestManager(store, map[string]types.Identity{
		"upstream": root,
		"child":    invalidChildIdentity{},
	})
	m.chain = []string{"upstream", "child"}

	_, err := m.authenticateChain(context.Background(), "child")

	require.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
	assert.Zero(t, root.calls.Load(), "the upstream helper must not run when a later identity is invalid")
}

// Negative path: a valid chain still authenticates its root.
func TestAuthenticateChain_ValidChainStillAuthenticates(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	root := &countingStandaloneRoot{}
	m := newPersistenceTestManager(&testStore{data: map[string]any{}}, map[string]types.Identity{"upstream": root})
	m.chain = []string{"upstream"}

	_, err := m.authenticateChain(context.Background(), "upstream")

	require.NoError(t, err)
	assert.Equal(t, int32(1), root.calls.Load())
}
