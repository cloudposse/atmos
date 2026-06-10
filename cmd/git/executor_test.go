package git

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

// stubGitProvider is a test double that lets tests control each operation.
type stubGitProvider struct {
	cloneFn  func(ctx context.Context, opts *atmosgit.CloneOptions) error
	pullFn   func(ctx context.Context, opts *atmosgit.PullOptions) error
	statusFn func(ctx context.Context, opts *atmosgit.StatusOptions) (*atmosgit.StatusResult, error)
	diffFn   func(ctx context.Context, opts *atmosgit.DiffOptions) (*atmosgit.DiffResult, error)
	commitFn func(ctx context.Context, opts *atmosgit.CommitOptions) (*atmosgit.CommitResult, error)
	pushFn   func(ctx context.Context, opts *atmosgit.PushOptions) error
}

func (s *stubGitProvider) Clone(ctx context.Context, opts *atmosgit.CloneOptions) error {
	if s.cloneFn != nil {
		return s.cloneFn(ctx, opts)
	}
	return nil
}

func (s *stubGitProvider) Pull(ctx context.Context, opts *atmosgit.PullOptions) error {
	if s.pullFn != nil {
		return s.pullFn(ctx, opts)
	}
	return nil
}

func (s *stubGitProvider) Status(ctx context.Context, opts *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
	if s.statusFn != nil {
		return s.statusFn(ctx, opts)
	}
	return &atmosgit.StatusResult{Clean: true}, nil
}

func (s *stubGitProvider) Diff(ctx context.Context, opts *atmosgit.DiffOptions) (*atmosgit.DiffResult, error) {
	if s.diffFn != nil {
		return s.diffFn(ctx, opts)
	}
	return &atmosgit.DiffResult{}, nil
}

func (s *stubGitProvider) Commit(ctx context.Context, opts *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
	if s.commitFn != nil {
		return s.commitFn(ctx, opts)
	}
	return &atmosgit.CommitResult{Committed: false}, nil
}

func (s *stubGitProvider) Push(ctx context.Context, opts *atmosgit.PushOptions) error {
	if s.pushFn != nil {
		return s.pushFn(ctx, opts)
	}
	return nil
}

// ---- Executor.Status tests ----

func TestExecutor_Status_Clean(t *testing.T) {
	stub := &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return &atmosgit.StatusResult{Clean: true, Entries: nil}, nil
		},
	}
	exec := newExecutorWithProvider(stub)

	result, err := execStatusForTest(context.Background(), exec, "/tmp/repo", nil)
	require.NoError(t, err)
	assert.True(t, result.Clean)
	assert.Empty(t, result.Entries)
}

func TestExecutor_Status_Dirty(t *testing.T) {
	stub := &stubGitProvider{
		statusFn: func(_ context.Context, opts *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return &atmosgit.StatusResult{
				Clean: false,
				Entries: []atmosgit.StatusEntry{
					{Code: " M", Path: "main.go"},
					{Code: "??", Path: "new_file.go"},
				},
			}, nil
		},
	}
	exec := newExecutorWithProvider(stub)

	result, err := execStatusForTest(context.Background(), exec, "/tmp/repo", nil)
	require.NoError(t, err)
	assert.False(t, result.Clean)
	require.Len(t, result.Entries, 2)
	assert.Equal(t, " M", result.Entries[0].Code)
	assert.Equal(t, "main.go", result.Entries[0].Path)
	assert.Equal(t, "??", result.Entries[1].Code)
}

func TestExecutor_Status_Error(t *testing.T) {
	providerErr := errors.New("git not found")
	stub := &stubGitProvider{
		statusFn: func(_ context.Context, _ *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
			return nil, providerErr
		},
	}
	exec := newExecutorWithProvider(stub)

	_, err := execStatusForTest(context.Background(), exec, "/tmp/repo", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, providerErr))
}

// ---- Executor.Pull tests ----

func TestExecutor_Pull_Success(t *testing.T) {
	var capturedOpts *atmosgit.PullOptions
	stub := &stubGitProvider{
		pullFn: func(_ context.Context, opts *atmosgit.PullOptions) error {
			capturedOpts = opts
			return nil
		},
	}
	exec := newExecutorWithProvider(stub)

	err := exec.Pull(context.Background(), &atmosgit.PullOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: "/tmp/repo",
			Remote:  "origin",
			Branch:  "main",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, capturedOpts)
	assert.Equal(t, "/tmp/repo", capturedOpts.Workdir)
	assert.Equal(t, "origin", capturedOpts.Remote)
	assert.Equal(t, "main", capturedOpts.Branch)
}

