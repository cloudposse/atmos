package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

func TestStatusParsesPorcelain(t *testing.T) {
	runner := newFakeRunner()
	runner.on("status --porcelain", atmosgit.RunResult{Stdout: " M clusters/prod/app.yaml\n?? clusters/prod/new.yaml\n"}, nil)
	provider := New(WithRunner(runner))

	status, err := provider.Status(context.Background(), &atmosgit.StatusOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w"},
	})
	require.NoError(t, err)

	assert.False(t, status.Clean)
	require.Len(t, status.Entries, 2)
	assert.Equal(t, atmosgit.StatusEntry{Code: " M", Path: "clusters/prod/app.yaml"}, status.Entries[0])
	assert.Equal(t, atmosgit.StatusEntry{Code: "??", Path: "clusters/prod/new.yaml"}, status.Entries[1])
}

func TestStatusCleanAndPathScoped(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))

	status, err := provider.Status(context.Background(), &atmosgit.StatusOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w"},
		Paths:       []string{"clusters/prod"},
	})
	require.NoError(t, err)
	assert.True(t, status.Clean)
	assert.Empty(t, status.Entries)
	assert.Equal(t, []string{"status", "--porcelain", "--untracked-files=all", "--", "clusters/prod"}, runner.calls[0].args)
}

func TestDiffReportsChangesAndUntracked(t *testing.T) {
	runner := newFakeRunner()
	runner.on("status --porcelain", atmosgit.RunResult{Stdout: " M a.yaml\n?? b.yaml\n"}, nil)
	runner.on("rev-parse --verify HEAD", atmosgit.RunResult{Stdout: "abc\n"}, nil)
	runner.on("diff HEAD", atmosgit.RunResult{Stdout: "--- a/a.yaml\n+++ b/a.yaml\n"}, nil)
	provider := New(WithRunner(runner))

	diff, err := provider.Diff(context.Background(), &atmosgit.DiffOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w"},
	})
	require.NoError(t, err)

	assert.True(t, diff.HasChanges)
	assert.Equal(t, []string{"b.yaml"}, diff.Untracked)
	assert.Contains(t, diff.Output, "+++ b/a.yaml")
}

func TestDiffNoHeadSkipsDiffInvocation(t *testing.T) {
	runner := newFakeRunner()
	runner.on("rev-parse --verify HEAD", atmosgit.RunResult{ExitCode: 128}, exitErr(128))
	provider := New(WithRunner(runner))

	diff, err := provider.Diff(context.Background(), &atmosgit.DiffOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w"},
	})
	require.NoError(t, err)
	assert.False(t, diff.HasChanges)
	assert.NotContains(t, runner.joinedCalls(), "diff HEAD")
}

func TestCommitPathScopedRejectsUnmanagedDirty(t *testing.T) {
	runner := newFakeRunner()
	runner.on("status --porcelain", atmosgit.RunResult{Stdout: " M clusters/prod/app.yaml\n M README.md\n"}, nil)
	provider := New(WithRunner(runner))

	_, err := provider.Commit(context.Background(), &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w"},
		Message:     "publish",
		Paths:       []string{"clusters/prod"},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitDirtyUnmanagedFiles))
	assert.Contains(t, err.Error(), "README.md")
	assert.NotContains(t, err.Error(), "clusters/prod/app.yaml")
}

func TestCommitNoChangesIsCleanNoOp(t *testing.T) {
	runner := newFakeRunner()
	runner.on("rev-parse --verify HEAD", atmosgit.RunResult{Stdout: "abc\n"}, nil)
	// diff --cached --quiet exits 0: nothing staged.
	provider := New(WithRunner(runner))

	result, err := provider.Commit(context.Background(), &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w"},
		Message:     "publish",
	})
	require.NoError(t, err)
	assert.False(t, result.Committed)
	assert.Empty(t, result.SHA)
}

