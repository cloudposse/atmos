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

// diffParser handles flag parsing for `atmos git diff`.
var diffParser = newDiffParser()

// diffCmd is the `atmos git diff` subcommand.
var diffCmd = &cobra.Command{
	Use:   "diff <name-or-path>",
	Short: "Show changes between working tree and HEAD in a managed Git repository",
	Long: `Show uncommitted changes for a named repository (configured under git.repositories)
or a filesystem path. The unified diff is written to stdout (pipeable).

This is the read-before-write step for GitOps publishing: it shows what would
be committed without actually committing. Use --path to scope to specific paths.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeRepoNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.diff.RunE")()

		v := viper.GetViper()
		if err := diffParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		paths := v.GetStringSlice(viperKey(diffViperPrefix, flagPath))
		return runDiff(cmd.Context(), args[0], paths)
	},
}

// runDiff shows the diff for a repository by name or path.
func runDiff(ctx context.Context, arg string, paths []string) error {
	defer perf.Track(nil, "git.runDiff")()

	cfg := gitConfig()
	kind := classifyArg(arg, cfg)

	if kind == argKindURI {
		return errUtils.Build(errUtils.ErrGitRepositoryRequired).
			WithHint("'atmos git diff' requires a repository name or path, not a URI. Use a configured repository name or a local path.").
			WithExitCode(2).
			Err()
	}

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

	return executeDiffAndPrint(ctx, exec, workdir, env, paths)
}

// printDiff writes diff output to stdout (pipeable) and untracked file
// notifications to stderr.
func printDiff(workdir string, result *atmosgit.DiffResult) error {
	if !result.HasChanges {
		ui.Infof("Repository at %s has no changes.", workdir)
		return nil
	}

	if result.Output != "" {
		if err := data.Write(result.Output); err != nil {
			return err
		}
	}

	for _, f := range result.Untracked {
		ui.Infof("Untracked: %s", f)
	}

	return nil
}

func init() {
	diffParser.RegisterFlags(diffCmd)
	if err := diffParser.BindToViper(viper.GetViper()); err != nil {
		panic(fmt.Sprintf("git diff: BindToViper: %v", err))
	}
}
