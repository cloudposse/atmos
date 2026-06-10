package git

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// pullParser handles flag parsing for `atmos git pull`.
var pullParser = newPullParser()

// pullCmd is the `atmos git pull` subcommand.
var pullCmd = &cobra.Command{
	Use:   "pull <name-or-path>",
	Short: "Fast-forward pull a managed Git repository",
	Long: `Pull the latest changes for a named repository (configured under git.repositories)
or a filesystem path. Pull is always fast-forward-only.

Use --all to pull all configured repositories concurrently.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.pull.RunE")()

		v := viper.GetViper()
		if err := pullParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		branch := v.GetString(flagBranch)
		remote := v.GetString(flagRemote)
		all := v.GetBool(flagAll)
		return runPull(cmd.Context(), all, branch, remote, args)
	},
}

// runPull orchestrates the pull subcommand.
func runPull(ctx context.Context, all bool, branch, remote string, args []string) error {
	defer perf.Track(nil, "git.runPull")()

	if all && len(args) > 0 {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHint("--all is mutually exclusive with a positional repository name.").
			WithExitCode(2).
			Err()
	}

	if all {
		return runPullAll(ctx, branch, remote)
	}

	if len(args) == 0 {
		return errUtils.Build(errUtils.ErrGitRepositoryRequired).
			WithHint("Provide a repository name or path, or use --all to pull all configured repositories.").
			WithExitCode(2).
			Err()
	}

	return runPullOne(ctx, args[0], branch, remote)
}

// runPullOne pulls a single repository by name or path.
func runPullOne(ctx context.Context, arg, branch, remote string) error {
	defer perf.Track(nil, "git.runPullOne")()

	cfg := gitConfig()
	kind := classifyArg(arg, cfg)

	var workdir, identity, effectiveRemote, effectiveBranch string

	if kind == argKindName {
		resolved, err := resolveRepoByName(arg, cfg)
		if err != nil {
			return wrapRepoNotFound(err, arg)
		}
		workdir = resolved.Workdir
		identity = resolved.Identity
		effectiveRemote = resolveStringPrecedence(remote, resolved.Remote)
		effectiveBranch = resolveStringPrecedence(branch, resolved.Branch)
	} else {
		workdir = arg
		effectiveRemote = resolveStringPrecedence(remote, atmosgit.DefaultRemote)
		effectiveBranch = branch
	}

	env, err := composeEnv(ctx, identity)
	if err != nil {
		return err
	}

	exec, err := providerForName("")
	if err != nil {
		return err
	}

	return exec.Pull(ctx, &atmosgit.PullOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: workdir,
			Remote:  effectiveRemote,
			Branch:  effectiveBranch,
			Env:     env,
		},
	})
}

// runPullAll pulls all configured repositories concurrently.
func runPullAll(ctx context.Context, branch, remote string) error {
	defer perf.Track(nil, "git.runPullAll")()

	cfg := gitConfig()
	if cfg == nil || len(cfg.Repositories) == 0 {
		ui.Info("No repositories configured under git.repositories.")
		return nil
	}

	names := atmosgit.ConfiguredRepositoryNames(cfg)
	return runConcurrent(ctx, names, func(ctx context.Context, name string) error {
		return runPullOne(ctx, name, branch, remote)
	})
}

func init() {
	pullParser.RegisterFlags(pullCmd)
	if err := pullParser.BindToViper(viper.GetViper()); err != nil {
		panic(fmt.Sprintf("git pull: BindToViper: %v", err))
	}
}
