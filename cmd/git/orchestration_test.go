// Package git contains tests for the orchestration helpers in the git command group.
// These tests cover error paths, argument routing, and the CommandProvider interface
// without requiring a real Git subprocess.
package git

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ---- CommandProvider interface tests ----

func TestGitCommandProvider_GetName(t *testing.T) {
	p := &GitCommandProvider{}
	assert.Equal(t, "git", p.GetName())
}

func TestGitCommandProvider_GetGroup(t *testing.T) {
	p := &GitCommandProvider{}
	assert.Equal(t, "GitOps", p.GetGroup())
}

func TestGitCommandProvider_GetCommand(t *testing.T) {
	p := &GitCommandProvider{}
	cmd := p.GetCommand()
	require.NotNil(t, cmd)
	assert.Equal(t, "git", cmd.Name())
}

func TestGitCommandProvider_GetFlagsBuilder(t *testing.T) {
	p := &GitCommandProvider{}
	assert.Nil(t, p.GetFlagsBuilder())
}

func TestGitCommandProvider_GetPositionalArgsBuilder(t *testing.T) {
	p := &GitCommandProvider{}
	assert.Nil(t, p.GetPositionalArgsBuilder())
}

func TestGitCommandProvider_GetCompatibilityFlags(t *testing.T) {
	p := &GitCommandProvider{}
	assert.Nil(t, p.GetCompatibilityFlags())
}

func TestGitCommandProvider_GetAliases(t *testing.T) {
	p := &GitCommandProvider{}
	// GetAliases returns the "atmos list git-repositories" alias added by the list subcommand.
	aliases := p.GetAliases()
	require.NotEmpty(t, aliases)
}

func TestGitCommandProvider_IsExperimental(t *testing.T) {
	p := &GitCommandProvider{}
	assert.True(t, p.IsExperimental())
}

func TestSetAtmosConfig(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })

	cfg := &schema.AtmosConfiguration{}
	SetAtmosConfig(cfg)
	assert.Equal(t, cfg, atmosConfigPtr)
}

// ---- runClone error paths ----

func TestRunClone_NoArgNilConfig(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = nil

	opts := &cloneOptions{All: false}
	err := runClone(context.Background(), opts, []string{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryRequired))
}

func TestRunClone_PathArgReturnsNotFoundError(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	// A plain path that doesn't look like a URI → argKindPath → not-found error.
	opts := &cloneOptions{}
	err := runClone(context.Background(), opts, []string{"./some/local/path"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryNotFound))
}

func TestRunClone_AllWithEmptyConfig(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	opts := &cloneOptions{All: true}
	// No repos configured → no error, no work done.
	err := runClone(context.Background(), opts, []string{})
	assert.NoError(t, err)
}

// ---- runStatus error paths ----

func TestRunStatus_NoArg(t *testing.T) {
	err := runStatus(context.Background(), false, []string{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryRequired))
}

func TestRunStatus_AllNoRepositories(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	// No repos configured → early return, no error.
	err := runStatus(context.Background(), true, []string{})
	assert.NoError(t, err)
}

func TestRunStatus_NilConfig(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = nil

	err := runStatus(context.Background(), true, []string{})
	assert.NoError(t, err)
}

// ---- runPull error paths ----

func TestRunPull_NoArg(t *testing.T) {
	err := runPull(context.Background(), false, "", "", []string{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryRequired))
}

func TestRunPull_AllNoRepositories(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	err := runPull(context.Background(), true, "", "", []string{})
	assert.NoError(t, err)
}

func TestRunPull_AllNilConfig(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = nil

	err := runPull(context.Background(), true, "", "", []string{})
	assert.NoError(t, err)
}

// ---- runDiff error paths ----

func TestRunDiff_URIArg(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	err := runDiff(context.Background(), "https://github.com/acme/repo.git", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryRequired))
}

// ---- runPush error paths ----

func TestRunPush_URIArg(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	err := runPush(context.Background(), "https://github.com/acme/repo.git", "", "", false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryRequired))
}

func TestRunPush_DryRunPath(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	// A plain path with dry-run → no provider needed, should succeed.
	err := runPush(context.Background(), "/tmp/somerepo", "main", "origin", true)
	assert.NoError(t, err)
}

// ---- runCommit error paths ----

func TestRunCommit_EmptyMessage(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	opts := &commitOptions{Message: "", DryRun: false}
	err := runCommit(context.Background(), "/tmp/repo", opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrRequiredFlagNotProvided))
}

