package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

func TestHasUnpushedCommits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, branch, remote, ref, output string
		want                              bool
	}{
		{name: "synchronized", branch: "main", ref: "origin/main"},
		{name: "ahead", branch: "main", ref: "origin/main", output: "abc123\n", want: true},
		{name: "configured remote", branch: "release", remote: "publish", ref: "publish/release", output: "abc123\n", want: true},
		{name: "default branch", ref: "@{upstream}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := newFakeRunner()
			command := "rev-list --max-count=1 " + tc.ref + "..HEAD --"
			runner.on(command, atmosgit.RunResult{Stdout: tc.output}, nil)
			provider := New(WithRunner(runner))
			ahead, err := provider.HasUnpushedCommits(t.Context(), atmosgit.RepoContext{Workdir: t.TempDir(), Branch: tc.branch, Remote: tc.remote})
			require.NoError(t, err)
			assert.Equal(t, tc.want, ahead)
			assert.Equal(t, []string{command}, runner.joinedCalls())
		})
	}
}

func TestHasUnpushedCommitsFailsClosed(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.on("rev-list --max-count=1 origin/main..HEAD --", atmosgit.RunResult{ExitCode: 128, StderrTail: "unknown revision"}, exitErr(128))
	provider := New(WithRunner(runner))
	_, err := provider.HasUnpushedCommits(t.Context(), atmosgit.RepoContext{Workdir: t.TempDir(), Branch: "main"})
	require.Error(t, err)
}
