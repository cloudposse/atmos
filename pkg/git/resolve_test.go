package git

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels: fail the build immediately if schema fields used by
// the resolver are renamed.
var (
	_ = schema.GitRepository{Provider: "cli", URI: "", Branch: "", Remote: "", Workdir: ""}
	_ = schema.GitCloneConfig{Depth: 1, Filter: "blob:none", SingleBranch: true, Submodules: false}
	_ = schema.GitCommitConfig{Signing: "auto", Author: schema.GitAuthorConfig{Name: "", Email: ""}}
	_ = schema.GitAuthConfig{Identity: ""}
	_ = schema.GitInitConfig{From: "", KeepHistory: false}
)

func gitConfigFixture() *schema.GitConfig {
	retries := 5
	return &schema.GitConfig{
		Repositories: map[string]schema.GitRepository{
			"flux-deploy": {
				URI:    "https://github.com/acme/flux-deploy.git",
				Branch: "main",
				Auth:   schema.GitAuthConfig{Identity: "platform-admin"},
				Commit: schema.GitCommitConfig{
					Signing: "always",
					Author:  schema.GitAuthorConfig{Name: "atmos[bot]", Email: "bot@acme.com"},
				},
				Push:  schema.GitPushConfig{Retries: &retries},
				Clone: schema.GitCloneConfig{Depth: 1, SingleBranch: true},
				Init: schema.GitInitConfig{
					From:        "https://github.com/acme/template.git",
					KeepHistory: true,
				},
			},
			"minimal": {
				URI: "https://github.com/acme/minimal.git",
			},
		},
	}
}

func TestResolveRepositoryAppliesExplicitConfig(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())

	resolved, err := ResolveRepository(gitConfigFixture(), "flux-deploy")
	require.NoError(t, err)

	assert.Equal(t, "flux-deploy", resolved.Name)
	assert.Equal(t, "https://github.com/acme/flux-deploy.git", resolved.URI)
	assert.Equal(t, "main", resolved.Branch)
	assert.Equal(t, "platform-admin", resolved.Identity)
	assert.Equal(t, SigningAlways, resolved.Signing)
	require.NotNil(t, resolved.Author)
	assert.Equal(t, "atmos[bot]", resolved.Author.Name)
	assert.Equal(t, "bot@acme.com", resolved.Author.Email)
	assert.Equal(t, 5, resolved.PushRetries)
	assert.Equal(t, 1, resolved.Clone.Depth)
	assert.True(t, resolved.Clone.SingleBranch)
	assert.Equal(t, "https://github.com/acme/template.git", resolved.From)
	assert.True(t, resolved.KeepHistory)
}

func TestResolveRepositoryAppliesDefaults(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", cacheRoot)

	resolved, err := ResolveRepository(gitConfigFixture(), "minimal")
	require.NoError(t, err)

	assert.Equal(t, DefaultProviderName, resolved.Provider)
	assert.Equal(t, DefaultRemote, resolved.Remote)
	assert.Equal(t, SigningAuto, resolved.Signing)
	assert.Nil(t, resolved.Author)
	assert.Equal(t, DefaultPushRetries, resolved.PushRetries)
	assert.Equal(t, 0, resolved.Clone.Depth)
	assert.Empty(t, resolved.From)
	assert.False(t, resolved.KeepHistory)

	// Automatic XDG workdir: <cache>/atmos/git/repositories/<name>.
	expected := filepath.Join(cacheRoot, "atmos", "git", "repositories", "minimal")
	assert.Equal(t, expected, resolved.Workdir)
	assert.NoDirExists(t, expected)
}

func TestResolveRepositoryZeroRetriesDisables(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	zero := 0
	cfg := &schema.GitConfig{Repositories: map[string]schema.GitRepository{
		"r": {URI: "https://example.com/r.git", Push: schema.GitPushConfig{Retries: &zero}},
	}}

	resolved, err := ResolveRepository(cfg, "r")
	require.NoError(t, err)
	assert.Equal(t, 0, resolved.PushRetries)
}

func TestResolveRepositoryUnknownName(t *testing.T) {
	_, err := ResolveRepository(gitConfigFixture(), "nope")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryNotFound))
	// Hint material: the error lists configured names.
	assert.Contains(t, err.Error(), "flux-deploy")
	assert.Contains(t, err.Error(), "minimal")
}

func TestResolveRepositoryNilConfig(t *testing.T) {
	_, err := ResolveRepository(nil, "anything")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryNotFound))
}

func TestConfiguredRepositoryNamesSorted(t *testing.T) {
	names := ConfiguredRepositoryNames(gitConfigFixture())
	assert.Equal(t, []string{"flux-deploy", "minimal"}, names)
	assert.Nil(t, ConfiguredRepositoryNames(nil))
}

func TestValidateRepoRelativePath(t *testing.T) {
	worktree := t.TempDir()

	tests := []struct {
		name    string
		rel     string
		wantErr bool
	}{
		{name: "simple", rel: "clusters/prod/app", wantErr: false},
		{name: "dot", rel: ".", wantErr: false},
		{name: "internal dotdot stays inside", rel: "a/../b", wantErr: false},
		{name: "escape", rel: "../outside", wantErr: true},
		{name: "deep escape", rel: "a/../../outside", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			abs, err := ValidateRepoRelativePath(worktree, tt.rel)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrGitPathEscapesWorktree))
				return
			}
			require.NoError(t, err)
			assert.True(t, filepath.IsAbs(abs))
		})
	}
}

type fakeIdentityEnv struct {
	env map[string]string
	err error
}

func (f *fakeIdentityEnv) EnsureIdentityEnvironment(_ context.Context, _ string) (map[string]string, error) {
	return f.env, f.err
}

func TestComposeEnvironmentMergesIdentityEnv(t *testing.T) {
	base := []string{"PATH=/usr/bin", "GIT_CONFIG_COUNT=0", "HOME=/home/u"}
	provider := &fakeIdentityEnv{env: map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "include.path",
		"GIT_CONFIG_VALUE_0": "/tmp/git.config",
	}}

	merged, err := ComposeEnvironment(context.Background(), base, "platform-admin", provider)
	require.NoError(t, err)

	// Identity env overrides base keys and appends new ones.
	assert.Contains(t, merged, "GIT_CONFIG_COUNT=1")
	assert.NotContains(t, merged, "GIT_CONFIG_COUNT=0")
	assert.Contains(t, merged, "GIT_CONFIG_KEY_0=include.path")
	assert.Contains(t, merged, "GIT_CONFIG_VALUE_0=/tmp/git.config")
	// Base entries without overrides are preserved.
	assert.Contains(t, merged, "PATH=/usr/bin")
	assert.Contains(t, merged, "HOME=/home/u")
}

func TestComposeEnvironmentNoIdentityPassthrough(t *testing.T) {
	base := []string{"PATH=/usr/bin"}

	merged, err := ComposeEnvironment(context.Background(), base, "", &fakeIdentityEnv{})
	require.NoError(t, err)
	assert.Equal(t, base, merged)

	merged, err = ComposeEnvironment(context.Background(), base, "id", nil)
	require.NoError(t, err)
	assert.Equal(t, base, merged)
}

func TestComposeEnvironmentPropagatesError(t *testing.T) {
	provider := &fakeIdentityEnv{err: errors.New("boom")}

	_, err := ComposeEnvironment(context.Background(), []string{}, "id", provider)
	require.Error(t, err)
}
