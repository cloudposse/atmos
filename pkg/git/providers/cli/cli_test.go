package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

func TestProviderRegistersAsCli(t *testing.T) {
	provider, err := atmosgit.NewProvider("cli")
	require.NoError(t, err)
	assert.IsType(t, &Provider{}, provider)

	// Empty name resolves to the default cli provider.
	provider, err = atmosgit.NewProvider("")
	require.NoError(t, err)
	assert.IsType(t, &Provider{}, provider)
}

func TestCloneFreshBuildsFullArgs(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	workdir := filepath.Join(t.TempDir(), "repos", "deploy")

	err := provider.Clone(context.Background(), &atmosgit.CloneOptions{
		RepoContext:  atmosgit.RepoContext{Workdir: workdir, Branch: "main", Remote: "origin"},
		URI:          "https://github.com/acme/deploy.git",
		Depth:        1,
		Filter:       "blob:none",
		SingleBranch: true,
		Submodules:   true,
	})
	require.NoError(t, err)

	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{
		"clone", "--depth", "1", "--filter", "blob:none", "--single-branch",
		"--branch", "main", "--recurse-submodules", "--",
		"https://github.com/acme/deploy.git", workdir,
	}, runner.calls[0].args)

	// Workdir parent is created so git clone can materialize the target.
	parent := filepath.Dir(workdir)
	info, statErr := os.Stat(parent)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestCloneFreshMinimalArgs(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	workdir := filepath.Join(t.TempDir(), "deploy")

	err := provider.Clone(context.Background(), &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir},
		URI:         "https://github.com/acme/deploy.git",
	})
	require.NoError(t, err)

	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{"clone", "--", "https://github.com/acme/deploy.git", workdir}, runner.calls[0].args)
}

func TestCloneReconcilesExistingWorkdir(t *testing.T) {
	runner := newFakeRunner()
	// The common case: the workdir's remote already matches the configured
	// URI (unchanged since the original clone), so reconcile must not issue
	// a redundant "remote set-url".
	runner.on("remote get-url origin", atmosgit.RunResult{Stdout: "https://github.com/acme/deploy.git\n"}, nil)
	provider := New(WithRunner(runner))
	workdir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(workdir, ".git"), 0o755))

	err := provider.Clone(context.Background(), &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir, Branch: "main", Remote: "origin"},
		URI:         "https://github.com/acme/deploy.git",
		Depth:       1,
	})
	require.NoError(t, err)

	calls := runner.joinedCalls()
	require.Len(t, calls, 5)
	assert.Equal(t, "status --porcelain --untracked-files=all", calls[0])
	assert.Equal(t, "remote get-url origin", calls[1])
	assert.Equal(t, "fetch origin +refs/heads/main:refs/remotes/origin/main --depth 1", calls[2])
	assert.Equal(t, "checkout main", calls[3])
	assert.Equal(t, "merge --ff-only origin/main", calls[4])
}

// TestCloneReconcileUpdatesRemoteURLOnChange covers the config-drift case: the
// workdir's existing "origin" remote no longer matches the configured URI
// (e.g. a corrected typo or a migrated repository), so reconcile must repoint
// it via "remote set-url" before fetching -- otherwise the correction in
// config would be silently ignored by an already-cloned workdir forever,
// since fetch/pull address the remote by name, never by the configured URI.
func TestCloneReconcileUpdatesRemoteURLOnChange(t *testing.T) {
	runner := newFakeRunner()
	runner.on("remote get-url origin", atmosgit.RunResult{Stdout: "https://github.com/acme/old-deploy.git\n"}, nil)
	provider := New(WithRunner(runner))
	workdir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(workdir, ".git"), 0o755))

	err := provider.Clone(context.Background(), &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir, Branch: "main", Remote: "origin"},
		URI:         "https://github.com/acme/deploy.git",
		Depth:       1,
	})
	require.NoError(t, err)

	calls := runner.joinedCalls()
	require.Len(t, calls, 6)
	assert.Equal(t, "status --porcelain --untracked-files=all", calls[0])
	assert.Equal(t, "remote get-url origin", calls[1])
	assert.Equal(t, "remote set-url origin https://github.com/acme/deploy.git", calls[2])
	assert.Equal(t, "fetch origin +refs/heads/main:refs/remotes/origin/main --depth 1", calls[3])
	assert.Equal(t, "checkout main", calls[4])
	assert.Equal(t, "merge --ff-only origin/main", calls[5])
}

func TestCloneReconcileRefusesDirtyWorkdir(t *testing.T) {
	runner := newFakeRunner()
	runner.on("status --porcelain", atmosgit.RunResult{Stdout: " M leftover.yaml\n"}, nil)
	provider := New(WithRunner(runner))
	workdir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(workdir, ".git"), 0o755))

	err := provider.Clone(context.Background(), &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir, Branch: "main"},
		URI:         "https://github.com/acme/deploy.git",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitDirtyUnmanagedFiles))
}

func TestCloneReconcileCreatesBranchFallback(t *testing.T) {
	runner := newFakeRunner()
	runner.on("checkout main", atmosgit.RunResult{ExitCode: 1}, exitErr(1))
	runner.on("checkout -b main origin/main", atmosgit.RunResult{}, nil)
	provider := New(WithRunner(runner))
	workdir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(workdir, ".git"), 0o755))

	err := provider.Clone(context.Background(), &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir, Branch: "main"},
		URI:         "https://github.com/acme/deploy.git",
	})
	require.NoError(t, err)

	calls := runner.joinedCalls()
	assert.Contains(t, calls, "checkout -b main origin/main")
}

func TestPullIsFastForwardOnly(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))

	err := provider.Pull(context.Background(), &atmosgit.PullOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w", Remote: "upstream", Branch: "main"},
	})
	require.NoError(t, err)

	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{"pull", "--ff-only", "upstream", "main"}, runner.calls[0].args)
	assert.Equal(t, "/w", runner.calls[0].dir)
}

func TestPullDefaultsRemote(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))

	err := provider.Pull(context.Background(), &atmosgit.PullOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pull", "--ff-only", "origin"}, runner.calls[0].args)
}

func TestPullNoTrackingBranchClassified(t *testing.T) {
	runner := newFakeRunner()
	runner.on("pull", atmosgit.RunResult{
		ExitCode:   1,
		StderrTail: "You asked to pull from the remote 'origin', but did not specify a branch.",
	}, exitErr(1))
	provider := New(WithRunner(runner))

	err := provider.Pull(context.Background(), &atmosgit.PullOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w"},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitNoTrackingBranch))
}

func TestCloneAuthFailureClassified(t *testing.T) {
	runner := newFakeRunner()
	runner.on("clone", atmosgit.RunResult{ExitCode: 128, StderrTail: "fatal: Authentication failed for 'https://github.com/acme/deploy.git'"}, exitErr(128))
	provider := New(WithRunner(runner))

	err := provider.Clone(context.Background(), &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{Workdir: filepath.Join(t.TempDir(), "x")},
		URI:         "https://github.com/acme/deploy.git",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitAuthFailed))
	// The stderr tail (which may contain secrets) is never embedded.
	assert.NotContains(t, err.Error(), "Authentication failed for")
}

func TestEnvPassedThroughToRunner(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	env := []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=include.path", "GIT_CONFIG_VALUE_0=/tmp/cfg"}

	err := provider.Pull(context.Background(), &atmosgit.PullOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w", Env: env},
	})
	require.NoError(t, err)
	assert.Equal(t, env, runner.calls[0].env)
}
