package git

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Shared flag/env-var names under the ATMOS_GIT_ prefix.
//
// Every parser sets a per-subcommand Viper prefix (e.g. "git.clone"), so flags
// are stored under namespaced keys like "git.clone.branch". This prevents two
// kinds of keyspace collisions on the global Viper:
//   - cross-command: "dry-run" is also bound by workflow/terraform with
//     different env vars (ATMOS_WORKFLOW_DRY_RUN, ATMOS_WORKDIR_DRY_RUN);
//   - intra-git: clone/pull/status/clean each bind "all" to a different env
//     var (ATMOS_GIT_CLONE_ALL vs ATMOS_GIT_PULL_ALL, ...), and Viper appends
//     env bindings per key, so a shared key would bleed across subcommands.
//
// Read values with viperKey(<subcommand>ViperPrefix, flagName).

// Per-subcommand Viper key prefixes.
const (
	initViperPrefix   = "git.init"
	cloneViperPrefix  = "git.clone"
	pullViperPrefix   = "git.pull"
	pushViperPrefix   = "git.push"
	statusViperPrefix = "git.status"
	diffViperPrefix   = "git.diff"
	commitViperPrefix = "git.commit"
	cleanViperPrefix  = "git.clean"
	listViperPrefix   = "git.list"
)

// viperKey returns the namespaced Viper key for a git subcommand flag.
func viperKey(prefix, flagName string) string {
	return prefix + "." + flagName
}

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
	flagFrom       = "from"
	flagKeepHist   = "keep-history"
	flagForce      = "force"
	// Opt-out flag for the fork-checkout safety gate.
	flagAllowUnsafeFork = "allow-unsafe-fork"
	// Shared "all" flag name used across clone, pull, and status subcommands.
	flagAll = "all"
)

// Shorthands and env var names shared by several subcommands.
const (
	branchShorthand = "b"
	dryRunShorthand = "n"
	envGitBranch    = "ATMOS_GIT_BRANCH"
	envGitDryRun    = "ATMOS_GIT_DRY_RUN"
)

// newInitParser creates a StandardParser for `atmos git init`.
func newInitParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithViperPrefix(initViperPrefix),
		flags.WithStringFlag(flagFrom, "", "", "Seed the new repository from another repository URI"),
		flags.WithBoolFlag(flagKeepHist, "", false, "Keep the --from repository's history and add it as an 'upstream' remote"),
		flags.WithStringFlag(flagBranch, branchShorthand, "", "Initial branch name"),
		flags.WithStringFlag(flagWorkdir, "", "", "Override destination directory"),
		flags.WithBoolFlag(flagForce, "f", false, "Delete an existing workdir and re-initialize from scratch (destructive)"),
		flags.WithBoolFlag(flagDryRun, dryRunShorthand, false, "Report what would be done without initializing"),
		flags.WithEnvVars(flagFrom, "ATMOS_GIT_FROM"),
		flags.WithEnvVars(flagKeepHist, "ATMOS_GIT_KEEP_HISTORY"),
		flags.WithEnvVars(flagBranch, envGitBranch),
		flags.WithEnvVars(flagWorkdir, "ATMOS_GIT_WORKDIR"),
		flags.WithEnvVars(flagForce, "ATMOS_GIT_FORCE"),
		flags.WithEnvVars(flagDryRun, envGitDryRun),
	)
}

// newCloneParser creates a StandardParser for `atmos git clone`.
func newCloneParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithViperPrefix(cloneViperPrefix),
		flags.WithStringFlag(flagRepoURI, "", "", "Remote repository URI (overrides configured URI)"),
		flags.WithStringFlag(flagBranch, branchShorthand, "", "Branch to clone"),
		flags.WithStringFlag(flagRemote, "", "", "Remote name (default: origin)"),
		flags.WithStringFlag(flagWorkdir, "", "", "Override destination directory"),
		flags.WithIntFlag(flagDepth, "", 0, "Shallow clone depth (0 = full history)"),
		flags.WithStringFlag(flagFilter, "", "", "Partial-clone filter spec (e.g. blob:none)"),
		flags.WithBoolFlag(flagSingleBr, "", false, "Limit clone to the specified branch"),
		flags.WithBoolFlag(flagSubmodules, "", false, "Initialize submodules after clone"),
		flags.WithBoolFlag(flagAll, "", false, "Clone/reconcile all configured repositories"),
		flags.WithBoolFlag(flagAllowUnsafeFork, "", false, "Allow cloning untrusted fork content in pull_request_target/workflow_run events (unsafe)"),
		flags.WithEnvVars(flagRepoURI, "ATMOS_GIT_REPO_URI"),
		flags.WithEnvVars(flagBranch, envGitBranch),
		flags.WithEnvVars(flagRemote, "ATMOS_GIT_REMOTE"),
		flags.WithEnvVars(flagWorkdir, "ATMOS_GIT_WORKDIR"),
		flags.WithEnvVars(flagDepth, "ATMOS_GIT_DEPTH"),
		flags.WithEnvVars(flagFilter, "ATMOS_GIT_FILTER"),
		flags.WithEnvVars(flagSingleBr, "ATMOS_GIT_SINGLE_BRANCH"),
		flags.WithEnvVars(flagSubmodules, "ATMOS_GIT_SUBMODULES"),
		flags.WithEnvVars(flagAll, "ATMOS_GIT_CLONE_ALL"),
		flags.WithEnvVars(flagAllowUnsafeFork, "ATMOS_ALLOW_UNSAFE_FORK_EXECUTION"),
	)
}