func TestExecutor_Pull_Error(t *testing.T) {
	providerErr := errors.New("pull failed")
	stub := &stubGitProvider{
		pullFn: func(_ context.Context, _ *atmosgit.PullOptions) error {
			return providerErr
		},
	}
	exec := newExecutorWithProvider(stub)

	err := exec.Pull(context.Background(), &atmosgit.PullOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/tmp/repo"},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, providerErr))
}

// ---- Executor.Commit tests ----

func TestExecutor_Commit_NothingToCommit(t *testing.T) {
	stub := &stubGitProvider{
		commitFn: func(_ context.Context, _ *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			return &atmosgit.CommitResult{Committed: false}, nil
		},
	}
	exec := newExecutorWithProvider(stub)

	result, err := exec.Commit(context.Background(), &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/tmp/repo"},
		Message:     "test",
	})
	require.NoError(t, err)
	assert.False(t, result.Committed)
}

func TestExecutor_Commit_Created(t *testing.T) {
	stub := &stubGitProvider{
		commitFn: func(_ context.Context, opts *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			return &atmosgit.CommitResult{Committed: true, SHA: "abc123"}, nil
		},
	}
	exec := newExecutorWithProvider(stub)

	result, err := exec.Commit(context.Background(), &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/tmp/repo"},
		Message:     "feat: add thing",
	})
	require.NoError(t, err)
	assert.True(t, result.Committed)
	assert.Equal(t, "abc123", result.SHA)
}

// ---- Executor.Push tests ----

func TestExecutor_Push_Success(t *testing.T) {
	var capturedOpts *atmosgit.PushOptions
	stub := &stubGitProvider{
		pushFn: func(_ context.Context, opts *atmosgit.PushOptions) error {
			capturedOpts = opts
			return nil
		},
	}
	exec := newExecutorWithProvider(stub)

	err := exec.Push(context.Background(), &atmosgit.PushOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: "/tmp/repo",
			Remote:  "origin",
			Branch:  "main",
		},
		Retries: 3,
	})
	require.NoError(t, err)
	require.NotNil(t, capturedOpts)
	assert.Equal(t, 3, capturedOpts.Retries)
}

func TestExecutor_Push_Error(t *testing.T) {
	pushErr := errors.New("push rejected")
	stub := &stubGitProvider{
		pushFn: func(_ context.Context, _ *atmosgit.PushOptions) error { return pushErr },
	}
	exec := newExecutorWithProvider(stub)

	err := exec.Push(context.Background(), &atmosgit.PushOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/tmp/repo"},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, pushErr))
}

// ---- Executor.Diff tests ----

func TestExecutor_Diff_NoChanges(t *testing.T) {
	stub := &stubGitProvider{
		diffFn: func(_ context.Context, _ *atmosgit.DiffOptions) (*atmosgit.DiffResult, error) {
			return &atmosgit.DiffResult{HasChanges: false}, nil
		},
	}
	exec := newExecutorWithProvider(stub)

	result, err := exec.Diff(context.Background(), &atmosgit.DiffOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/tmp/repo"},
	})
	require.NoError(t, err)
	assert.False(t, result.HasChanges)
}

func TestExecutor_Diff_WithChanges(t *testing.T) {
	stub := &stubGitProvider{
		diffFn: func(_ context.Context, _ *atmosgit.DiffOptions) (*atmosgit.DiffResult, error) {
			return &atmosgit.DiffResult{
				HasChanges: true,
				Output:     "--- a/foo.go\n+++ b/foo.go\n@@ -1 +1 @@\n",
				Untracked:  []string{"newfile.go"},
			}, nil
		},
	}
	exec := newExecutorWithProvider(stub)

	result, err := exec.Diff(context.Background(), &atmosgit.DiffOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/tmp/repo"},
	})
	require.NoError(t, err)
	assert.True(t, result.HasChanges)
	assert.NotEmpty(t, result.Output)
	require.Len(t, result.Untracked, 1)
	assert.Equal(t, "newfile.go", result.Untracked[0])
}

// ---- Signing mode with commit opts ----

func TestExecutor_Commit_SigningAlways(t *testing.T) {
	var capturedOpts *atmosgit.CommitOptions
	stub := &stubGitProvider{
		commitFn: func(_ context.Context, opts *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			capturedOpts = opts
			return &atmosgit.CommitResult{Committed: true, SHA: "def456"}, nil
		},
	}
	exec := newExecutorWithProvider(stub)

	_, err := exec.Commit(context.Background(), &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/tmp/repo"},
		Message:     "signed commit",
		Signing:     atmosgit.SigningAlways,
	})
	require.NoError(t, err)
	require.NotNil(t, capturedOpts)
	assert.Equal(t, atmosgit.SigningAlways, capturedOpts.Signing)
}

