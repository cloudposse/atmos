// Package git contains tests using the test-provider override seam to exercise
// orchestration logic without invoking real Git subprocesses.
package git

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/schema"
)

// withTestProvider installs stub as the test provider.
// The provider is restored automatically when the test ends via t.Cleanup.
func withTestProvider(t *testing.T, stub *stubGitProvider) {
	t.Helper()
	cleanup := setTestProvider(stub)
	t.Cleanup(cleanup)
}

// ---- runStatusOne with stub ----

func TestRunStatusOne_Path_Clean(t *testing.T) {
	withTestProvider(t, &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return &atmosgit.StatusResult{Clean: true}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	err := runStatusOne(context.Background(), t.TempDir())
	assert.NoError(t, err)
}

func TestRunStatusOne_Path_Dirty(t *testing.T) {
	withTestProvider(t, &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return &atmosgit.StatusResult{
				Clean: false,
				Entries: []atmosgit.StatusEntry{
					{Code: "M ", Path: "foo.go"},
				},
			}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	err := runStatusOne(context.Background(), t.TempDir())
	assert.NoError(t, err)
}

func TestRunStatusOne_ConfiguredNameNotFound(t *testing.T) {
	// When a configured repo has no workdir set, it defaults to XDG cache.
	// Here we test that a configured name resolves via the name path.
	withTestProvider(t, &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return &atmosgit.StatusResult{Clean: true}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"configured": {URI: "https://github.com/acme/configured.git"},
			},
		},
	}

	// "configured" resolves to the name path (XDG workdir) and stub returns clean.
	err := runStatusOne(context.Background(), "configured")
	assert.NoError(t, err)
}

func TestRunStatusNoArg_UsesSingleConfiguredRepository(t *testing.T) {
	var capturedWorkdir string
	withTestProvider(t, &stubGitProvider{
		statusFn: func(_ context.Context, opts *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			capturedWorkdir = opts.Workdir
			return &atmosgit.StatusResult{Clean: true}, nil
		},
	})

	dir := t.TempDir()
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"configured": {
					URI:     "https://github.com/acme/configured.git",
					Workdir: dir,
				},
			},
		},
	}

	err := runStatus(context.Background(), false, nil)
	require.NoError(t, err)
	assert.Equal(t, dir, capturedWorkdir)
}

func TestRunStatusAll_WithConfiguredRepos(t *testing.T) {
	callCount := 0
	withTestProvider(t, &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			callCount++
			return &atmosgit.StatusResult{Clean: true}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"repo-a": {URI: "https://github.com/acme/a.git"},
				"repo-b": {URI: "https://github.com/acme/b.git"},
			},
		},
	}

	err := runStatusAll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "both repos should have been queried")
}

// ---- runPullOne with stub ----

