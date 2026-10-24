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

// TestCloneReconcileRemoteSync covers syncRemoteURL's three non-error
// outcomes during reconcile of an already-cloned workdir:
//   - the remote already matches the configured URI (unchanged since the
//     original clone), so reconcile must not issue a redundant "remote
//     set-url";
//   - the remote no longer matches (e.g. a corrected typo or a migrated
//     repository), so reconcile must repoint it via "remote set-url" before
//     fetching -- otherwise the correction in config would be silently
//     ignored by an already-cloned workdir forever, since fetch/pull address
//     the remote by name, never by the configured URI;
//   - no URI is configured at all (a bare reconcile with only a
//     Workdir/Branch), so reconcile must not issue a "remote get-url" --
//     there is nothing to compare it to.
func TestCloneReconcileRemoteSync(t *testing.T) {
	tests := []struct {
		name              string
		uri               string
		remoteURLResp     string // stubbed "remote get-url origin" stdout; empty means unstubbed
		wantCalls         []string
		checkNoRemoteSync bool
	}{
		{
			name:          "remote already matches configured URI: no set-url",
			uri:           "https://github.com/acme/deploy.git",
			remoteURLResp: "https://github.com/acme/deploy.git\n",
			wantCalls: []string{
				"status --porcelain --untracked-files=all",
				"remote get-url origin",
				"fetch origin +refs/heads/main:refs/remotes/origin/main --depth 1",
				"checkout main",
				"merge --ff-only origin/main",
			},
		},
		{
			name:          "remote URI changed: repoints via remote set-url",
			uri:           "https://github.com/acme/deploy.git",
			remoteURLResp: "https://github.com/acme/old-deploy.git\n",
			wantCalls: []string{
				"status --porcelain --untracked-files=all",
				"remote get-url origin",
				"remote set-url origin https://github.com/acme/deploy.git",
				"fetch origin +refs/heads/main:refs/remotes/origin/main --depth 1",
				"checkout main",
				"merge --ff-only origin/main",
			},
		},
		{
			name: "no URI configured: skips remote sync entirely",
			wantCalls: []string{
				"status --porcelain --untracked-files=all",
				"fetch origin +refs/heads/main:refs/remotes/origin/main --depth 1",
				"checkout main",
				"merge --ff-only origin/main",
			},
			checkNoRemoteSync: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			if tt.remoteURLResp != "" {
				runner.on("remote get-url origin", atmosgit.RunResult{Stdout: tt.remoteURLResp}, nil)
			}
			provider := New(WithRunner(runner))
			workdir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(workdir, ".git"), 0o755))

			err := provider.Clone(context.Background(), &atmosgit.CloneOptions{
				RepoContext: atmosgit.RepoContext{Workdir: workdir, Branch: "main", Remote: "origin"},
				URI:         tt.uri,
				Depth:       1,
			})
			require.NoError(t, err)

			calls := runner.joinedCalls()
			assert.Equal(t, tt.wantCalls, calls)

			if tt.checkNoRemoteSync {
				for _, c := range calls {
					assert.NotContains(t, c, "remote get-url", "no URI to compare against, so no remote lookup should happen")
					assert.NotContains(t, c, "remote set-url")
				}
			}
		})
	}
}

// TestCloneReconcileRemoteGetURLFailurePropagates verifies reconcile
// surfaces a failure from "git remote get-url" (e.g. a corrupted git config)
// instead of proceeding to fetch against a remote whose URL couldn't be
// determined.
func TestCloneReconcileRemoteGetURLFailurePropagates(t *testing.T) {
	runner := newFakeRunner()
	runner.on("remote get-url origin", atmosgit.RunResult{ExitCode: 1}, exitErr(1))
	provider := New(WithRunner(runner))
	workdir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(workdir, ".git"), 0o755))

	err := provider.Clone(context.Background(), &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir, Branch: "main", Remote: "origin"},
		URI:         "https://github.com/acme/deploy.git",
		Depth:       1,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitCommandExited))

	// Must fail before ever fetching -- the remote URL could not be verified.
	calls := runner.joinedCalls()
	for _, c := range calls {
		assert.NotContains(t, c, "fetch", "must not fetch when the remote URL could not be determined")
	}
}

// TestCloneReconcileRemoteSetURLFailurePropagates verifies reconcile
// surfaces a failure from "git remote set-url" (e.g. a read-only git config)
// instead of proceeding to fetch against a remote that was never repointed
// to the configured URI.
func TestCloneReconcileRemoteSetURLFailurePropagates(t *testing.T) {
	runner := newFakeRunner()
	runner.on("remote get-url origin", atmosgit.RunResult{Stdout: "https://github.com/acme/old-deploy.git\n"}, nil)
	runner.on("remote set-url", atmosgit.RunResult{ExitCode: 1}, exitErr(1))
	provider := New(WithRunner(runner))
	workdir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(workdir, ".git"), 0o755))

	err := provider.Clone(context.Background(), &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir, Branch: "main", Remote: "origin"},
		URI:         "https://github.com/acme/deploy.git",
		Depth:       1,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitCommandExited))

	calls := runner.joinedCalls()
	for _, c := range calls {
		assert.NotContains(t, c, "fetch", "must not fetch when the remote URL could not be repointed")
	}
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
