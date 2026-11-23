package auth

import (
	"context"
	"encoding/json"
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

// Compile-time sentinels for the schema fields the persisted digest reads or skips.
var (
	_ = schema.Provider{Kind: "", StartURL: "", URL: "", Region: "", Username: "", Password: "", Spec: nil, Tags: nil, Default: false}
	_ = schema.Identity{Kind: "", Provider: "", Via: nil, Principal: nil, Spec: nil, Emulator: "", Credentials: nil, Alias: "", Env: nil}
)

func digestManager(cfg *schema.AuthConfig) *manager {
	return &manager{config: cfg, realm: realm.RealmInfo{Value: "acme"}}
}

// TestPersistedStepConfig_CanonicalFields pins exactly which fields the persisted digest hashes,
// and that secrets never reach it.
func TestPersistedStepConfig_CanonicalFields(t *testing.T) {
	m := digestManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{"prov": {
			Kind: "aws/saml", URL: "https://idp.example.com", Region: "us-east-1", Username: "me",
			Password: "hunter2", Driver: "Browser", Default: true, Tags: []string{"x"},
			Session: &schema.SessionConfig{Duration: "1h"},
			Spec:    map[string]any{"client_secret": "s", "token_file_path": "/var/run/token", "audience": "sts"},
		}},
		Identities: map[string]schema.Identity{"mid": {
			Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "prov"},
			Principal: map[string]any{
				"assume_role": "arn:aws:iam::111111111111:role/admin",
				"external_id": "e",
				"nested":      []any{map[string]any{"session_token": "x", "name": "keep"}},
			},
			Credentials: map[string]any{"access_key_id": "AKIA", "secret_access_key": "s"},
			Alias:       "a", Default: true, Required: true, Env: []schema.EnvironmentVariable{{Key: "K", Value: "V"}},
		}},
	})

	provider, err := json.Marshal(m.persistedStepConfigAt(0, "prov"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"step":"provider","name":"prov","provider":{"kind":"aws/saml","url":"https://idp.example.com","region":"us-east-1","username":"me","spec":{"audience":"sts","token_file_path":"/var/run/token"}}}`, string(provider))

	identity, err := json.Marshal(m.persistedStepConfigAt(1, "mid"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"step":"identity","name":"mid","identity":{"kind":"aws/assume-role","via":{"provider":"prov"},"principal":{"assume_role":"arn:aws:iam::111111111111:role/admin","nested":[{"name":"keep"}]}}}`, string(identity))

	for _, secret := range []string{"hunter2", "client_secret", "external_id", "session_token", "AKIA", "secret_access_key"} {
		assert.NotContains(t, string(provider)+string(identity), secret)
	}
}

// TestChainPrefixDigest_TracksOnlyPrincipalFields verifies the digest changes with the fields
// that select a principal, including upstream steps, and ignores the others.
func TestChainPrefixDigest_TracksOnlyPrincipalFields(t *testing.T) {
	base := prodConfig.authConfig
	chain := []string{"prov", "mid", "target"}
	digestOf := func(cfg *schema.AuthConfig, index int) string {
		d, ok := digestManager(cfg).chainPrefixDigest(chain, index)
		require.True(t, ok)
		return d
	}
	want := digestOf(base(), 1)
	assert.Len(t, want, 64, "a hex SHA-256")
	assert.Equal(t, want, digestOf(base(), 1), "stable across managers and processes")

	ignored := base()
	mid := ignored.Identities["mid"]
	mid.Default, mid.Tags, mid.Session = true, []string{"t"}, &schema.SessionConfig{Duration: "12h"}
	mid.Credentials = map[string]any{"secret_access_key": "rotated"}
	ignored.Identities["mid"] = mid
	prov := ignored.Providers["prov"]
	prov.Password = "rotated"
	ignored.Providers["prov"] = prov
	assert.Equal(t, want, digestOf(ignored, 1), "secrets and presentation fields must not change the digest")

	changedUpstream := base()
	prov = changedUpstream.Providers["prov"]
	prov.Region = "eu-west-1"
	changedUpstream.Providers["prov"] = prov
	assert.NotEqual(t, want, digestOf(changedUpstream, 1), "a changed upstream step changes downstream digests")

	changedDownstream := base()
	target := changedDownstream.Identities["target"]
	target.Principal = map[string]any{"assume_role": "arn:aws:iam::333333333333:role/other"}
	changedDownstream.Identities["target"] = target
	assert.Equal(t, want, digestOf(changedDownstream, 1), "a later step never changes an earlier step's digest")

	_, ok := digestManager(base()).chainPrefixDigest(chain, 3)
	assert.False(t, ok, "an out-of-range index has no digest")
	unserializable := base()
	mid = unserializable.Identities["mid"]
	mid.Spec = map[string]any{"bad": func() {}}
	unserializable.Identities["mid"] = mid
	_, ok = digestManager(unserializable).chainPrefixDigest(chain, 1)
	assert.False(t, ok, "an unserializable configuration has no digest")
}