func TestCommitFullFlowWithAuthorSigningTrailers(t *testing.T) {
	runner := newFakeRunner()
	runner.on("status --porcelain", atmosgit.RunResult{Stdout: " M clusters/prod/app.yaml\n"}, nil)
	runner.on("rev-parse --verify HEAD", atmosgit.RunResult{Stdout: "abc\n"}, nil)
	runner.on("diff --cached --quiet", atmosgit.RunResult{ExitCode: 1}, exitErr(1))
	runner.on("rev-parse HEAD", atmosgit.RunResult{Stdout: "deadbeef\n"}, nil)
	provider := New(WithRunner(runner))

	result, err := provider.Commit(context.Background(), &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w"},
		Message:     "Render argocd for prod",
		Paths:       []string{"clusters/prod"},
		Signing:     atmosgit.SigningAlways,
		Author:      &atmosgit.Author{Name: "atmos[bot]", Email: "bot@acme.com"},
		Trailers: map[string]string{
			"Atmos-Stack":     "prod",
			"Atmos-Component": "argocd",
		},
	})
	require.NoError(t, err)
	assert.True(t, result.Committed)
	assert.Equal(t, "deadbeef", result.SHA)

	calls := runner.joinedCalls()
	assert.Contains(t, calls, "add -- clusters/prod")

	commitCall := findCall(t, runner, "commit")
	assert.Equal(t, []string{
		"-c", "user.name=atmos[bot]",
		"-c", "user.email=bot@acme.com",
		"commit", "-m", "Render argocd for prod\n\nAtmos-Component: argocd\nAtmos-Stack: prod",
		"-S",
	}, commitCall.args)
}

func TestCommitSigningNever(t *testing.T) {
	runner := newFakeRunner()
	runner.on("rev-parse --verify HEAD", atmosgit.RunResult{Stdout: "abc\n"}, nil)
	runner.on("diff --cached --quiet", atmosgit.RunResult{ExitCode: 1}, exitErr(1))
	runner.on("rev-parse HEAD", atmosgit.RunResult{Stdout: "cafe\n"}, nil)
	provider := New(WithRunner(runner))

	result, err := provider.Commit(context.Background(), &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w"},
		Message:     "msg",
		Signing:     atmosgit.SigningNever,
	})
	require.NoError(t, err)
	assert.True(t, result.Committed)

	commitCall := findCall(t, runner, "commit")
	assert.Equal(t, []string{"commit", "-m", "msg", "--no-gpg-sign"}, commitCall.args)
}

func TestCommitInitialCommitNoHead(t *testing.T) {
	runner := newFakeRunner()
	runner.on("rev-parse --verify HEAD", atmosgit.RunResult{ExitCode: 128}, exitErr(128))
	runner.on("ls-files --cached", atmosgit.RunResult{Stdout: "app.yaml\n"}, nil)
	runner.on("rev-parse HEAD", atmosgit.RunResult{Stdout: "f00d\n"}, nil)
	provider := New(WithRunner(runner))

	result, err := provider.Commit(context.Background(), &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w"},
		Message:     "initial",
	})
	require.NoError(t, err)
	assert.True(t, result.Committed)
	assert.Equal(t, "f00d", result.SHA)
}

func findCall(t *testing.T, runner *fakeRunner, sub string) call {
	t.Helper()
	for _, c := range runner.calls {
		for _, a := range c.args {
			if a == sub {
				return c
			}
		}
	}
	t.Fatalf("no call containing %q; calls: %v", sub, runner.joinedCalls())
	return call{}
}

func TestMessageWithTrailersFormatting(t *testing.T) {
	msg := messageWithTrailers("Update\n", map[string]string{
		"Atmos-Stack":      "prod",
		"Atmos-Component":  "argocd",
		"Atmos-Source-SHA": "abc123",
	})
	assert.Equal(t, "Update\n\nAtmos-Component: argocd\nAtmos-Source-SHA: abc123\nAtmos-Stack: prod", msg)

	assert.Equal(t, "plain", messageWithTrailers("plain", nil))
}

func TestPathWithinAny(t *testing.T) {
	assert.True(t, pathWithinAny("clusters/prod/app.yaml", []string{"clusters/prod"}))
	assert.True(t, pathWithinAny("clusters/prod", []string{"clusters/prod/"}))
	assert.False(t, pathWithinAny("clusters/production/app.yaml", []string{"clusters/prod"}))
	assert.False(t, pathWithinAny("README.md", []string{"clusters/prod"}))
}
