package git

import (
	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Shared flag/env-var names under the ATMOS_GIT_ prefix.
// Using non-colliding Viper keys (all prefixed with "git-") avoids keyspace
// collisions with existing Atmos bindings.

const (
	flagRepoURI    = "repo-uri"
	flagBranch     = "branch"
	flagRemote     = "remote"
	flagWorkdir    = "workdir"
	flagDepth      = "depth"
	flagFilter     = "filter"
	flagDryRun     = "dry-run"
	flagClone      = "clone"
	flagPath       = "path"
	flagMessage    = "message"
	flagSign       = "sign"
	flagNoSign     = "no-sign"
	flagSingleBr   = "single-branch"
	flagSubmodules = "submodules"
	// Shared "all" flag name used across clone, pull, and status subcommands.
	flagAll = "all"
)

// newCloneParser creates a StandardParser for `atmos git clone`.
func newCloneParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithStringFlag(flagRepoURI, "", "", "Remote repository URI (overrides configured URI)"),
		flags.WithStringFlag(flagBranch, "b", "", "Branch to clone"),
		flags.WithStringFlag(flagRemote, "", "", "Remote name (default: origin)"),
		flags.WithStringFlag(flagWorkdir, "", "", "Override destination directory"),
		flags.WithIntFlag(flagDepth, "", 0, "Shallow clone depth (0 = full history)"),
		flags.WithStringFlag(flagFilter, "", "", "Partial-clone filter spec (e.g. blob:none)"),
		flags.WithBoolFlag(flagSingleBr, "", false, "Limit clone to the specified branch"),
		flags.WithBoolFlag(flagSubmodules, "", false, "Initialize submodules after clone"),
		flags.WithBoolFlag(flagAll, "", false, "Clone/reconcile all configured repositories"),
		flags.WithEnvVars(flagRepoURI, "ATMOS_GIT_REPO_URI"),
		flags.WithEnvVars(flagBranch, "ATMOS_GIT_BRANCH"),
		flags.WithEnvVars(flagRemote, "ATMOS_GIT_REMOTE"),
		flags.WithEnvVars(flagWorkdir, "ATMOS_GIT_WORKDIR"),
		flags.WithEnvVars(flagDepth, "ATMOS_GIT_DEPTH"),
		flags.WithEnvVars(flagFilter, "ATMOS_GIT_FILTER"),
		flags.WithEnvVars(flagAll, "ATMOS_GIT_CLONE_ALL"),
	)
}

// newPullParser creates a StandardParser for `atmos git pull`.
func newPullParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithStringFlag(flagBranch, "b", "", "Branch to pull"),
		flags.WithStringFlag(flagRemote, "", "", "Remote name (default: origin)"),
		flags.WithBoolFlag(flagAll, "", false, "Pull all configured repositories"),
		flags.WithBoolFlag(flagClone, "", false, "Clone the configured repository first when the workdir is missing"),
		flags.WithEnvVars(flagBranch, "ATMOS_GIT_BRANCH"),
		flags.WithEnvVars(flagRemote, "ATMOS_GIT_REMOTE"),
		flags.WithEnvVars(flagAll, "ATMOS_GIT_PULL_ALL"),
		flags.WithEnvVars(flagClone, "ATMOS_GIT_PULL_CLONE"),
	)
}

// newStatusParser creates a StandardParser for `atmos git status`.
func newStatusParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithBoolFlag(flagAll, "", false, "Report status for all configured repositories"),
		flags.WithEnvVars(flagAll, "ATMOS_GIT_STATUS_ALL"),
	)
}

// newDiffParser creates a StandardParser for `atmos git diff`.
func newDiffParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithStringSliceFlag(flagPath, "", []string{}, "Limit diff to these repo-relative paths"),
		flags.WithEnvVars(flagPath, "ATMOS_GIT_DIFF_PATH"),
	)
}

// newCommitParser creates a StandardParser for `atmos git commit`.
func newCommitParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithStringFlag(flagMessage, "m", "", "Commit message (required)"),
		flags.WithStringSliceFlag(flagPath, "", []string{}, "Stage only these repo-relative paths"),
		flags.WithBoolFlag(flagSign, "", false, "Sign the commit with GPG (-S)"),
		flags.WithBoolFlag(flagNoSign, "", false, "Disable GPG signing (--no-gpg-sign)"),
		flags.WithBoolFlag(flagDryRun, "n", false, "Report what would be staged/committed without committing"),
		flags.WithEnvVars(flagMessage, "ATMOS_GIT_MESSAGE"),
		flags.WithEnvVars(flagDryRun, "ATMOS_GIT_DRY_RUN"),
	)
}

// newPushParser creates a StandardParser for `atmos git push`.
func newPushParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithStringFlag(flagBranch, "b", "", "Branch to push (default: current branch)"),
		flags.WithStringFlag(flagRemote, "", "", "Remote name (default: origin)"),
		flags.WithBoolFlag(flagDryRun, "n", false, "Report what would be pushed without pushing"),
		flags.WithEnvVars(flagBranch, "ATMOS_GIT_BRANCH"),
		flags.WithEnvVars(flagRemote, "ATMOS_GIT_REMOTE"),
		flags.WithEnvVars(flagDryRun, "ATMOS_GIT_DRY_RUN"),
	)
}

// newCleanParser creates a StandardParser for `atmos git clean`.
func newCleanParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithBoolFlag(flagAll, "", false, "Clean all configured repository workdirs"),
		flags.WithBoolFlag(flagForce, "f", false, "Delete dirty workdirs after safety checks"),
		flags.WithBoolFlag(flagDryRun, "n", false, "Report what would be deleted without deleting"),
		flags.WithEnvVars(flagAll, "ATMOS_GIT_CLEAN_ALL"),
		flags.WithEnvVars(flagForce, "ATMOS_GIT_CLEAN_FORCE"),
		flags.WithEnvVars(flagDryRun, "ATMOS_GIT_DRY_RUN"),
	)
}

// gitConfig returns the Git section of the loaded Atmos configuration, or nil.
func gitConfig() *schema.GitConfig {
	if atmosConfigPtr == nil {
		return nil
	}
	return &atmosConfigPtr.Git
}

// resolveStringPrecedence returns flagVal when non-empty, otherwise configVal.
func resolveStringPrecedence(flagVal, configVal string) string {
	if flagVal != "" {
		return flagVal
	}
	return configVal
}

// resolveIntPrecedence returns flagVal when non-zero, otherwise configVal.
func resolveIntPrecedence(flagVal, configVal int) int {
	if flagVal != 0 {
		return flagVal
	}
	return configVal
}

// resolveWorkdir returns workdirFlag when non-empty, otherwise repoWorkdir.
func resolveWorkdir(workdirFlag, repoWorkdir string) string {
	if workdirFlag != "" {
		return workdirFlag
	}
	return repoWorkdir
}

// wrapRepoNotFound wraps ErrGitRepositoryNotFound with actionable hints.
func wrapRepoNotFound(err error, name string) error {
	cfg := gitConfig()
	names := atmosgit.ConfiguredRepositoryNames(cfg)
	var listHint string
	if len(names) == 0 {
		listHint = "No repositories are configured under git.repositories in atmos.yaml."
	} else {
		listHint = "Run 'atmos git list' to see configured repository names."
	}
	return errUtils.Build(err).
		WithHintf("Repository %q is not configured under git.repositories.", name).
		WithHint(listHint).
		WithExitCode(2).
		Err()
}