// TestNamedStepDigest_MatchesChainIndex verifies the digest written for an identity on its own
// (login, whoami) equals the one checked when it is a step of a longer chain.
func TestNamedStepDigest_MatchesChainIndex(t *testing.T) {
	m := digestManager(prodConfig.authConfig())
	chain := []string{"prov", "mid", "target"}
	for i, name := range chain {
		want, ok := m.chainPrefixDigest(chain, i)
		require.True(t, ok)
		got, ok := m.namedStepDigest(name)
		require.True(t, ok)
		assert.Equal(t, want, got, name)
	}

	broken := prodConfig.authConfig()
	broken.Identities["orphan"] = schema.Identity{Kind: "aws/assume-role"}
	_, ok := digestManager(broken).namedStepDigest("orphan")
	assert.False(t, ok, "an identity without a chain has no digest")
	_, ok = digestManager(nil).namedStepDigest("prov")
	assert.True(t, ok, "a step without configuration still has a stable digest")
}

// TestIsSecretConfigKey pins the secret-key heuristic used inside principal and spec.
func TestIsSecretConfigKey(t *testing.T) {
	for key, secret := range map[string]bool{
		"password": true, "client-secret": true, "SessionToken": true, "private_key": true,
		"external_id": true, "api_key": true, "credentials": true, "client_assertion": true,
		"token_file_path": false, "password_env": false, "secret_arn": false, "token_url": false,
		"assume_role": false, "account": false, "region": false, "name": false,
	} {
		assert.Equal(t, secret, isSecretConfigKey(key), key)
	}
}

// TestChainStepFileCache_RecordMustMatchCredentials verifies a digest record only vouches for
// the exact credentials it was written for: files rewritten by something else are not reused.
func TestChainStepFileCache_RecordMustMatchCredentials(t *testing.T) {
	store := newStepDigestStore(t)
	files := &fakeFiles{}

	login := newStepDigestProcess(t, store, prodConfig, fileSessionCreds("mid"), files)
	login.authenticate(t, "mid")

	// Another tool (or an older Atmos) rewrites the files with a later expiration.
	rewritten := fileSessionCreds("mid-rewritten").(*types.AWSCredentials)
	rewritten.Expiration = time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second).Format(time.RFC3339)
	files.creds = rewritten

	next := newStepDigestProcess(t, store, prodConfig, fileSessionCreds("mid"), files)
	next.authenticate(t, "target")
	assert.Equal(t, int32(1), next.mid.callCount.Load(), "files the digest record does not describe must not be reused")
}

// unsupportedDigestStore is a digest-capable store that cannot persist digests, like the noop
// keyring (which persists nothing).
type unsupportedDigestStore struct {
	credentials.ConfigDigestStore
	types.CredentialStore
}

func (unsupportedDigestStore) SupportsConfigDigests() bool { return false }

// TestChainStepCache_NoDigestPersistenceKeepsNameOnlyReuse documents the noop keyring
// limitation: no digest can be recorded, so file-backed steps keep the previous name-only reuse
// instead of being re-authenticated by every process.
func TestChainStepCache_NoDigestPersistenceKeepsNameOnlyReuse(t *testing.T) {
	inner := newStepDigestStore(t)
	digestStore, ok := inner.(credentials.ConfigDigestStore)
	require.True(t, ok)
	store := unsupportedDigestStore{ConfigDigestStore: digestStore, CredentialStore: inner}
	files := &fakeFiles{}

	login := newStepDigestProcess(t, store, prodConfig, fileSessionCreds("mid"), files)
	login.authenticate(t, "mid")
	_, err := digestStore.RetrieveExternalConfigDigest("mid", "acme")
	require.ErrorIs(t, err, credentials.ErrDataNotFound, "nothing is recorded when digests cannot be persisted")

	p := newStepDigestProcess(t, store, prodConfig, fileSessionCreds("mid"), files)
	p.authenticate(t, "target")
	assert.Equal(t, int32(0), p.mid.callCount.Load())
}

