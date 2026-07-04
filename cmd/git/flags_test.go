package git

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGitViperPrefixIsolation verifies the git.* Viper key namespace shields
// git env bindings from other commands. Without the per-subcommand prefixes,
// workflow/terraform also bind the bare "dry-run" key (to different env vars)
// on the global Viper, and whichever package init() ran last won — making
// ATMOS_GIT_DRY_RUN silently ignored.
func TestGitViperPrefixIsolation(t *testing.T) {
	v := viper.New()
	// Simulate another command binding the bare "dry-run" key to its own env var.
	require.NoError(t, v.BindEnv("dry-run", "ATMOS_WORKFLOW_DRY_RUN"))

	require.NoError(t, newCommitParser().BindToViper(v))

	t.Setenv("ATMOS_GIT_DRY_RUN", "true")

	assert.True(t, v.GetBool(viperKey(commitViperPrefix, flagDryRun)),
		"ATMOS_GIT_DRY_RUN must reach the git.commit.dry-run key")
	assert.False(t, v.GetBool("dry-run"),
		"ATMOS_GIT_DRY_RUN must not leak into the unprefixed dry-run key")
}

// TestGitAllEnvVarsIsolatedPerSubcommand verifies that per-subcommand env vars
// sharing a flag name (--all) do not bleed across git subcommands. Viper
// appends env bindings per key, so a shared "git.all" key would make
// ATMOS_GIT_CLONE_ALL also enable --all for status/pull/clean.
func TestGitAllEnvVarsIsolatedPerSubcommand(t *testing.T) {
	v := viper.New()
	require.NoError(t, newCloneParser().BindToViper(v))
	require.NoError(t, newStatusParser().BindToViper(v))

	t.Setenv("ATMOS_GIT_CLONE_ALL", "true")

	assert.True(t, v.GetBool(viperKey(cloneViperPrefix, flagAll)),
		"ATMOS_GIT_CLONE_ALL must enable git clone --all")
	assert.False(t, v.GetBool(viperKey(statusViperPrefix, flagAll)),
		"ATMOS_GIT_CLONE_ALL must not enable git status --all")
}

// TestGitCommitFlagsBindEnvVars verifies that every commit flag is settable
// via its ATMOS_GIT_* environment variable (CI automation needs --no-sign
// without CLI access).
func TestGitCommitFlagsBindEnvVars(t *testing.T) {
	v := viper.New()
	require.NoError(t, newCommitParser().BindToViper(v))

	t.Setenv("ATMOS_GIT_SIGN", "true")
	t.Setenv("ATMOS_GIT_NO_SIGN", "true")
	t.Setenv("ATMOS_GIT_COMMIT_PATH", "stacks/dev.yaml")

	assert.True(t, v.GetBool(viperKey(commitViperPrefix, flagSign)))
	assert.True(t, v.GetBool(viperKey(commitViperPrefix, flagNoSign)))
	assert.Equal(t, []string{"stacks/dev.yaml"}, v.GetStringSlice(viperKey(commitViperPrefix, flagPath)))
}

// TestGitCloneFlagsBindEnvVars verifies the clone-only boolean flags are
// settable via environment variables.
func TestGitCloneFlagsBindEnvVars(t *testing.T) {
	v := viper.New()
	require.NoError(t, newCloneParser().BindToViper(v))

	t.Setenv("ATMOS_GIT_SINGLE_BRANCH", "true")
	t.Setenv("ATMOS_GIT_SUBMODULES", "true")

	assert.True(t, v.GetBool(viperKey(cloneViperPrefix, flagSingleBr)))
	assert.True(t, v.GetBool(viperKey(cloneViperPrefix, flagSubmodules)))
}
