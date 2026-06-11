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
	Use:   "pull [name-or-path]",
	Short: "Fast-forward pull a managed Git repository",
	Long: `Pull the latest changes for a named repository (configured under git.repositories)
or a filesystem path. When no argument is provided, Atmos pulls the single
configured repository. Pull is always fast-forward-only.

Use the --clone flag to clone a configured repository when its workdir is missing.
Use the --all flag to pull all configured repositories concurrently.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.pull.RunE")()

		v := viper.GetViper()
		if err := pullParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		branch := v.GetString(flagBranch)
		remote := v.GetString(flagRemote)
		opts := &pullOptions{
			All:    v.GetBool(flagAll),
			Clone:  v.GetBool(flagClone),
			Branch: branch,
			Remote: remote,
		}
		return runPull(cmd.Context(), opts, args)
	},
}

type pullOptions struct {
	All    bool
	Clone  bool
	Branch string
	Remote string
}

type pullTarget struct {
	Workdir  string
	Remote   string
	Branch   string
	Identity string
}

// runPull orchestrates the pull subcommand.
func runPull(ctx context.Context, opts *pullOptions, args []string) error {
	defer perf.Track(nil, "git.runPull")()

	if opts.All && len(args) > 0 {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHint("--all is mutually exclusive with a positional repository name.").
			WithExitCode(2).
			Err()
	}

	if opts.All {
		return runPullAll(ctx, opts)
	}

	if len(args) == 0 {
		if name, ok := singleConfiguredRepositoryName(gitConfig()); ok {
			return runPullOne(ctx, name, opts)
		}
		return errUtils.Build(errUtils.ErrGitRepositoryRequired).
			WithHint("Provide a repository name or path to pull.").
			WithHint("Use 'atmos git pull --all' to pull all configured repositories.").
			WithHint("When exactly one repository is configured under git.repositories, no-arg pull uses that repository.").
			WithExitCode(2).
			Err()
	}

	return runPullOne(ctx, args[0], opts)
}

// runPullOne pulls a single repository by name or path.
func runPullOne(ctx context.Context, arg string, opts *pullOptions) error {
	defer perf.Track(nil, "git.runPullOne")()

	if opts == nil {
		opts = &pullOptions{}
	}

	target, cloned, err := resolvePullTarget(ctx, arg, opts)
	if err != nil || cloned {
		return err
	}

	env, err := composeEnv(ctx, target.Identity)
	if err != nil {
		return err
	}

	exec, err := providerForName("")
	if err != nil {
		return err
	}

	return exec.Pull(ctx, &atmosgit.PullOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: target.Workdir,
			Remote:  target.Remote,
			Branch:  target.Branch,
			Env:     env,
		},
	})
}

func resolvePullTarget(ctx context.Context, arg string, opts *pullOptions) (*pullTarget, bool, error) {
	cfg := gitConfig()
	if classifyArg(arg, cfg) != argKindName {
		target, err := resolvePullPathTarget(arg, opts)
		return target, false, err
	}

	resolved, err := resolveRepoByName(arg, cfg)
	if err != nil {
		return nil, false, wrapRepoNotFound(err, arg)
	}
	if !isGitWorktreePath(resolved.Workdir) {
		if !opts.Clone {
			return nil, false, requireClonedNamedWorkdir(arg, resolved.Workdir, "pull")
		}
		return nil, true, runCloneNamed(ctx, arg, &cloneOptions{
			Branch: opts.Branch,
			Remote: opts.Remote,
			All:    opts.All,
		})
	}

	return &pullTarget{
		Workdir:  resolved.Workdir,
		Identity: resolved.Identity,
		Remote:   resolveStringPrecedence(opts.Remote, resolved.Remote),
		Branch:   resolveStringPrecedence(opts.Branch, resolved.Branch),
	}, false, nil
}

func resolvePullPathTarget(arg string, opts *pullOptions) (*pullTarget, error) {
	if opts.Clone {
		return nil, errUtils.Build(errUtils.ErrInvalidFlag).
			WithHint("--clone can only be used with a configured repository name.").
			WithHint("Run 'atmos git clone <name>' for configured repositories, or clone local paths manually.").
			WithExitCode(2).
			Err()
	}

	return &pullTarget{
		Workdir: arg,
		Remote:  resolveStringPrecedence(opts.Remote, atmosgit.DefaultRemote),
		Branch:  opts.Branch,
	}, nil
}

// runPullAll pulls all configured repositories concurrently.
func runPullAll(ctx context.Context, opts *pullOptions) error {
	defer perf.Track(nil, "git.runPullAll")()

	if opts == nil {
		opts = &pullOptions{}
	}

	cfg := gitConfig()
	if cfg == nil || len(cfg.Repositories) == 0 {
		ui.Info("No repositories configured under git.repositories.")
		return nil
	}

	names := atmosgit.ConfiguredRepositoryNames(cfg)
	return runConcurrent(ctx, names, func(ctx context.Context, name string) error {
		return runPullOne(ctx, name, opts)
	})
}

func init() {
	pullParser.RegisterFlags(pullCmd)
	if err := pullParser.BindToViper(viper.GetViper()); err != nil {
		panic(fmt.Sprintf("git pull: BindToViper: %v", err))
	}
}