func TestRunPullOne_Path(t *testing.T) {
	dir := t.TempDir()
	var capturedWorkdir string
	withTestProvider(t, &stubGitProvider{
		pullFn: func(_ context.Context, opts *atmosgit.PullOptions) error {
			capturedWorkdir = opts.Workdir
			return nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	err := runPullOne(context.Background(), dir, "main", "origin")
	require.NoError(t, err)
	assert.Equal(t, dir, capturedWorkdir)
}

func TestRunPullOne_Path_Error(t *testing.T) {
	pullErr := errors.New("pull failed: fast-forward only")
	withTestProvider(t, &stubGitProvider{
		pullFn: func(_ context.Context, _ *atmosgit.PullOptions) error {
			return pullErr
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	err := runPullOne(context.Background(), t.TempDir(), "", "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, pullErr))
}

func TestRunPullOne_ConfiguredNameResolved(t *testing.T) {
	// A configured repository name resolves via the name path, stub succeeds.
	dir := t.TempDir()
	var capturedWorkdir string
	withTestProvider(t, &stubGitProvider{
		pullFn: func(_ context.Context, opts *atmosgit.PullOptions) error {
			capturedWorkdir = opts.Workdir
			return nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"configured": {
					URI:     "https://github.com/acme/configured.git",
					Workdir: dir,
				},
			},
		},
	}

	err := runPullOne(context.Background(), "configured", "", "")
	require.NoError(t, err)
	assert.Equal(t, dir, capturedWorkdir)
}

func TestRunPullNoArg_UsesSingleConfiguredRepository(t *testing.T) {
	var capturedWorkdir string
	withTestProvider(t, &stubGitProvider{
		pullFn: func(_ context.Context, opts *atmosgit.PullOptions) error {
			capturedWorkdir = opts.Workdir
			return nil
		},
	})

	dir := t.TempDir()
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"configured": {
					URI:     "https://github.com/acme/configured.git",
					Workdir: dir,
				},
			},
		},
	}

	err := runPull(context.Background(), false, "", "", nil)
	require.NoError(t, err)
	assert.Equal(t, dir, capturedWorkdir)
}

func TestRunPullAll_WithRepos(t *testing.T) {
	callCount := 0
	withTestProvider(t, &stubGitProvider{
		pullFn: func(_ context.Context, _ *atmosgit.PullOptions) error {
			callCount++
			return nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"r1": {URI: "https://github.com/acme/r1.git"},
				"r2": {URI: "https://github.com/acme/r2.git"},
				"r3": {URI: "https://github.com/acme/r3.git"},
			},
		},
	}

	err := runPullAll(context.Background(), "", "")
	require.NoError(t, err)
	assert.Equal(t, 3, callCount, "all repos should be pulled")
}

// ---- runDiff with stub (path case) ----

func TestRunDiff_Path_NoChanges(t *testing.T) {
	withTestProvider(t, &stubGitProvider{
		diffFn: func(_ context.Context, _ *atmosgit.DiffOptions) (*atmosgit.DiffResult, error) {
			return &atmosgit.DiffResult{HasChanges: false}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	err := runDiff(context.Background(), t.TempDir(), nil)
	assert.NoError(t, err)
}

func TestRunDiff_Path_WithChanges(t *testing.T) {
	var capturedPaths []string
	withTestProvider(t, &stubGitProvider{
		diffFn: func(_ context.Context, opts *atmosgit.DiffOptions) (*atmosgit.DiffResult, error) {
			capturedPaths = opts.Paths
			return &atmosgit.DiffResult{
				HasChanges: true,
				Output:     "--- a/foo.go\n+++ b/foo.go\n",
			}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	paths := []string{"foo.go", "bar.go"}
	err := runDiff(context.Background(), t.TempDir(), paths)
	require.NoError(t, err)
	assert.Equal(t, paths, capturedPaths)
}

func TestRunDiff_ConfiguredName_Success(t *testing.T) {
	// A configured name resolves via argKindName path.
	dir := t.TempDir()
	withTestProvider(t, &stubGitProvider{
		diffFn: func(_ context.Context, _ *atmosgit.DiffOptions) (*atmosgit.DiffResult, error) {
			return &atmosgit.DiffResult{HasChanges: false}, nil
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

	err := runDiff(context.Background(), "my-repo", nil)
	assert.NoError(t, err)
}

// ---- runPush with stub ----

func TestRunPush_Path_Success(t *testing.T) {
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
	atmosConfigPtr = &schema.AtmosConfiguration{}

	err := runPush(context.Background(), dir, "main", "origin", false)
	require.NoError(t, err)
	require.NotNil(t, capturedOpts)
	assert.Equal(t, dir, capturedOpts.Workdir)
	assert.Equal(t, "main", capturedOpts.Branch)
	assert.Equal(t, "origin", capturedOpts.Remote)
}

func TestRunPush_Path_Error(t *testing.T) {
	pushErr := errors.New("remote rejected")
	withTestProvider(t, &stubGitProvider{
		pushFn: func(_ context.Context, _ *atmosgit.PushOptions) error {
			return pushErr
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	err := runPush(context.Background(), t.TempDir(), "main", "origin", false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, pushErr))
}

// ---- runCommit with stub ----

func TestRunCommit_DryRun_Clean(t *testing.T) {
	withTestProvider(t, &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return &atmosgit.StatusResult{Clean: true}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	opts := &commitOptions{Message: "", DryRun: true}
	err := runCommit(context.Background(), t.TempDir(), opts)
	assert.NoError(t, err)
}

func TestRunCommit_DryRun_WithChanges(t *testing.T) {
	withTestProvider(t, &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return &atmosgit.StatusResult{
				Clean: false,
				Entries: []atmosgit.StatusEntry{
					{Code: " M", Path: "main.go"},
				},
			}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	opts := &commitOptions{Message: "feat: x", DryRun: true}
	err := runCommit(context.Background(), t.TempDir(), opts)
	assert.NoError(t, err)
}

func TestRunCommit_Actual_NothingToCommit(t *testing.T) {
	withTestProvider(t, &stubGitProvider{
		commitFn: func(_ context.Context, _ *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			return &atmosgit.CommitResult{Committed: false}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	opts := &commitOptions{Message: "feat: test", DryRun: false}
	err := runCommit(context.Background(), t.TempDir(), opts)
	assert.NoError(t, err)
}

func TestRunCommit_Actual_Committed(t *testing.T) {
	withTestProvider(t, &stubGitProvider{
		commitFn: func(_ context.Context, _ *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			return &atmosgit.CommitResult{Committed: true, SHA: "abc123"}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	opts := &commitOptions{Message: "feat: test", DryRun: false}
	err := runCommit(context.Background(), t.TempDir(), opts)
	assert.NoError(t, err)
}

func TestRunCommit_Actual_SigningAlways(t *testing.T) {
	var capturedSigning atmosgit.SigningMode
	withTestProvider(t, &stubGitProvider{
		commitFn: func(_ context.Context, opts *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			capturedSigning = opts.Signing
			return &atmosgit.CommitResult{Committed: true, SHA: "signed123"}, nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	opts := &commitOptions{Message: "chore: sign", Sign: true, DryRun: false}
	err := runCommit(context.Background(), t.TempDir(), opts)
	require.NoError(t, err)
	assert.Equal(t, atmosgit.SigningAlways, capturedSigning)
}

func TestRunCommit_Error(t *testing.T) {
	commitErr := errors.New("untracked files block commit")
	withTestProvider(t, &stubGitProvider{
		commitFn: func(_ context.Context, _ *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			return nil, commitErr
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	opts := &commitOptions{Message: "feat: x", DryRun: false}
	err := runCommit(context.Background(), t.TempDir(), opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, commitErr))
}

// ---- runCloneNamed with stub ----

func TestRunCloneNamed_Success(t *testing.T) {
	var clonedURI string
	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, opts *atmosgit.CloneOptions) error {
			clonedURI = opts.URI
			return nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"my-repo": {URI: "https://github.com/acme/my-repo.git"},
			},
		},
	}

	opts := &cloneOptions{}
	err := runCloneNamed(context.Background(), "my-repo", opts)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme/my-repo.git", clonedURI)
}

func TestRunCloneNoArg_ClonesSingleConfiguredRepository(t *testing.T) {
	var clonedURI string
	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, opts *atmosgit.CloneOptions) error {
			clonedURI = opts.URI
			return nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"deploy": {URI: "https://github.com/acme/deploy.git"},
			},
		},
	}

	err := runClone(context.Background(), &cloneOptions{}, nil)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme/deploy.git", clonedURI)
}

func TestRunCloneNamed_NotFound(t *testing.T) {
	withTestProvider(t, &stubGitProvider{})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{},
		},
	}

	opts := &cloneOptions{}
	err := runCloneNamed(context.Background(), "missing", opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryNotFound))
}

func TestRunCloneNamed_CloneError(t *testing.T) {
	cloneErr := errors.New("permission denied")
	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, _ *atmosgit.CloneOptions) error {
			return cloneErr
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"repo": {URI: "https://github.com/acme/repo.git"},
			},
		},
	}

	opts := &cloneOptions{}
	err := runCloneNamed(context.Background(), "repo", opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, cloneErr))
}

// ---- runCloneURI with stub ----

func TestRunCloneURI_HTTPS_Success(t *testing.T) {
	var clonedURI string
	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, opts *atmosgit.CloneOptions) error {
			clonedURI = opts.URI
			return nil
		},
	})

	opts := &cloneOptions{Workdir: t.TempDir()}
	err := runCloneURI(context.Background(), "https://github.com/acme/repo.git", opts)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme/repo.git", clonedURI)
}

func TestRunCloneURI_GetterPrefix_BranchFromQueryParam(t *testing.T) {
	var capturedBranch string
	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, opts *atmosgit.CloneOptions) error {
			capturedBranch = opts.Branch
			return nil
		},
	})

	opts := &cloneOptions{Workdir: t.TempDir()}
	err := runCloneURI(context.Background(), "git::https://github.com/acme/repo.git?ref=main", opts)
	require.NoError(t, err)
	assert.Equal(t, "main", capturedBranch)
}

func TestRunCloneURI_FlagBranchBeatsQueryParam(t *testing.T) {
	var capturedBranch string
	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, opts *atmosgit.CloneOptions) error {
			capturedBranch = opts.Branch
			return nil
		},
	})

	// Flag branch "feature-x" should win over query param "main".
	opts := &cloneOptions{Workdir: t.TempDir(), Branch: "feature-x"}
	err := runCloneURI(context.Background(), "git::https://github.com/acme/repo.git?ref=main", opts)
	require.NoError(t, err)
	assert.Equal(t, "feature-x", capturedBranch)
}

func TestRunCloneURI_CloneError(t *testing.T) {
	cloneErr := errors.New("remote unreachable")
	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, _ *atmosgit.CloneOptions) error {
			return cloneErr
		},
	})

	opts := &cloneOptions{Workdir: t.TempDir()}
	err := runCloneURI(context.Background(), "https://github.com/acme/repo.git", opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, cloneErr))
}

// ---- runCICheckout with stub ----

func TestRunCICheckout_WithStub(t *testing.T) {
	var clonedURI string
	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, opts *atmosgit.CloneOptions) error {
			clonedURI = opts.URI
			return nil
		},
	})

	opts := &cloneOptions{Workdir: t.TempDir()}
	ciCtx := &ci.Context{
		Repository: "acme/my-repo",
		Ref:        "refs/heads/main",
		CloneURL:   "https://github.com/acme/my-repo.git",
	}
	err := runCICheckout(context.Background(), "github", ciCtx, opts)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme/my-repo.git", clonedURI)
}

