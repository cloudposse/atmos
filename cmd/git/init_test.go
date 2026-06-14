package git

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setInitTestConfig installs an Atmos config with the given repositories and
// restores the previous config when the test ends.
func setInitTestConfig(t *testing.T, repos map[string]schema.GitRepository) {
	t.Helper()
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{Repositories: repos},
	}
}

func TestRunInit_KeepHistoryRequiresFrom(t *testing.T) {
	err := runInit(context.Background(), &initOptions{KeepHistory: true}, []string{"docs"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidFlag))
}

func TestRunInit_UnknownRepository(t *testing.T) {
	setInitTestConfig(t, map[string]schema.GitRepository{
		"docs": {URI: "https://github.com/acme/docs.git"},
	})

	err := runInit(context.Background(), &initOptions{}, []string{"nope"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryNotFound))
}

func TestRunInit_RequiresURI(t *testing.T) {
	setInitTestConfig(t, map[string]schema.GitRepository{
		"docs": {Workdir: t.TempDir()},
	})

	err := runInit(context.Background(), &initOptions{}, []string{"docs"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
}

func TestRunInit_NoArgRequiresSingleConfiguredRepo(t *testing.T) {
	setInitTestConfig(t, map[string]schema.GitRepository{
		"a": {URI: "https://example.com/a.git"},
		"b": {URI: "https://example.com/b.git"},
	})

	err := runInit(context.Background(), &initOptions{}, []string{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryRequired))
}

func TestRunInit_ForwardsResolvedOptionsToProvider(t *testing.T) {
	workdir := t.TempDir()
	setInitTestConfig(t, map[string]schema.GitRepository{
		"docs": {
			URI:     "https://github.com/acme/docs.git",
			Branch:  "main",
			Workdir: workdir,
		},
	})

	var got *atmosgit.InitOptions
	withTestProvider(t, &stubGitProvider{
		initFn: func(_ context.Context, opts *atmosgit.InitOptions) error {
			got = opts
			return nil
		},
	})

	opts := &initOptions{
		From:        "https://github.com/acme/template.git",
		KeepHistory: true,
		ExtraArgs:   []string{"--no-tags"},
	}
	// No positional arg: the single configured repository is used.
	require.NoError(t, runInit(context.Background(), opts, []string{}))

	require.NotNil(t, got)
	assert.Equal(t, "https://github.com/acme/docs.git", got.URI)
	assert.Equal(t, "https://github.com/acme/template.git", got.FromURI)
	assert.True(t, got.KeepHistory)
	assert.Equal(t, workdir, got.Workdir)
	assert.Equal(t, "main", got.Branch)
	assert.Equal(t, []string{"--no-tags"}, got.ExtraArgs)
}

func TestRunInit_FlagsOverrideConfig(t *testing.T) {
	setInitTestConfig(t, map[string]schema.GitRepository{
		"docs": {
			URI:     "https://github.com/acme/docs.git",
			Branch:  "main",
			Workdir: t.TempDir(),
		},
	})

	var got *atmosgit.InitOptions
	withTestProvider(t, &stubGitProvider{
		initFn: func(_ context.Context, opts *atmosgit.InitOptions) error {
			got = opts
			return nil
		},
	})

	override := t.TempDir()
	opts := &initOptions{Branch: "develop", Workdir: override}
	require.NoError(t, runInit(context.Background(), opts, []string{"docs"}))

	require.NotNil(t, got)
	assert.Equal(t, "develop", got.Branch)
	assert.Equal(t, override, got.Workdir)
}

func TestRunInit_DryRunSkipsProvider(t *testing.T) {
	setInitTestConfig(t, map[string]schema.GitRepository{
		"docs": {URI: "https://github.com/acme/docs.git", Workdir: t.TempDir()},
	})

	called := false
	withTestProvider(t, &stubGitProvider{
		initFn: func(_ context.Context, _ *atmosgit.InitOptions) error {
			called = true
			return nil
		},
	})

	require.NoError(t, runInit(context.Background(), &initOptions{DryRun: true}, []string{"docs"}))
	assert.False(t, called, "dry-run must not invoke the provider")
}

func TestInitCmdForwardsNativeArgs(t *testing.T) {
	setInitTestConfig(t, map[string]schema.GitRepository{
		"docs": {URI: "https://github.com/acme/docs.git", Workdir: t.TempDir()},
	})

	var got []string
	withTestProvider(t, &stubGitProvider{
		initFn: func(_ context.Context, opts *atmosgit.InitOptions) error {
			got = opts.ExtraArgs
			return nil
		},
	})

	require.NoError(t, initCmd.ParseFlags([]string{"docs", "--", "--template", "/tmp/tpl"}))
	require.NoError(t, initCmd.RunE(initCmd, initCmd.Flags().Args()))

	assert.Equal(t, []string{"--template", "/tmp/tpl"}, got)
}
