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

func TestInitEmptyCreatesRepoWithRemote(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	workdir := filepath.Join(t.TempDir(), "repos", "deploy")

	err := provider.Init(context.Background(), &atmosgit.InitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir, Branch: "main", Remote: "origin"},
		URI:         "https://github.com/acme/deploy.git",
	})
	require.NoError(t, err)

	calls := runner.joinedCalls()
	require.Len(t, calls, 2)
	assert.Equal(t, "init -b main", calls[0])
	assert.Equal(t, "remote add origin https://github.com/acme/deploy.git", calls[1])
	assert.Equal(t, workdir, runner.calls[0].dir)

	// The workdir itself is created so git init can run inside it.
	info, statErr := os.Stat(workdir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestInitEmptyDefaultsBranchAndRemote(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	workdir := filepath.Join(t.TempDir(), "deploy")

	err := provider.Init(context.Background(), &atmosgit.InitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir},
		URI:         "https://github.com/acme/deploy.git",
	})
	require.NoError(t, err)

	calls := runner.joinedCalls()
	require.Len(t, calls, 2)
	// No -b flag: git's own init.defaultBranch applies.
	assert.Equal(t, "init", calls[0])
	assert.Equal(t, "remote add origin https://github.com/acme/deploy.git", calls[1])
}

func TestInitEmptyAppendsExtraArgs(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	workdir := filepath.Join(t.TempDir(), "deploy")
	tpl := filepath.Join(t.TempDir(), "tpl")

	err := provider.Init(context.Background(), &atmosgit.InitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir, Branch: "main"},
		URI:         "https://github.com/acme/deploy.git",
		ExtraArgs:   []string{"--template", tpl},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"init", "-b", "main", "--template", tpl}, runner.calls[0].args)
}

func TestInitFromFreshImportsContentWithSingleCommit(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	workdir := filepath.Join(t.TempDir(), "deploy")

	err := provider.Init(context.Background(), &atmosgit.InitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir, Branch: "main", Remote: "origin"},
		URI:         "https://github.com/acme/deploy.git",
		FromURI:     "https://github.com/acme/template.git",
	})
	require.NoError(t, err)

	calls := runner.joinedCalls()
	require.Len(t, calls, 5)
	// History is discarded, so the source clone is shallow; the configured
	// branch names the NEW history (git init -b), not a source branch.
	assert.Equal(t, "clone --depth 1 -- https://github.com/acme/template.git "+workdir, calls[0])
	assert.Equal(t, "init -b main", calls[1])
	assert.Equal(t, "remote add origin https://github.com/acme/deploy.git", calls[2])
	assert.Equal(t, "add -A", calls[3])
	assert.Equal(t, "commit -m Initialize from https://github.com/acme/template.git --allow-empty", calls[4])
}

func TestInitFromFreshSignsAndSetsAuthor(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	workdir := filepath.Join(t.TempDir(), "deploy")

	err := provider.Init(context.Background(), &atmosgit.InitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir},
		URI:         "https://github.com/acme/deploy.git",
		FromURI:     "https://github.com/acme/template.git",
		Signing:     atmosgit.SigningNever,
		Author:      &atmosgit.Author{Name: "Bot", Email: "bot@acme.com"},
	})
	require.NoError(t, err)

	commitCall := runner.calls[len(runner.calls)-1].args
	assert.Equal(t, []string{
		"-c", "user.name=Bot", "-c", "user.email=bot@acme.com",
		"commit", "-m", "Initialize from https://github.com/acme/template.git",
		"--no-gpg-sign", "--allow-empty",
	}, commitCall)
}

func TestInitFromKeepHistoryPreservesSourceAsUpstream(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	workdir := filepath.Join(t.TempDir(), "deploy")

	err := provider.Init(context.Background(), &atmosgit.InitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir, Branch: "main", Remote: "origin"},
		URI:         "https://github.com/acme/deploy.git",
		FromURI:     "https://github.com/acme/old-deploy.git",
		KeepHistory: true,
	})
	require.NoError(t, err)

	calls := runner.joinedCalls()
	require.Len(t, calls, 3)
	// Full history (no --depth); the configured branch must exist in the source.
	assert.Equal(t, "clone --branch main -- https://github.com/acme/old-deploy.git "+workdir, calls[0])
	assert.Equal(t, "remote rename origin upstream", calls[1])
	assert.Equal(t, "remote add origin https://github.com/acme/deploy.git", calls[2])
}

func TestInitFromKeepHistoryWithUpstreamRemoteUsesSource(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	workdir := filepath.Join(t.TempDir(), "deploy")

	err := provider.Init(context.Background(), &atmosgit.InitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir, Remote: "upstream"},
		URI:         "https://github.com/acme/deploy.git",
		FromURI:     "https://github.com/acme/old-deploy.git",
		KeepHistory: true,
	})
	require.NoError(t, err)

	calls := runner.joinedCalls()
	require.Len(t, calls, 3)
	// The configured remote claims "upstream", so the source moves to "source".
	assert.Equal(t, "remote rename origin source", calls[1])
	assert.Equal(t, "remote add upstream https://github.com/acme/deploy.git", calls[2])
}

func TestInitRefusesNonEmptyWorkdir(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	workdir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "existing.txt"), []byte("x"), 0o644))

	err := provider.Init(context.Background(), &atmosgit.InitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir},
		URI:         "https://github.com/acme/deploy.git",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitWorkdirExists))
	assert.Empty(t, runner.calls, "no git command may run against a non-empty target")
}

func TestInitAllowsExistingEmptyWorkdir(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	workdir := t.TempDir()

	err := provider.Init(context.Background(), &atmosgit.InitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir, Branch: "main"},
		URI:         "https://github.com/acme/deploy.git",
	})
	require.NoError(t, err)
	assert.Equal(t, "init -b main", runner.joinedCalls()[0])
}
