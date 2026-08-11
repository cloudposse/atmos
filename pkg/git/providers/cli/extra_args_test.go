package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

// These tests verify that native passthrough args (everything after "--" on
// the atmos command line) reach the underlying git invocation verbatim.

func TestCloneFreshAppendsExtraArgsBeforeSeparator(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))
	workdir := filepath.Join(t.TempDir(), "deploy")

	err := provider.Clone(context.Background(), &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workdir},
		URI:         "https://github.com/acme/deploy.git",
		ExtraArgs:   []string{"--no-tags", "--origin", "upstream"},
	})
	require.NoError(t, err)

	require.Len(t, runner.calls, 1)
	// Extra args are git clone flags, so they must precede the "--" that
	// separates flags from the URI/workdir positionals.
	assert.Equal(t, []string{
		"clone", "--no-tags", "--origin", "upstream", "--",
		"https://github.com/acme/deploy.git", workdir,
	}, runner.calls[0].args)
}

func TestPullAppendsExtraArgs(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))

	err := provider.Pull(context.Background(), &atmosgit.PullOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w", Remote: "origin", Branch: "main"},
		ExtraArgs:   []string{"--no-tags"},
	})
	require.NoError(t, err)

	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{"pull", "--ff-only", "origin", "main", "--no-tags"}, runner.calls[0].args)
}

func TestPushAppendsExtraArgs(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))

	err := provider.Push(context.Background(), &atmosgit.PushOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w", Remote: "origin", Branch: "main"},
		ExtraArgs:   []string{"--follow-tags"},
	})
	require.NoError(t, err)

	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{"push", "origin", "main", "--follow-tags"}, runner.calls[0].args)
}
