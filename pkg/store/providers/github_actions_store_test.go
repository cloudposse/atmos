package providers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"

	storepkg "github.com/cloudposse/atmos/pkg/store"
)

// fakeGitHubActionsClient is an in-memory gitHubActionsClient for tests (no network).
type fakeGitHubActionsClient struct {
	secrets map[string]string
	putErr  error
	hasErr  error
	delErr  error
	puts    []string // names written, in order.
}

var _ gitHubActionsClient = (*fakeGitHubActionsClient)(nil)

func newFakeGitHubActionsClient() *fakeGitHubActionsClient {
	return &fakeGitHubActionsClient{secrets: map[string]string{}}
}

func (f *fakeGitHubActionsClient) PutSecret(_ context.Context, name, value string) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.secrets[name] = value
	f.puts = append(f.puts, name)
	return nil
}

func (f *fakeGitHubActionsClient) HasSecret(_ context.Context, name string) (bool, error) {
	if f.hasErr != nil {
		return false, f.hasErr
	}
	_, ok := f.secrets[name]
	return ok, nil
}

func (f *fakeGitHubActionsClient) DeleteSecret(_ context.Context, name string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.secrets, name)
	return nil
}

// newTestStore builds a store wired to a fake client and an explicit CI predicate.
func newTestStore(client gitHubActionsClient, opts *GitHubActionsStoreOptions, ci bool) *GitHubActionsStore {
	return &GitHubActionsStore{
		options: *opts,
		prefix:  opts.Prefix,
		isCI:    func() bool { return ci },
		client:  client,
	}
}

