// Package git contains additional tests targeting remaining coverage gaps.
package git

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ---- runCommit: argKindName path ----

func TestRunCommit_ConfiguredName_Committed(t *testing.T) {
	dir := t.TempDir()
	withTestProvider(t, &stubGitProvider{
		commitFn: func(_ context.Context, _ *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			return &atmosgit.CommitResult{Committed: true, SHA: "deadbeef"}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"my-repo": {
					URI:     "https://github.com/acme/repo.git",
					Workdir: dir,
				},
			},
		},
	}

	opts := &commitOptions{Message: "feat: commit via name", DryRun: false}
	err := runCommit(context.Background(), "my-repo", opts)
	assert.NoError(t, err)
}

func TestRunCommit_DryRun_ConfiguredName(t *testing.T) {
	dir := t.TempDir()
	withTestProvider(t, &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return &atmosgit.StatusResult{Clean: false, Entries: []atmosgit.StatusEntry{
				{Code: "M ", Path: "README.md"},
			}}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"my-repo": {
					URI:     "https://github.com/acme/repo.git",
					Workdir: dir,
				},
			},
		},
	}

	opts := &commitOptions{Message: "", DryRun: true}
	err := runCommit(context.Background(), "my-repo", opts)
	assert.NoError(t, err)
}

// ---- runPush: argKindName path ----

func TestRunPush_ConfiguredName_Success(t *testing.T) {
	dir := t.TempDir()
	var capturedOpts *atmosgit.PushOptions
	withTestProvider(t, &stubGitProvider{
		pushFn: func(_ context.Context, opts *atmosgit.PushOptions) error {
			capturedOpts = opts
			return nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"my-repo": {
					URI:     "https://github.com/acme/repo.git",
					Workdir: dir,
				},
			},
		},
	}

	err := runPush(context.Background(), "my-repo", &pushOptions{})
	require.NoError(t, err)
	require.NotNil(t, capturedOpts)
	assert.Equal(t, dir, capturedOpts.Workdir)
}

func TestRunPush_ConfiguredName_DryRun(t *testing.T) {
	dir := t.TempDir()
	withTestProvider(t, &stubGitProvider{})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"my-repo": {
					URI:     "https://github.com/acme/repo.git",
					Workdir: dir,
				},
			},
		},
	}

	// Dry-run with named repo: no provider call needed.
	err := runPush(context.Background(), "my-repo", &pushOptions{Branch: "main", Remote: "origin", DryRun: true})
	assert.NoError(t, err)
}

func TestRunPush_PathArg_NonDryRun(t *testing.T) {
	withTestProvider(t, &stubGitProvider{
		pushFn: func(_ context.Context, _ *atmosgit.PushOptions) error { return nil },
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	// A path that's not a configured name is treated as argKindPath.
	// With test provider override it succeeds.
	err := runPush(context.Background(), t.TempDir(), &pushOptions{Branch: "main", Remote: "origin"})
	assert.NoError(t, err)
}

// ---- runClone: path argument (should error) ----

func TestRunClone_AbsPathArg_ErrorsAsNotFound(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	opts := &cloneOptions{}
	// An absolute path arg → argKindPath → not a valid clone target.
	err := runClone(context.Background(), opts, []string{t.TempDir()})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryNotFound))
}

// ---- runPullOne: branch/remote flag precedence ----

func TestRunPullOne_FlagBranchBeatsResolved(t *testing.T) {
	dir := t.TempDir()
	markGitWorktree(t, dir)
	var capturedBranch string
	withTestProvider(t, &stubGitProvider{
		pullFn: func(_ context.Context, opts *atmosgit.PullOptions) error {
			capturedBranch = opts.Branch
			return nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"my-repo": {
					URI:     "https://github.com/acme/repo.git",
					Workdir: dir,
				},
			},
		},
	}

	// Flag branch "feature-x" should beat the repo's default branch.
	err := runPullOne(context.Background(), "my-repo", &pullOptions{Branch: "feature-x"})
	require.NoError(t, err)
	assert.Equal(t, "feature-x", capturedBranch)
}

// ---- runCloneNamed: flag workdir beats resolved workdir ----

func TestRunCloneNamed_WorkdirFlagOverride(t *testing.T) {
	customWorkdir := t.TempDir()
	var capturedWorkdir string
	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, opts *atmosgit.CloneOptions) error {
			capturedWorkdir = opts.Workdir
			return nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"my-repo": {
					URI:     "https://github.com/acme/repo.git",
					Workdir: t.TempDir(), // Different from customWorkdir.
				},
			},
		},
	}

	opts := &cloneOptions{Workdir: customWorkdir}
	err := runCloneNamed(context.Background(), "my-repo", opts)
	require.NoError(t, err)
	assert.Equal(t, customWorkdir, capturedWorkdir)
}

// ---- runCloneURI: unknown query param returns error ----

func TestRunCloneURI_UnknownQueryParam_Error(t *testing.T) {
	withTestProvider(t, &stubGitProvider{})

	opts := &cloneOptions{Workdir: t.TempDir()}
	// Unknown query param "foo=bar" should cause ParseCloneURI to error.
	err := runCloneURI(context.Background(), "git::https://github.com/acme/repo.git?foo=bar", opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown query parameter")
}

// ---- runStatusOne: status error propagated ----

func TestRunStatusOne_ProviderError(t *testing.T) {
	statusErr := errors.New("not a git repository")
	withTestProvider(t, &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return nil, statusErr
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	err := runStatusOne(context.Background(), t.TempDir())
	require.Error(t, err)
	assert.True(t, errors.Is(err, statusErr))
}

// ---- runCommitDryRun: status error propagated ----

func TestRunCommitDryRun_StatusError(t *testing.T) {
	statusErr := errors.New("lock file exists")
	withTestProvider(t, &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return nil, statusErr
		},
	})

	opts := &commitOptions{DryRun: true}
	err := runCommitDryRun(context.Background(), t.TempDir(), "", opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, statusErr))
}

// ---- executeCommit: identity path ----

func TestExecuteCommit_WithIdentity(t *testing.T) {
	var capturedMsg string
	withTestProvider(t, &stubGitProvider{
		commitFn: func(_ context.Context, opts *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			capturedMsg = opts.Message
			return &atmosgit.CommitResult{Committed: true, SHA: "abc"}, nil
		},
	})

	opts := &commitOptions{Message: "fix: with identity", DryRun: false}
	err := executeCommit(context.Background(), t.TempDir(), "", nil, opts)
	require.NoError(t, err)
	assert.Equal(t, "fix: with identity", capturedMsg)
}

// ---- runCloneAll: errors.Join aggregation ----

func TestRunCloneAll_PartialFailure(t *testing.T) {
	failErr := errors.New("authentication required")
	var callCount atomic.Int32
	privateDest := t.TempDir()
	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, opts *atmosgit.CloneOptions) error {
			callCount.Add(1)
			// Fail for the private repo by checking the URI.
			if opts.URI == "https://github.com/acme/private.git" {
				return failErr
			}
			return nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"public":  {URI: "https://github.com/acme/public.git", Workdir: t.TempDir()},
				"private": {URI: "https://github.com/acme/private.git", Workdir: privateDest},
			},
		},
	}

	opts := &cloneOptions{}
	err := runCloneAll(context.Background(), opts)

	// Both repos were attempted.
	assert.Equal(t, int32(2), callCount.Load(), "both repos should be attempted")

	// Error is returned (aggregated via errors.Join).
	require.Error(t, err)
	assert.True(t, errors.Is(err, failErr))
}
