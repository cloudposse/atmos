package auth

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/providers/mock"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields referenced by these tests.
var (
	_ = schema.Identity{Principal: map[string]any{}, Via: &schema.IdentityVia{Identity: ""}}
	_ = schema.Provider{StartURL: "", Password: ""}
	_ = schema.KeyringConfig{Type: ""}
)

// fakeFiles stands in for an identity's own credential files (for example the AWS
// credentials file), which survive across processes like the keyring does.
type fakeFiles struct {
	creds types.ICredentials
}

// fileBackedIdentity is a counting identity whose PostAuthenticate "writes" its credentials to
// shared fake files and whose LoadCredentials reads them back, like the AWS identities.
type fileBackedIdentity struct {
	*countingIdentity
	files *fakeFiles
}

func (i *fileBackedIdentity) PostAuthenticate(_ context.Context, params *types.PostAuthenticateParams) error {
	i.files.creds = params.Credentials
	return nil
}

func (i *fileBackedIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	if i.files == nil || i.files.creds == nil {
		return nil, credentials.ErrCredentialsNotFound
	}
	return i.files.creds, nil
}

// countingProvider returns persistable (registered) credentials and counts authentications.
type countingProvider struct {
	*testProvider
	creds types.ICredentials
	calls atomic.Int32
}

func (p *countingProvider) Authenticate(_ context.Context) (types.ICredentials, error) {
	p.calls.Add(1)
	return p.creds, nil
}

// stepDigestProcess is one simulated Atmos process for the chain prov -> mid -> target.
type stepDigestProcess struct {
	m        *manager
	provider *countingProvider
	mid      *countingIdentity
	target   *countingIdentity
}

// stepDigestConfig is the effective auth configuration of one simulated process.
type stepDigestConfig struct {
	startURL   string
	midAccount string
}

func (c stepDigestConfig) authConfig() *schema.AuthConfig {
	return &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"prov": {Kind: "aws/iam-identity-center", StartURL: c.startURL, Region: "us-east-1"},
		},
		Identities: map[string]schema.Identity{
			"mid": {
				Kind:      "aws/permission-set",
				Via:       &schema.IdentityVia{Provider: "prov"},
				Principal: map[string]any{"name": "Admin", "account": map[string]any{"id": c.midAccount}},
			},
			"target": {
				Kind:      "aws/assume-role",
				Via:       &schema.IdentityVia{Identity: "mid"},
				Principal: map[string]any{"assume_role": "arn:aws:iam::333333333333:role/deployer"},
			},
		},
	}
}

// newStepDigestProcess builds a fresh manager (a new process) sharing store and files with
// earlier processes. midCreds is what the intermediate step authenticates to; when files is
// set, the intermediate step persists its credentials there instead of in the keyring.
func newStepDigestProcess(t *testing.T, store types.CredentialStore, cfg stepDigestConfig, midCreds types.ICredentials, files *fakeFiles) *stepDigestProcess {
	t.Helper()
	resetProcessCredentialCache()
	exp := time.Now().UTC().Add(time.Hour)
	p := &stepDigestProcess{
		provider: &countingProvider{testProvider: &testProvider{name: "prov"}, creds: &mock.Credentials{AccessKeyID: "provider", Expiration: exp}},
		mid:      &countingIdentity{provider: "prov", creds: midCreds},
		target:   &countingIdentity{provider: "prov", creds: &mock.Credentials{AccessKeyID: "target", Expiration: exp}},
	}
	var mid types.Identity = p.mid
	if files != nil {
		mid = &fileBackedIdentity{countingIdentity: p.mid, files: files}
	}
	p.m = &manager{
		config:          cfg.authConfig(),
		providers:       map[string]types.Provider{"prov": p.provider},
		identities:      map[string]types.Identity{"mid": mid, "target": p.target},
		credentialStore: store,
		realm:           realm.RealmInfo{Value: "acme", Source: realm.SourceConfig},
	}
	return p
}

// authenticate runs the chain for the named target identity, as Authenticate does.
func (p *stepDigestProcess) authenticate(t *testing.T, target string) {
	t.Helper()
	chain, err := p.m.buildAuthenticationChain(target)
	require.NoError(t, err)
	p.m.chain = chain
	_, err = p.m.authenticateResolvedIdentity(context.Background(), target)
	require.NoError(t, err)
}

func newStepDigestStore(t *testing.T) types.CredentialStore {
	t.Helper()
	t.Setenv("ATMOS_KEYRING_TYPE", "memory")
	// Keep the realm-scoped legacy file cleanup away from the developer's config directory.
	t.Setenv("ATMOS_XDG_CONFIG_HOME", t.TempDir())
	t.Cleanup(resetProcessCredentialCache)
	return credentials.NewCredentialStoreWithConfig(&schema.AuthConfig{Keyring: schema.KeyringConfig{Type: "memory"}})
}

func persistableCreds(name string) types.ICredentials {
	return &mock.Credentials{AccessKeyID: name, Expiration: time.Now().UTC().Add(time.Hour)}
}

func fileSessionCreds(name string) types.ICredentials {
	return &types.AWSCredentials{
		AccessKeyID:     name,
		SecretAccessKey: "secret",
		SessionToken:    "session",
		Expiration:      time.Now().UTC().Add(time.Hour).Truncate(time.Second).Format(time.RFC3339),
	}
}

var (
	prodConfig = stepDigestConfig{startURL: "https://acme.awsapps.com/start", midAccount: "111111111111"}
	devConfig  = stepDigestConfig{startURL: "https://acme.awsapps.com/start", midAccount: "222222222222"}
)

