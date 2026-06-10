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

// pushParser handles flag parsing for `atmos git push`.
var pushParser = newPushParser()

// pushCmd is the `atmos git push` subcommand.
var pushCmd = &cobra.Command{
	Use:   "push <name-or-path>",
	Short: "Push commits to a remote Git repository",
	Long: `Push the current branch to the configured remote for a named repository
(configured under git.repositories) or a filesystem path. Atmos never force-pushes.

On non-fast-forward rejection, push will retry (pull --ff-only, then re-push)
up to push.retries times (default 3). Use --dry-run to report what would be
pushed without pushing.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.push.RunE")()

		v := viper.GetViper()
		if err := pushParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		branch := v.GetString(flagBranch)
		remote := v.GetString(flagRemote)
		dryRun := v.GetBool(flagDryRun)
		return runPush(cmd.Context(), args[0], branch, remote, dryRun)
	},
}

// runPush orchestrates the push subcommand.
func runPush(ctx context.Context, arg, branch, remote string, dryRun bool) error {
	defer perf.Track(nil, "git.runPush")()

	cfg := gitConfig()
	kind := classifyArg(arg, cfg)

	var workdir, identity, effectiveRemote, effectiveBranch string
	var retries int

	if kind == argKindName {
		resolved, err := resolveRepoByName(arg, cfg)
		if err != nil {
			return wrapRepoNotFound(err, arg)
		}
		workdir = resolved.Workdir
		identity = resolved.Identity
		effectiveRemote = resolveStringPrecedence(remote, resolved.Remote)
		effectiveBranch = resolveStringPrecedence(branch, resolved.Branch)
		retries = resolved.PushRetries
	} else {
		if kind == argKindURI {
			return errUtils.Build(errUtils.ErrGitRepositoryRequired).
				WithHint("'atmos git push' requires a repository name or path, not a URI.").
				WithExitCode(2).
				Err()
		}
		workdir = arg
		effectiveRemote = resolveStringPrecedence(remote, atmosgit.DefaultRemote)
		effectiveBranch = branch
		retries = atmosgit.DefaultPushRetries
	}

	if dryRun {
		ui.Infof("[dry-run] Would push branch %q to remote %q in %s.", effectiveBranch, effectiveRemote, workdir)
		return nil
	}

	env, err := composeEnv(ctx, identity)
	if err != nil {
		return err
	}

	exec, err := providerForName("")
	if err != nil {
		return err
	}

	return exec.Push(ctx, &atmosgit.PushOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: workdir,
			Remote:  effectiveRemote,
			Branch:  effectiveBranch,
			Env:     env,
		},
		Retries: retries,
	})
}

func init() {
	pushParser.RegisterFlags(pushCmd)
	if err := pushParser.BindToViper(viper.GetViper()); err != nil {
		panic(fmt.Sprintf("git push: BindToViper: %v", err))
	}
}
