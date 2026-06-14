package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/schema"
)

// These tests verify that args after "--" on the command line flow through the
// cobra RunE wiring into the provider options. ParseFlags records
// ArgsLenAtDash on the command exactly as a full cobra Execute would, so RunE
// sees the same state without spinning up the whole root command.

func TestPushCmdForwardsNativeArgs(t *testing.T) {
	var got []string
	withTestProvider(t, &stubGitProvider{
		pushFn: func(_ context.Context, opts *atmosgit.PushOptions) error {
			got = opts.ExtraArgs
			return nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	dir := t.TempDir()
	markGitWorktree(t, dir)

	require.NoError(t, pushCmd.ParseFlags([]string{dir, "--", "--follow-tags"}))
	require.NoError(t, pushCmd.RunE(pushCmd, pushCmd.Flags().Args()))

	assert.Equal(t, []string{"--follow-tags"}, got)
}

func TestPullCmdForwardsNativeArgs(t *testing.T) {
	var got []string
	withTestProvider(t, &stubGitProvider{
		pullFn: func(_ context.Context, opts *atmosgit.PullOptions) error {
			got = opts.ExtraArgs
			return nil
		},
	})

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{}

	dir := t.TempDir()
	markGitWorktree(t, dir)

	require.NoError(t, pullCmd.ParseFlags([]string{dir, "--", "--no-tags"}))
	require.NoError(t, pullCmd.RunE(pullCmd, pullCmd.Flags().Args()))

	assert.Equal(t, []string{"--no-tags"}, got)
}

func TestPushArgsValidatorAllowsSeparatedArgs(t *testing.T) {
	// Plain cobra.ExactArgs(1) would reject `push <name> -- --follow-tags`
	// because it counts the pass-through args. The separator-aware wrapper
	// must accept it.
	require.NoError(t, pushCmd.ParseFlags([]string{"/some/repo", "--", "--follow-tags"}))
	err := pushCmd.Args(pushCmd, pushCmd.Flags().Args())
	assert.NoError(t, err)
}