// TestChainStepKeyringCache_ChangedIntermediateConfigMisses reproduces the cross-process
// part of #3720 item 2.1: a stack or component auth override gives the intermediate identity
// "mid" a different account in a later process. Its keyring entry, written for the old account,
// must not be reused as the source of the target step.
func TestChainStepKeyringCache_ChangedIntermediateConfigMisses(t *testing.T) {
	store := newStepDigestStore(t)

	first := newStepDigestProcess(t, store, prodConfig, persistableCreds("mid-prod"), nil)
	first.authenticate(t, "target")
	require.Equal(t, int32(1), first.mid.callCount.Load())

	// A later process with the same effective configuration reuses the intermediate step.
	same := newStepDigestProcess(t, store, prodConfig, persistableCreds("mid-prod"), nil)
	same.authenticate(t, "target")
	assert.Equal(t, int32(0), same.mid.callCount.Load(), "an unchanged intermediate step must be reused across processes")
	assert.Equal(t, int32(1), same.target.callCount.Load(), "the target step is always re-authenticated")

	// A later process whose override changes the intermediate step must re-authenticate it.
	changed := newStepDigestProcess(t, store, devConfig, persistableCreds("mid-dev"), nil)
	changed.authenticate(t, "target")
	assert.Equal(t, int32(1), changed.mid.callCount.Load(), "a changed intermediate step must not reuse credentials cached for another configuration")
}

// TestChainStepKeyringCache_ChangedProviderConfigMisses verifies the chain root is covered:
// provider credentials cached for one start URL are not reused for another.
func TestChainStepKeyringCache_ChangedProviderConfigMisses(t *testing.T) {
	store := newStepDigestStore(t)

	// Session credentials are not stored in the keyring, so the provider is the reuse point.
	first := newStepDigestProcess(t, store, prodConfig, fileSessionCreds("mid"), nil)
	first.authenticate(t, "target")
	require.Equal(t, int32(1), first.provider.calls.Load())

	same := newStepDigestProcess(t, store, prodConfig, fileSessionCreds("mid"), nil)
	same.authenticate(t, "target")
	assert.Equal(t, int32(0), same.provider.calls.Load(), "an unchanged provider must be reused across processes")

	other := prodConfig
	other.startURL = "https://other.awsapps.com/start"
	changed := newStepDigestProcess(t, store, other, fileSessionCreds("mid"), nil)
	changed.authenticate(t, "target")
	assert.Equal(t, int32(1), changed.provider.calls.Load(), "a changed provider must not reuse credentials cached for another configuration")
}

// TestChainStepFileCache_ChangedIntermediateConfigMisses covers credentials an identity keeps
// in its own files (AWS session credentials are never stored in the keyring): "mid" was the
// target of an earlier login, and a later chain through "mid" with a different account must
// not reuse those files.
func TestChainStepFileCache_ChangedIntermediateConfigMisses(t *testing.T) {
	store := newStepDigestStore(t)
	files := &fakeFiles{}

	login := newStepDigestProcess(t, store, prodConfig, fileSessionCreds("mid-prod"), files)
	login.authenticate(t, "mid")
	require.NotNil(t, files.creds)

	same := newStepDigestProcess(t, store, prodConfig, fileSessionCreds("mid-prod"), files)
	same.authenticate(t, "target")
	assert.Equal(t, int32(0), same.mid.callCount.Load(), "files written for the same configuration must be reused")

	changed := newStepDigestProcess(t, store, devConfig, fileSessionCreds("mid-dev"), files)
	changed.authenticate(t, "target")
	assert.Equal(t, int32(1), changed.mid.callCount.Load(), "files written for another configuration must not be reused")
}

// TestChainStepKeyringCache_EntryWithoutDigestMissesForIntermediateStep pins the upgrade
// behavior: an entry written by an older Atmos has no configuration digest, so it cannot be
// shown to match and is not reused as an intermediate step. This costs at most one extra
// authentication of that step, after which the entry carries a digest.
func TestChainStepKeyringCache_EntryWithoutDigestMissesForIntermediateStep(t *testing.T) {
	store := newStepDigestStore(t)
	require.NoError(t, store.Store("mid", persistableCreds("mid-old"), "acme"))

	upgraded := newStepDigestProcess(t, store, prodConfig, persistableCreds("mid"), nil)
	upgraded.authenticate(t, "target")
	assert.Equal(t, int32(1), upgraded.mid.callCount.Load(), "an entry without a digest must not be reused as an intermediate step")

	next := newStepDigestProcess(t, store, prodConfig, persistableCreds("mid"), nil)
	next.authenticate(t, "target")
	assert.Equal(t, int32(0), next.mid.callCount.Load(), "the rewritten entry carries a digest and is reused")
}

// TestChainStepKeyringCache_WhoamiCacheKeepsDigest verifies the target's keyring entry, which
// is written twice (by the chain and again for whoami), still matches when that identity is
// later an intermediate step.
func TestChainStepKeyringCache_WhoamiCacheKeepsDigest(t *testing.T) {
	store := newStepDigestStore(t)

	login := newStepDigestProcess(t, store, prodConfig, persistableCreds("mid"), nil)
	login.authenticate(t, "mid")

	chained := newStepDigestProcess(t, store, prodConfig, persistableCreds("mid"), nil)
	chained.authenticate(t, "target")
	assert.Equal(t, int32(0), chained.mid.callCount.Load())
}