func TestRunCICheckout_ProviderSuppliedHost(t *testing.T) {
	// The clone URL comes verbatim from the provider Context — a GitHub
	// Enterprise (or any non-github.com) host flows through untouched.
	var clonedURI string
	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, opts *atmosgit.CloneOptions) error {
			clonedURI = opts.URI
			return nil
		},
	})

	opts := &cloneOptions{Workdir: t.TempDir()}
	ciCtx := &ci.Context{
		Repository: "acme/repo",
		CloneURL:   "https://ghe.acme.com/acme/repo.git",
	}
	err := runCICheckout(context.Background(), "github", ciCtx, opts)
	require.NoError(t, err)
	assert.Equal(t, "https://ghe.acme.com/acme/repo.git", clonedURI)
}

func TestRunCICheckout_BranchFromRef(t *testing.T) {
	var capturedBranch string
	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, opts *atmosgit.CloneOptions) error {
			capturedBranch = opts.Branch
			return nil
		},
	})

	opts := &cloneOptions{Workdir: t.TempDir()}
	ciCtx := &ci.Context{
		Repository: "acme/repo",
		Ref:        "feature/x",
		CloneURL:   "https://github.com/acme/repo.git",
	}
	err := runCICheckout(context.Background(), "github", ciCtx, opts)
	require.NoError(t, err)
	assert.Equal(t, "feature/x", capturedBranch)
}