func TestRunCommit_EmptyMessageWithDryRunAllowed(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	// dry-run with empty message is allowed: it just shows status without committing.
	// The path hits composeEnv → atmosgit.ComposeEnvironment which calls os.Environ()
	// (always succeeds with empty identity) then NewProvider() which will fail if
	// cli provider not registered. We test the validation path only.
	opts := &commitOptions{Message: "", DryRun: true}
	// At minimum, no ErrRequiredFlagNotProvided should be returned.
	// The function may fail later at provider resolution, which is acceptable.
	err := runCommit(context.Background(), "/tmp/repo", opts)
	if err != nil {
		assert.False(t, errors.Is(err, errUtils.ErrRequiredFlagNotProvided))
	}
}

// ---- parseCloneFlags ----

func TestParseCloneFlags_Defaults(t *testing.T) {
	// Use a fresh Viper instance so defaults are zero values.
	v := viper.New()
	opts := parseCloneFlags(v)

	assert.Equal(t, "", opts.RepoURI)
	assert.Equal(t, "", opts.Branch)
	assert.Equal(t, "", opts.Remote)
	assert.Equal(t, "", opts.Workdir)
	assert.Equal(t, "", opts.Filter)
	assert.Equal(t, 0, opts.Depth)
	assert.False(t, opts.SingleBranch)
	assert.False(t, opts.Submodules)
	assert.False(t, opts.All)
}

// ---- parseCommitFlags ----

func TestParseCommitFlags_Defaults(t *testing.T) {
	// Use a fresh Viper instance so defaults are zero values.
	v := viper.New()
	opts := parseCommitFlags(v)

	assert.Equal(t, "", opts.Message)
	assert.Empty(t, opts.Paths)
	assert.False(t, opts.Sign)
	assert.False(t, opts.NoSign)
	assert.False(t, opts.DryRun)
}

// ---- resolveWorkdir ----

func TestResolveWorkdir_FlagWins(t *testing.T) {
	assert.Equal(t, "/custom", resolveWorkdir("/custom", "/default"))
}

func TestResolveWorkdir_FallsToRepo(t *testing.T) {
	assert.Equal(t, "/default", resolveWorkdir("", "/default"))
}

func TestResolveWorkdir_BothEmpty(t *testing.T) {
	assert.Equal(t, "", resolveWorkdir("", ""))
}

// ---- executeStatusAndPrint with stub ----

func TestExecuteStatusAndPrint_Clean(t *testing.T) {
	stub := &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return &atmosgit.StatusResult{Clean: true}, nil
		},
	}
	exec := newExecutorWithProvider(stub)
	err := executeStatusAndPrint(context.Background(), exec, "/tmp/repo", nil)
	assert.NoError(t, err)
}

func TestExecuteStatusAndPrint_Dirty(t *testing.T) {
	stub := &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return &atmosgit.StatusResult{
				Clean: false,
				Entries: []atmosgit.StatusEntry{
					{Code: "M ", Path: "changed.go"},
				},
			}, nil
		},
	}
	exec := newExecutorWithProvider(stub)
	err := executeStatusAndPrint(context.Background(), exec, "/tmp/repo", nil)
	assert.NoError(t, err)
}

func TestExecuteStatusAndPrint_Error(t *testing.T) {
	expected := errors.New("status exploded")
	stub := &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return nil, expected
		},
	}
	exec := newExecutorWithProvider(stub)
	err := executeStatusAndPrint(context.Background(), exec, "/tmp/repo", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, expected))
}

// ---- executeDiffAndPrint with stub ----

func TestExecuteDiffAndPrint_NoChanges(t *testing.T) {
	stub := &stubGitProvider{
		diffFn: func(_ context.Context, _ *atmosgit.DiffOptions) (*atmosgit.DiffResult, error) {
			return &atmosgit.DiffResult{HasChanges: false}, nil
		},
	}
	exec := newExecutorWithProvider(stub)
	err := executeDiffAndPrint(context.Background(), exec, "/tmp/repo", nil, nil)
	assert.NoError(t, err)
}