// newPullParser creates a StandardParser for `atmos git pull`.
func newPullParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithViperPrefix(pullViperPrefix),
		flags.WithStringFlag(flagBranch, branchShorthand, "", "Branch to pull"),
		flags.WithStringFlag(flagRemote, "", "", "Remote name (default: origin)"),
		flags.WithBoolFlag(flagAll, "", false, "Pull all configured repositories"),
		flags.WithBoolFlag(flagClone, "", false, "Clone the configured repository first when the workdir is missing"),
		flags.WithEnvVars(flagBranch, envGitBranch),
		flags.WithEnvVars(flagRemote, "ATMOS_GIT_REMOTE"),
		flags.WithEnvVars(flagAll, "ATMOS_GIT_PULL_ALL"),
		flags.WithEnvVars(flagClone, "ATMOS_GIT_PULL_CLONE"),
	)
}

// newStatusParser creates a StandardParser for `atmos git status`.
func newStatusParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithViperPrefix(statusViperPrefix),
		flags.WithBoolFlag(flagAll, "", false, "Report status for all configured repositories"),
		flags.WithEnvVars(flagAll, "ATMOS_GIT_STATUS_ALL"),
	)
}

// newDiffParser creates a StandardParser for `atmos git diff`.
func newDiffParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithViperPrefix(diffViperPrefix),
		flags.WithStringSliceFlag(flagPath, "", []string{}, "Limit diff to these repo-relative paths"),
		flags.WithEnvVars(flagPath, "ATMOS_GIT_DIFF_PATH"),
	)
}

// newCommitParser creates a StandardParser for `atmos git commit`.
func newCommitParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithViperPrefix(commitViperPrefix),
		flags.WithStringFlag(flagMessage, "m", "", "Commit message (required)"),
		flags.WithStringSliceFlag(flagPath, "", []string{}, "Stage only these repo-relative paths"),
		flags.WithBoolFlag(flagSign, "", false, "Sign the commit with GPG (-S)"),
		flags.WithBoolFlag(flagNoSign, "", false, "Disable GPG signing (--no-gpg-sign)"),
		flags.WithBoolFlag(flagDryRun, dryRunShorthand, false, "Report what would be staged/committed without committing"),
		flags.WithEnvVars(flagMessage, "ATMOS_GIT_MESSAGE"),
		flags.WithEnvVars(flagPath, "ATMOS_GIT_COMMIT_PATH"),
		flags.WithEnvVars(flagSign, "ATMOS_GIT_SIGN"),
		flags.WithEnvVars(flagNoSign, "ATMOS_GIT_NO_SIGN"),
		flags.WithEnvVars(flagDryRun, envGitDryRun),
	)
}

// newPushParser creates a StandardParser for `atmos git push`.
func newPushParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithViperPrefix(pushViperPrefix),
		flags.WithStringFlag(flagBranch, branchShorthand, "", "Branch to push (default: current branch)"),
		flags.WithStringFlag(flagRemote, "", "", "Remote name (default: origin)"),
		flags.WithBoolFlag(flagDryRun, dryRunShorthand, false, "Report what would be pushed without pushing"),
		flags.WithEnvVars(flagBranch, envGitBranch),
		flags.WithEnvVars(flagRemote, "ATMOS_GIT_REMOTE"),
		flags.WithEnvVars(flagDryRun, envGitDryRun),
	)
}

// newCleanParser creates a StandardParser for `atmos git clean`.
func newCleanParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithViperPrefix(cleanViperPrefix),
		flags.WithBoolFlag(flagAll, "", false, "Clean all configured repository workdirs"),
		flags.WithBoolFlag(flagForce, "f", false, "Delete dirty workdirs after safety checks"),
		flags.WithBoolFlag(flagDryRun, dryRunShorthand, false, "Report what would be deleted without deleting"),
		flags.WithEnvVars(flagAll, "ATMOS_GIT_CLEAN_ALL"),
		flags.WithEnvVars(flagForce, "ATMOS_GIT_CLEAN_FORCE"),
		flags.WithEnvVars(flagDryRun, envGitDryRun),
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

// completeRepoNames is a cobra ValidArgsFunction completing the positional
// argument with repository names configured under git.repositories. Commands
// that also accept filesystem paths keep default file completion as well.
func completeRepoNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return atmosgit.ConfiguredRepositoryNames(gitConfig()), cobra.ShellCompDirectiveDefault
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