// ---- runCloneAll with repos and stub ----

func TestRunCloneAll_WithRepos(t *testing.T) {
	clonedNames := make([]string, 0)
	collectCh := make(chan string, 10)

	withTestProvider(t, &stubGitProvider{
		cloneFn: func(_ context.Context, opts *atmosgit.CloneOptions) error {
			collectCh <- opts.URI
			return nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"r1": {URI: "https://github.com/acme/r1.git"},
				"r2": {URI: "https://github.com/acme/r2.git"},
			},
		},
	}

	opts := &cloneOptions{}
	err := runCloneAll(context.Background(), opts)
	require.NoError(t, err)

	close(collectCh)
	for uri := range collectCh {
		clonedNames = append(clonedNames, uri)
	}
	assert.Len(t, clonedNames, 2)
}

// ---- providerForName with override ----

func TestProviderForName_WithOverride(t *testing.T) {
	stub := &stubGitProvider{}
	cleanup := setTestProvider(stub)
	defer cleanup()

	exec, err := providerForName("")
	require.NoError(t, err)
	require.NotNil(t, exec)
}

func TestProviderForName_WithoutOverride_ReturnsRealProvider(t *testing.T) {
	// Ensure no override is active.
	prev := testProviderOverride
	testProviderOverride = nil
	t.Cleanup(func() { testProviderOverride = prev })

	// The CLI provider is registered by git.go's blank import.
	exec, err := providerForName("")
	require.NoError(t, err)
	require.NotNil(t, exec)
}