func TestExecuteDiffAndPrint_WithDiff(t *testing.T) {
	stub := &stubGitProvider{
		diffFn: func(_ context.Context, opts *atmosgit.DiffOptions) (*atmosgit.DiffResult, error) {
			return &atmosgit.DiffResult{
				HasChanges: true,
				Output:     "diff --git a/foo.go b/foo.go\n",
				Untracked:  []string{"new.go"},
			}, nil
		},
	}
	exec := newExecutorWithProvider(stub)
	err := executeDiffAndPrint(context.Background(), exec, "/tmp/repo", nil, []string{"foo.go"})
	assert.NoError(t, err)
}

func TestExecuteDiffAndPrint_Error(t *testing.T) {
	expected := errors.New("diff failed")
	stub := &stubGitProvider{
		diffFn: func(_ context.Context, _ *atmosgit.DiffOptions) (*atmosgit.DiffResult, error) {
			return nil, expected
		},
	}
	exec := newExecutorWithProvider(stub)
	err := executeDiffAndPrint(context.Background(), exec, "/tmp/repo", nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, expected))
}

// ---- printStatus edge cases ----

func TestPrintStatus_Entries(t *testing.T) {
	result := &atmosgit.StatusResult{
		Clean: false,
		Entries: []atmosgit.StatusEntry{
			{Code: " M", Path: "modified.go"},
			{Code: "??", Path: "untracked.go"},
		},
	}
	err := printStatus("/tmp/repo", result)
	assert.NoError(t, err)
}

// ---- printDiff edge cases ----

func TestPrintDiff_WithUntrackedOnly(t *testing.T) {
	result := &atmosgit.DiffResult{
		HasChanges: true,
		Output:     "",
		Untracked:  []string{"new_file.go", "another.go"},
	}
	err := printDiff("/tmp/repo", result)
	assert.NoError(t, err)
}

func TestPrintDiff_WithDiffAndUntracked(t *testing.T) {
	result := &atmosgit.DiffResult{
		HasChanges: true,
		Output:     "--- a/foo\n+++ b/foo\n",
		Untracked:  []string{"extra.go"},
	}
	err := printDiff("/tmp/repo", result)
	assert.NoError(t, err)
}

// ---- runCICheckout error path ----

func TestRunCICheckout_EmptyRepository(t *testing.T) {
	opts := &cloneOptions{Workdir: "/tmp/ci"}
	err := runCICheckout(context.Background(), "github", "", "main", opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryRequired))
}

// ---- checkCIEnabled ----

func TestCheckCIEnabled_NilConfig(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = nil

	err := checkCIEnabled()
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryRequired))
}

func TestCheckCIEnabled_Disabled(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: false}}

	err := checkCIEnabled()
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryRequired))
}

func TestCheckCIEnabled_Enabled(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}

	err := checkCIEnabled()
	assert.NoError(t, err)
}

// ---- runCloneAll with empty repositories ----

func TestRunCloneAll_NoRepositories(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	opts := &cloneOptions{}
	err := runCloneAll(context.Background(), opts)
	assert.NoError(t, err)
}

func TestRunCloneAll_NilConfig(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = nil

	opts := &cloneOptions{}
	err := runCloneAll(context.Background(), opts)
	assert.NoError(t, err)
}

// ---- runPullAll with empty repositories ----

func TestRunPullAll_NoRepositories(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	err := runPullAll(context.Background(), "", "")
	assert.NoError(t, err)
}

func TestRunPullAll_NilConfig(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = nil

	err := runPullAll(context.Background(), "", "")
	assert.NoError(t, err)
}

// ---- resolveRepoByName error path ----

func TestResolveRepoByName_NotFound(t *testing.T) {
	cfg := &schema.GitConfig{
		Repositories: map[string]schema.GitRepository{
			"existing": {URI: "https://github.com/acme/repo.git"},
		},
	}
	_, err := resolveRepoByName("missing", cfg)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryNotFound))
}

func TestResolveRepoByName_Found(t *testing.T) {
	cfg := &schema.GitConfig{
		Repositories: map[string]schema.GitRepository{
			"my-repo": {URI: "https://github.com/acme/repo.git"},
		},
	}
	resolved, err := resolveRepoByName("my-repo", cfg)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, "my-repo", resolved.Name)
}

// ---- composeEnv with empty identity ----

func TestComposeEnv_EmptyIdentity(t *testing.T) {
	// With no identity, ComposeEnvironment returns the base env unchanged.
	env, err := composeEnv(context.Background(), "")
	require.NoError(t, err)
	// Should at least contain PATH.
	assert.NotEmpty(t, env)
}