func TestExecutor_Commit_SigningNever(t *testing.T) {
	var capturedOpts *atmosgit.CommitOptions
	stub := &stubGitProvider{
		commitFn: func(_ context.Context, opts *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			capturedOpts = opts
			return &atmosgit.CommitResult{Committed: true, SHA: "abc"}, nil
		},
	}
	exec := newExecutorWithProvider(stub)

	_, err := exec.Commit(context.Background(), &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/tmp/repo"},
		Message:     "unsigned",
		Signing:     atmosgit.SigningNever,
	})
	require.NoError(t, err)
	assert.Equal(t, atmosgit.SigningNever, capturedOpts.Signing)
}

// ---- executeCommitWithResult logic ----

func TestExecuteCommitWithResult_NothingToCommit(t *testing.T) {
	stub := &stubGitProvider{
		commitFn: func(_ context.Context, _ *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			return &atmosgit.CommitResult{Committed: false}, nil
		},
	}
	exec := newExecutorWithProvider(stub)

	err := executeCommitWithResult(context.Background(), exec, &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/tmp/repo"},
		Message:     "test",
	})
	assert.NoError(t, err)
}

func TestExecuteCommitWithResult_Committed(t *testing.T) {
	stub := &stubGitProvider{
		commitFn: func(_ context.Context, _ *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			return &atmosgit.CommitResult{Committed: true, SHA: "abc123"}, nil
		},
	}
	exec := newExecutorWithProvider(stub)

	err := executeCommitWithResult(context.Background(), exec, &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/tmp/repo"},
		Message:     "test",
	})
	assert.NoError(t, err)
}

func TestExecuteCommitWithResult_Error(t *testing.T) {
	commitErr := errors.New("dirty unmanaged files")
	stub := &stubGitProvider{
		commitFn: func(_ context.Context, _ *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
			return nil, commitErr
		},
	}
	exec := newExecutorWithProvider(stub)

	err := executeCommitWithResult(context.Background(), exec, &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/tmp/repo"},
		Message:     "test",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, commitErr))
}

// ---- buildRepoContext ----

func TestBuildRepoContext(t *testing.T) {
	env := []string{"GIT_CONFIG_KEY_0=url.https://github.com.insteadOf=git@github.com:"}
	rc := buildRepoContext("/tmp/repo", "upstream", "feature/x", env)

	assert.Equal(t, "/tmp/repo", rc.Workdir)
	assert.Equal(t, "upstream", rc.Remote)
	assert.Equal(t, "feature/x", rc.Branch)
	require.Len(t, rc.Env, 1)
	assert.Equal(t, env[0], rc.Env[0])
}

// ---- resolveAuthorFromResolved ----

func TestResolveAuthorFromResolved_Nil(t *testing.T) {
	assert.Nil(t, resolveAuthorFromResolved(nil))
}

func TestResolveAuthorFromResolved_WithAuthor(t *testing.T) {
	resolved := &atmosgit.ResolvedRepository{
		Author: &atmosgit.Author{Name: "bot", Email: "bot@example.com"},
	}
	author := resolveAuthorFromResolved(resolved)
	require.NotNil(t, author)
	assert.Equal(t, "bot", author.Name)
	assert.Equal(t, "bot@example.com", author.Email)
}

func TestResolveAuthorFromResolved_NilAuthor(t *testing.T) {
	resolved := &atmosgit.ResolvedRepository{Author: nil}
	assert.Nil(t, resolveAuthorFromResolved(resolved))
}

// ---- printStatus ----

func TestPrintStatus_Clean(t *testing.T) {
	// printStatus writes to ui; just ensure it doesn't error on clean repos.
	result := &atmosgit.StatusResult{Clean: true}
	err := printStatus("/tmp/repo", result)
	assert.NoError(t, err)
}

// ---- printDiff ----

func TestPrintDiff_NoChanges(t *testing.T) {
	result := &atmosgit.DiffResult{HasChanges: false}
	err := printDiff("/tmp/repo", result)
	assert.NoError(t, err)
}

// ---- resolveCIWorkdir ----

func TestResolveCIWorkdir_FlagOverrides(t *testing.T) {
	workdir, err := resolveCIWorkdir("/custom/path")
	require.NoError(t, err)
	assert.Equal(t, "/custom/path", workdir)
}

func TestResolveCIWorkdir_FallbackToCwd(t *testing.T) {
	workdir, err := resolveCIWorkdir("")
	require.NoError(t, err)
	assert.NotEmpty(t, workdir)
}
