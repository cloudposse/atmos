package git

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// statusParser handles flag parsing for `atmos git status`.
var statusParser = newStatusParser()

// statusCmd is the `atmos git status` subcommand.
var statusCmd = &cobra.Command{
	Use:   "status <name-or-path>",
	Short: "Show the working tree status of a managed Git repository",
	Long: `Report the working tree status for a named repository (configured under git.repositories)
or a filesystem path. Output is the porcelain status (pipeable to stdout).

Use --all to report status for all configured repositories.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.status.RunE")()

		v := viper.GetViper()
		if err := statusParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		all := v.GetBool(flagAll)
		return runStatus(cmd.Context(), all, args)
	},
}

// runStatus orchestrates the status subcommand.
func runStatus(ctx context.Context, all bool, args []string) error {
	defer perf.Track(nil, "git.runStatus")()

	if all && len(args) > 0 {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHint("--all is mutually exclusive with a positional repository name.").
			WithExitCode(2).
			Err()
	}

	if all {
		return runStatusAll(ctx)
	}

	if len(args) == 0 {
		return errUtils.Build(errUtils.ErrGitRepositoryRequired).
			WithHint("Provide a repository name or path, or use --all to report status for all configured repositories.").
			WithExitCode(2).
			Err()
	}

	return runStatusOne(ctx, args[0])
}

// runStatusOne reports status for a single repository by name or path.
func runStatusOne(ctx context.Context, arg string) error {
	defer perf.Track(nil, "git.runStatusOne")()

	cfg := gitConfig()
	kind := classifyArg(arg, cfg)

	var workdir, identity string

	if kind == argKindName {
		resolved, err := resolveRepoByName(arg, cfg)
		if err != nil {
			return wrapRepoNotFound(err, arg)
		}
		workdir = resolved.Workdir
		identity = resolved.Identity
	} else {
		workdir = arg
	}

	env, err := composeEnv(ctx, identity)
	if err != nil {
		return err
	}

	exec, err := providerForName("")
	if err != nil {
		return err
	}

	return executeStatusAndPrint(ctx, exec, workdir, env)
}

// execStatusForTest is a seam for unit tests: runs status with an injected provider.
func execStatusForTest(ctx context.Context, exec *Executor, workdir string, env []string) (*atmosgit.StatusResult, error) {
	return exec.Status(ctx, &atmosgit.StatusOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: workdir,
			Env:     env,
		},
	})
}

// printStatus writes status output to stdout (pipeable).
func printStatus(workdir string, result *atmosgit.StatusResult) error {
	if result.Clean {
		ui.Infof("Repository at %s is clean.", workdir)
		return nil
	}

	for _, entry := range result.Entries {
		if err := data.Writeln(entry.Code + " " + entry.Path); err != nil {
			return err
		}
	}
	return nil
}

// runStatusAll reports status for all configured repositories concurrently.
func runStatusAll(ctx context.Context) error {
	defer perf.Track(nil, "git.runStatusAll")()

	cfg := gitConfig()
	if cfg == nil || len(cfg.Repositories) == 0 {
		ui.Info("No repositories configured under git.repositories.")
		return nil
	}

	names := atmosgit.ConfiguredRepositoryNames(cfg)
	return runConcurrent(ctx, names, runStatusOne)
}

func init() {
	statusParser.RegisterFlags(statusCmd)
	if err := statusParser.BindToViper(viper.GetViper()); err != nil {
		panic(fmt.Sprintf("git status: BindToViper: %v", err))
	}
}
