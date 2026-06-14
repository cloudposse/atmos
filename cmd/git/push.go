package git

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
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
pushed without pushing.

Arguments after -- are passed verbatim to the underlying git push invocation:
  atmos git push flux-deploy -- --follow-tags`,
	Args:              flags.SeparatorAwareValidator(cobra.ExactArgs(1)),
	ValidArgsFunction: completeRepoNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.push.RunE")()

		v := viper.GetViper()
		if err := pushParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		positional, nativeArgs := flags.SplitArgsAtDash(cmd, args)
		opts := &pushOptions{
			Branch:    v.GetString(viperKey(pushViperPrefix, flagBranch)),
			Remote:    v.GetString(viperKey(pushViperPrefix, flagRemote)),
			DryRun:    v.GetBool(viperKey(pushViperPrefix, flagDryRun)),
			ExtraArgs: nativeArgs,
		}
		return runPush(cmd.Context(), positional[0], opts)
	},
}

// pushOptions holds parsed flags for the push subcommand.
type pushOptions struct {
	Branch string
	Remote string
	DryRun bool
	// ExtraArgs are native git arguments captured after "--" on the command line.
	ExtraArgs []string
}

// runPush orchestrates the push subcommand.
func runPush(ctx context.Context, arg string, opts *pushOptions) error {
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
		effectiveRemote = resolveStringPrecedence(opts.Remote, resolved.Remote)
		effectiveBranch = resolveStringPrecedence(opts.Branch, resolved.Branch)
		retries = resolved.PushRetries
	} else {
		if kind == argKindURI {
			return errUtils.Build(errUtils.ErrGitRepositoryRequired).
				WithHint("'atmos git push' requires a repository name or path, not a URI.").
				WithExitCode(2).
				Err()
		}
		workdir = arg
		effectiveRemote = resolveStringPrecedence(opts.Remote, atmosgit.DefaultRemote)
		effectiveBranch = opts.Branch
		retries = atmosgit.DefaultPushRetries
	}

	if opts.DryRun {
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
		Retries:   retries,
		ExtraArgs: opts.ExtraArgs,
	})
}

func init() {
	pushParser.RegisterFlags(pushCmd)
	if err := pushParser.BindToViper(viper.GetViper()); err != nil {
		panic(fmt.Sprintf("git push: BindToViper: %v", err))
	}
}