// failingDigestStore is a digest-capable store whose digest record writes fail.
type failingDigestStore struct {
	credentials.ConfigDigestStore
	types.CredentialStore
}

func (failingDigestStore) StoreExternalConfigDigest(string, string, credentials.ExternalConfigDigest) error {
	return assert.AnError
}

// TestRecordExternalStepDigest_FailureIsNonFatal verifies a failed record write never fails the
// login; the files are then simply not reused as an intermediate step.
func TestRecordExternalStepDigest_FailureIsNonFatal(t *testing.T) {
	inner := newStepDigestStore(t)
	digestStore, ok := inner.(credentials.ConfigDigestStore)
	require.True(t, ok)
	store := failingDigestStore{ConfigDigestStore: digestStore, CredentialStore: inner}
	files := &fakeFiles{}

	login := newStepDigestProcess(t, store, prodConfig, fileSessionCreds("mid"), files)
	login.authenticate(t, "mid")
	require.NotNil(t, files.creds)

	next := newStepDigestProcess(t, store, prodConfig, fileSessionCreds("mid"), files)
	next.authenticate(t, "target")
	assert.Equal(t, int32(1), next.mid.callCount.Load())
}

// TestVerifyChainStepDigest_Reasons verifies refused steps report why, and wrap the sentinel.
func TestVerifyChainStepDigest_Reasons(t *testing.T) {
	store := newStepDigestStore(t)
	m := newStepDigestProcess(t, store, prodConfig, nil, nil).m
	chain := []string{"prov", "mid", "target"}
	creds := persistableCreds("mid")

	err := m.verifyChainStepDigest(chain, 1, loadedCredentials{creds: creds, source: credentialSourceKeyring, digest: "other"})
	require.ErrorIs(t, err, errUtils.ErrChainStepConfigChanged)
	assert.Contains(t, err.Error(), "different configuration")

	err = m.verifyChainStepDigest(chain, 1, loadedCredentials{creds: creds, source: credentialSourceKeyring})
	require.ErrorIs(t, err, errUtils.ErrChainStepConfigChanged)
	assert.Contains(t, err.Error(), "older version")

	err = m.verifyChainStepDigest(chain, 1, loadedCredentials{creds: creds, source: credentialSourceIdentityStorage})
	require.ErrorIs(t, err, errUtils.ErrChainStepConfigChanged)
	assert.Contains(t, err.Error(), "identity credential files")

	m.config.Identities["mid"] = schema.Identity{Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "prov"}, Spec: map[string]any{"bad": func() {}}}
	err = m.verifyChainStepDigest(chain, 1, loadedCredentials{creds: creds, source: credentialSourceKeyring, digest: "x"})
	require.ErrorIs(t, err, errUtils.ErrChainStepConfigChanged)
	assert.Contains(t, err.Error(), "cannot be digested")

	// A configuration that cannot be digested is still stored, without a digest.
	require.NoError(t, m.storeChainStepCredentials(chain, 1, creds))
	_, digest, err := store.(credentials.ConfigDigestStore).RetrieveWithConfigDigest("mid", "acme")
	require.NoError(t, err)
	assert.Empty(t, digest)
}

// TestExpirationStamp verifies the record binding survives a round trip through files.
func TestExpirationStamp(t *testing.T) {
	assert.Empty(t, expirationStamp(nil))
	assert.Empty(t, expirationStamp(&types.AWSCredentials{}))
	exp := time.Date(2026, 11, 22, 12, 0, 0, 500, time.FixedZone("x", 3600))
	assert.Equal(t, "2026-11-22T11:00:00Z", expirationStamp(&testCreds{exp: &exp}))
}

