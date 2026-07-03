package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// errResolveAuth is a sentinel used to assert the resolver-error path propagates verbatim.
var errResolveAuth = errors.New("resolve failed")

// TestSecretsManagerStore_EnsureClient_IdentityNoResolver verifies that calling a public
// operation on an identity-based store with no injected resolver returns ErrIdentityNotConfigured.
// This drives ensureClient() into the identity branch and initIdentityClient()'s guard.
func TestSecretsManagerStore_EnsureClient_IdentityNoResolver(t *testing.T) {
	delim := "-"
	s := &SecretsManagerStore{
		identityName:   "aws/admin",
		region:         "us-east-1",
		stackDelimiter: &delim,
		// No authResolver injected.
	}

	_, err := s.Get("prod", "api", "K")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrIdentityNotConfigured)
}

// TestSecretsManagerStore_EnsureClient_IdentityResolverError verifies that a resolver error
// is wrapped with ErrAuthContextNotAvailable and surfaced through a public operation.
func TestSecretsManagerStore_EnsureClient_IdentityResolverError(t *testing.T) {
	ctrl := gomock.NewController(t)

	resolver := NewMockAuthContextResolver(ctrl)
	resolver.EXPECT().
		ResolveAWSAuthContext(gomock.Any(), "aws/admin").
		Return(nil, errResolveAuth)

	delim := "-"
	s := &SecretsManagerStore{
		identityName:   "aws/admin",
		region:         "us-east-1",
		stackDelimiter: &delim,
		authResolver:   resolver,
	}

	err := s.Set("prod", "api", "K", "v")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthContextNotAvailable)
	// The underlying resolver error is preserved in the chain.
	assert.ErrorIs(t, err, errResolveAuth)
}

// TestSecretsManagerStore_InitIdentityClient_FullPath drives initIdentityClient() through the
// success path: the resolver returns a full AWSAuthConfig (creds/config files, profile, region),
// each config option is appended, LoadDefaultConfig succeeds, and a real client is constructed.
func TestSecretsManagerStore_InitIdentityClient_FullPath(t *testing.T) {
	ctrl := gomock.NewController(t)

	tmpDir := t.TempDir()
	credsFile := filepath.Join(tmpDir, "credentials")
	configFile := filepath.Join(tmpDir, "config")

	require.NoError(t, os.WriteFile(credsFile,
		[]byte("[prod]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = wJalrXUtnFEMI\n"), 0o600))
	require.NoError(t, os.WriteFile(configFile,
		[]byte("[profile prod]\nregion = us-east-1\n"), 0o600))

	resolver := NewMockAuthContextResolver(ctrl)
	resolver.EXPECT().
		ResolveAWSAuthContext(gomock.Any(), "prod-admin").
		Return(&AWSAuthConfig{
			CredentialsFile: credsFile,
			ConfigFile:      configFile,
			Profile:         "prod",
			Region:          "us-east-1",
		}, nil)

	delim := "-"
	s := &SecretsManagerStore{
		identityName:   "prod-admin",
		region:         "us-east-1",
		stackDelimiter: &delim,
		authResolver:   resolver,
	}

	require.NoError(t, s.ensureClient())
	assert.NotNil(t, s.client, "identity client must be constructed")
}

// TestSecretsManagerStore_InitIdentityClient_RegionFromAuthContext covers the branch where the
// store has no region of its own and falls back to the region from the resolved auth context.
func TestSecretsManagerStore_InitIdentityClient_RegionFromAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)

	resolver := NewMockAuthContextResolver(ctrl)
	resolver.EXPECT().
		ResolveAWSAuthContext(gomock.Any(), "prod-admin").
		Return(&AWSAuthConfig{
			Region: "eu-west-1",
		}, nil)

	delim := "-"
	s := &SecretsManagerStore{
		identityName:   "prod-admin",
		region:         "", // No store region → falls back to auth context region.
		stackDelimiter: &delim,
		authResolver:   resolver,
	}

	require.NoError(t, s.ensureClient())
	assert.NotNil(t, s.client)
}

// TestSecretsManagerStore_EnsureClient_OnlyOnce verifies the sync.Once guard: when a client is
// already set, ensureClient short-circuits and never invokes the resolver, and the cached
// initErr is returned on subsequent calls without re-running init.
func TestSecretsManagerStore_EnsureClient_OnlyOnce(t *testing.T) {
	ctrl := gomock.NewController(t)

	// Resolver must never be called because a client is already present.
	resolver := NewMockAuthContextResolver(ctrl)

	delim := "-"
	s := &SecretsManagerStore{
		client:         newFakeSecretsManager(),
		identityName:   "aws/admin",
		region:         "us-east-1",
		stackDelimiter: &delim,
		authResolver:   resolver,
	}

	require.NoError(t, s.ensureClient())
	// Second call hits the cached initErr (nil) without re-initializing.
	require.NoError(t, s.ensureClient())
}

// TestSecretsManagerStore_InitDefaultClient_LazyDefault drives the default (no-identity) lazy
// path via ensureClient(): with no identity and no pre-set client, ensureClient calls
// initDefaultClient(), which loads the default AWS config (no network) and builds a client.
func TestSecretsManagerStore_InitDefaultClient_LazyDefault(t *testing.T) {
	// Pin a region via env so LoadDefaultConfig does not probe IMDS/metadata services.
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	delim := "-"
	s := &SecretsManagerStore{
		// identityName empty → default path; client nil → must be lazily built.
		region:         "us-east-1",
		stackDelimiter: &delim,
	}

	require.NoError(t, s.ensureClient())
	assert.NotNil(t, s.client, "default lazy path must construct a client")

	// Calling again is a no-op (sync.Once) and stays successful.
	require.NoError(t, s.ensureClient())
}

// TestSecretsManagerStore_InitDefaultClient_Direct exercises initDefaultClient() directly to
// cover the region assignment and client construction in isolation.
func TestSecretsManagerStore_InitDefaultClient_Direct(t *testing.T) {
	t.Setenv("AWS_REGION", "us-west-2")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	s := &SecretsManagerStore{region: "us-west-2"}
	require.NoError(t, s.initDefaultClient())
	require.NotNil(t, s.client)
}

// TestSecretsManagerStore_PutOrCreate_PutSucceedsExisting covers putOrCreate's happy path where
// PutSecretValue succeeds because the secret already exists (no CreateSecret call).
func TestSecretsManagerStore_PutOrCreate_PutSucceedsExisting(t *testing.T) {
	fake := newFakeSecretsManager()
	fake.data["atmos/secrets/prod/api/K"] = `"old"`
	s := newTestASMStore(fake)

	require.NoError(t, s.putOrCreate(context.TODO(), "atmos/secrets/prod/api/K", `"new"`))
	assert.Equal(t, `"new"`, fake.data["atmos/secrets/prod/api/K"])
}

// TestSecretsManagerStore_PutOrCreate_CreatesWhenMissing covers putOrCreate's create branch:
// PutSecretValue returns ResourceNotFound, then CreateSecret persists the value.
func TestSecretsManagerStore_PutOrCreate_CreatesWhenMissing(t *testing.T) {
	fake := newFakeSecretsManager()
	s := newTestASMStore(fake)

	const id = "atmos/secrets/prod/api/NEW"
	require.NoError(t, s.putOrCreate(context.TODO(), id, `"v"`))
	assert.Equal(t, `"v"`, fake.data[id], "secret should have been created")
}