func TestNewGitHubActionsStore_RequiresOwnerRepo(t *testing.T) {
	tests := []struct {
		name    string
		opts    GitHubActionsStoreOptions
		wantErr bool
	}{
		{name: "missing owner", opts: GitHubActionsStoreOptions{Repo: "infra"}, wantErr: true},
		{name: "missing repo", opts: GitHubActionsStoreOptions{Owner: "acme"}, wantErr: true},
		{name: "both present", opts: GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGitHubActionsStore(&tt.opts)
			if tt.wantErr {
				require.ErrorIs(t, err, storepkg.ErrGitHubOwnerRepoRequired)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestGitHubSecretName(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		key     string
		want    string
		wantErr error
	}{
		{name: "plain key uppercased", key: "db_password", want: "DB_PASSWORD"},
		{name: "with prefix", prefix: "atmos", key: "db_password", want: "ATMOS_DB_PASSWORD"},
		{name: "sanitizes separators", key: "api-key.v2", want: "API_KEY_V2"},
		{name: "empty key", key: "", wantErr: storepkg.ErrEmptyKey},
		{name: "github prefix rejected", prefix: "github", key: "token", wantErr: storepkg.ErrGitHubInvalidSecretName},
		{name: "leading digit rejected", key: "1password", wantErr: storepkg.ErrGitHubInvalidSecretName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := githubSecretName(tt.prefix, tt.key)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGitHubActionsStore_Set_WritesEncodedValue(t *testing.T) {
	fake := newFakeGitHubActionsClient()
	store := newTestStore(fake, &GitHubActionsStoreOptions{Owner: "acme", Repo: "infra", Prefix: "atmos"}, false)

	// String passes through.
	require.NoError(t, store.Set("dev", "vpc", "db_password", "s3cr3t"))
	assert.Equal(t, "s3cr3t", fake.secrets["ATMOS_DB_PASSWORD"])

	// Non-string is JSON-encoded.
	require.NoError(t, store.Set("dev", "vpc", "config", map[string]any{"a": 1}))
	assert.JSONEq(t, `{"a":1}`, fake.secrets["ATMOS_CONFIG"])

	// stack/component do not affect the (flat, repo-global) secret name.
	require.NoError(t, store.Set("prod", "rds", "db_password", "other"))
	assert.Equal(t, "other", fake.secrets["ATMOS_DB_PASSWORD"])
	assert.Equal(t, []string{"ATMOS_DB_PASSWORD", "ATMOS_CONFIG", "ATMOS_DB_PASSWORD"}, fake.puts)

	// nil value is rejected.
	require.ErrorIs(t, store.Set("dev", "vpc", "db_password", nil), storepkg.ErrNilValue)
}

func TestGitHubActionsStore_Has(t *testing.T) {
	fake := newFakeGitHubActionsClient()
	fake.secrets["DB_PASSWORD"] = "x"
	store := newTestStore(fake, &GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"}, false)

	has, err := store.Has("dev", "vpc", "db_password")
	require.NoError(t, err)
	assert.True(t, has, "existing secret should report initialized")

	has, err = store.Has("dev", "vpc", "missing")
	require.NoError(t, err)
	assert.False(t, has, "absent secret should report not-initialized")

	// Transport errors propagate.
	fake.hasErr = errors.New("boom")
	_, err = store.Has("dev", "vpc", "db_password")
	require.Error(t, err)
}

func TestGitHubActionsStore_Delete_Idempotent(t *testing.T) {
	fake := newFakeGitHubActionsClient()
	fake.secrets["DB_PASSWORD"] = "x"
	store := newTestStore(fake, &GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"}, false)

	require.NoError(t, store.Delete("dev", "vpc", "db_password"))
	_, ok := fake.secrets["DB_PASSWORD"]
	assert.False(t, ok)

	// Deleting again is not an error (fake delete is a no-op for missing keys).
	require.NoError(t, store.Delete("dev", "vpc", "db_password"))
}

func TestGitHubActionsStore_Get_CIGating(t *testing.T) {
	t.Run("blocked outside CI", func(t *testing.T) {
		store := newTestStore(newFakeGitHubActionsClient(), &GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"}, false)
		_, err := store.Get("dev", "vpc", "db_password")
		require.ErrorIs(t, err, storepkg.ErrGitHubSecretValueCIOnly)
	})

	t.Run("allowed when ci.enabled forces it", func(t *testing.T) {
		t.Setenv("DB_PASSWORD", "from-env")
		opts := GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"}
		opts.CI.Enabled = true
		store := newTestStore(newFakeGitHubActionsClient(), &opts, false)
		got, err := store.Get("dev", "vpc", "db_password")
		require.NoError(t, err)
		assert.Equal(t, "from-env", got)
	})

	t.Run("allowed when detected as a runner", func(t *testing.T) {
		t.Setenv("DB_PASSWORD", "from-env")
		store := newTestStore(newFakeGitHubActionsClient(), &GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"}, true)
		got, err := store.Get("dev", "vpc", "db_password")
		require.NoError(t, err)
		assert.Equal(t, "from-env", got)
	})

	t.Run("missing env value reports not-initialized", func(t *testing.T) {
		store := newTestStore(newFakeGitHubActionsClient(), &GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"}, true)
		_, err := store.Get("dev", "vpc", "db_password")
		require.ErrorIs(t, err, storepkg.ErrGitHubSecretNotInEnv)
	})

	t.Run("structured JSON value round-trips", func(t *testing.T) {
		t.Setenv("CONFIG", `{"a":1}`)
		store := newTestStore(newFakeGitHubActionsClient(), &GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"}, true)
		got, err := store.Get("dev", "vpc", "config")
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"a": float64(1)}, got)
	})
}

func TestGitHubActionsStore_EnvHint(t *testing.T) {
	withEnv := newTestStore(newFakeGitHubActionsClient(),
		&GitHubActionsStoreOptions{Owner: "acme", Repo: "infra", Environment: "production"}, false)
	assert.Contains(t, withEnv.envHint("DB_PASSWORD"), "environment: production")
	assert.Contains(t, withEnv.envHint("DB_PASSWORD"), "secrets.DB_PASSWORD")

	noEnv := newTestStore(newFakeGitHubActionsClient(),
		&GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"}, false)
	assert.NotContains(t, noEnv.envHint("DB_PASSWORD"), "environment:")
	assert.Contains(t, noEnv.envHint("DB_PASSWORD"), "secrets.DB_PASSWORD")
}

func TestGitHubActionsStore_Get_EnrichedErrors(t *testing.T) {
	// Neutralize ambient GitHub Actions vars so the alignment check never makes a network call.
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITHUB_REPOSITORY", "")

	opts := GitHubActionsStoreOptions{Owner: "acme", Repo: "infra", Environment: "production"}
	store := newTestStore(newFakeGitHubActionsClient(), &opts, true) // ci=true allows the read path.

	_, err := store.Get("dev", "vpc", "db_password")
	require.ErrorIs(t, err, storepkg.ErrGitHubSecretNotInEnv)
	assert.Contains(t, err.Error(), "environment: production")
	assert.Contains(t, err.Error(), "secrets.DB_PASSWORD")

	// CI-only error (reads not allowed) also names the environment + mapping requirement.
	blocked := newTestStore(newFakeGitHubActionsClient(), &opts, false)
	_, err = blocked.Get("dev", "vpc", "db_password")
	require.ErrorIs(t, err, storepkg.ErrGitHubSecretValueCIOnly)
	assert.Contains(t, err.Error(), "environment: production")
}

func TestSealSecret_RoundTrip(t *testing.T) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err)

	plaintext := "super-secret-value"
	sealedB64, err := sealSecret(base64.StdEncoding.EncodeToString(pub[:]), plaintext)
	require.NoError(t, err)

	sealed, err := base64.StdEncoding.DecodeString(sealedB64)
	require.NoError(t, err)

	decrypted, ok := box.OpenAnonymous(nil, sealed, pub, priv)
	require.True(t, ok, "sealed box should decrypt with the matching keypair")
	assert.Equal(t, plaintext, string(decrypted))
}

func TestSealSecret_Errors(t *testing.T) {
	t.Run("invalid base64", func(t *testing.T) {
		_, err := sealSecret("not-base64!!!", "x")
		require.ErrorIs(t, err, storepkg.ErrGitHubSealSecret)
	})
	t.Run("wrong key size", func(t *testing.T) {
		_, err := sealSecret(base64.StdEncoding.EncodeToString([]byte("tooshort")), "x")
		require.ErrorIs(t, err, storepkg.ErrGitHubPublicKeySize)
	})
}

func TestGitHubActionsStore_RegistryWiring(t *testing.T) {
	// Canonical kind and legacy type both resolve to a GitHub Actions store via the registry.
	canonical, err := storepkg.NewStoreRegistry(&storepkg.StoresConfig{
		"gha": storepkg.StoreConfig{Kind: "github/actions", Options: map[string]any{"owner": "acme", "repo": "infra"}},
	})
	require.NoError(t, err)
	_, ok := canonical["gha"].(*GitHubActionsStore)
	assert.True(t, ok, "canonical kind should build a GitHub Actions store")

	legacy, err := storepkg.NewStoreRegistry(&storepkg.StoresConfig{
		"gha": storepkg.StoreConfig{Type: "github-actions", Options: map[string]any{"owner": "acme", "repo": "infra"}},
	})
	require.NoError(t, err)
	_, ok = legacy["gha"].(*GitHubActionsStore)
	assert.True(t, ok, "legacy type should build a GitHub Actions store")

	// Secret-by-default: a GitHub Actions store is marked secret even when the config omits it.
	cfg := storepkg.StoresConfig{
		"gha": {Kind: "github/actions", Options: map[string]any{"owner": "acme", "repo": "infra"}},
	}
	storepkg.ApplySecretDefaults(cfg)
	assert.True(t, cfg["gha"].Secret, "GitHub Actions store should default to secret: true")
}

func TestGitHubActionsStore_BuildsViaRegistry(t *testing.T) {
	cfg := storepkg.StoreConfig{
		Kind:    "github/actions",
		Secret:  true,
		Options: map[string]any{"owner": "acme", "repo": "infra", "environment": "production"},
	}
	s, err := buildGitHubActionsStore("gha", cfg)
	require.NoError(t, err)
	require.NotNil(t, s)

	gha, ok := s.(*GitHubActionsStore)
	require.True(t, ok)
	assert.Equal(t, "acme", gha.options.Owner)
	assert.Equal(t, "production", gha.options.Environment)
}