// TestOperationCredentialStore_ForwardsDigests verifies operation-scoped targets keep their
// digests in the operation realm, like their credentials, and that a store without digest
// support reports it.
func TestOperationCredentialStore_ForwardsDigests(t *testing.T) {
	inner := newStepDigestStore(t)
	scope := &identityOperationScope{target: "target", realm: "operation-1"}
	op := &operationCredentialStore{CredentialStore: inner, scope: scope}
	require.True(t, op.SupportsConfigDigests())

	creds := persistableCreds("target")
	require.NoError(t, op.StoreWithConfigDigest("target", creds, "acme", "d1"))
	require.NoError(t, op.StoreExternalConfigDigest("target", "acme", credentials.ExternalConfigDigest{Digest: "d2"}))

	innerDigests := inner.(credentials.ConfigDigestStore)
	_, digest, err := innerDigests.RetrieveWithConfigDigest("target", "operation-1")
	require.NoError(t, err)
	assert.Equal(t, "d1", digest)
	record, err := innerDigests.RetrieveExternalConfigDigest("target", "operation-1")
	require.NoError(t, err)
	assert.Equal(t, "d2", record.Digest)
	_, err = innerDigests.RetrieveExternalConfigDigest("target", "acme")
	require.Error(t, err, "the operation target's record must not land in the shared realm")

	_, digest, err = op.RetrieveWithConfigDigest("target", "acme")
	require.NoError(t, err)
	assert.Equal(t, "d1", digest)
	record, err = op.RetrieveExternalConfigDigest("target", "acme")
	require.NoError(t, err)
	assert.Equal(t, "d2", record.Digest)

	require.NoError(t, op.DeleteExternalConfigDigest("target", "acme"))
	_, err = innerDigests.RetrieveExternalConfigDigest("target", "operation-1")
	require.ErrorIs(t, err, credentials.ErrDataNotFound, "the delete must reach the operation realm")

	plain := &operationCredentialStore{CredentialStore: &testStore{}, scope: scope}
	assert.False(t, plain.SupportsConfigDigests())
	require.ErrorIs(t, plain.StoreWithConfigDigest("target", creds, "acme", "d"), credentials.ErrNotSupported)
	require.ErrorIs(t, plain.StoreExternalConfigDigest("target", "acme", credentials.ExternalConfigDigest{}), credentials.ErrNotSupported)
	_, _, err = plain.RetrieveWithConfigDigest("target", "acme")
	require.ErrorIs(t, err, credentials.ErrNotSupported)
	_, err = plain.RetrieveExternalConfigDigest("target", "acme")
	require.ErrorIs(t, err, credentials.ErrNotSupported)
	require.ErrorIs(t, plain.DeleteExternalConfigDigest("target", "acme"), credentials.ErrNotSupported)
}

// TestChainStepCache_StoreWithoutDigestsKeepsNameOnlyReuse verifies custom stores that cannot
// keep digests are unaffected.
func TestChainStepCache_StoreWithoutDigestsKeepsNameOnlyReuse(t *testing.T) {
	t.Cleanup(resetProcessCredentialCache)
	store := &testStore{data: map[string]any{}}
	chain := []string{"prov", "mid", "target"}
	first := newStepDigestProcess(t, store, prodConfig, persistableCreds("mid"), nil)
	first.m.chain = chain
	_, err := first.m.authenticateChain(context.Background(), "target")
	require.NoError(t, err)
	changed := newStepDigestProcess(t, store, devConfig, persistableCreds("mid"), nil)
	changed.m.chain = chain
	_, err = changed.m.authenticateChain(context.Background(), "target")
	require.NoError(t, err)
	assert.Equal(t, int32(0), changed.mid.callCount.Load())
	_, err = changed.m.loadChainStepCredentials(context.Background(), chain, 1)
	require.NoError(t, err)
}

// TestRecordExternalStepDigest_SkipsUndigestableConfig verifies a login whose configuration
// cannot be digested records nothing, so its files are never vouched for.
func TestRecordExternalStepDigest_SkipsUndigestableConfig(t *testing.T) {
	store := newStepDigestStore(t)
	p := newStepDigestProcess(t, store, prodConfig, fileSessionCreds("mid"), &fakeFiles{})
	mid := p.m.config.Identities["mid"]
	mid.Spec = map[string]any{"bad": func() {}}
	p.m.config.Identities["mid"] = mid

	p.authenticate(t, "mid")
	_, err := store.(credentials.ConfigDigestStore).RetrieveExternalConfigDigest("mid", "acme")
	require.ErrorIs(t, err, credentials.ErrDataNotFound)
}
