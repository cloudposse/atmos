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

// initParser handles flag parsing for `atmos git init`.
var initParser = newInitParser()

// initCmd is the `atmos git init` subcommand.
var initCmd = &cobra.Command{
	Use:   "init [name]",
	Short: "Initialize a managed Git repository from scratch",
	Long: `Initialize a named repository (configured under git.repositories) whose remote
has no content yet — the inverse of clone.

Without --from, an empty repository is created at the resolved workdir on the
configured branch, with the configured remote wired up, ready for
'atmos git commit' and 'atmos git push'.

With --from=<uri>, the new repository is seeded from another repository:
  - Default (fresh history): the source content is imported as a single fresh
    initial commit; no link to the source remains.
  - With --keep-history: the source's full history is preserved and the source
    stays reachable as the 'upstream' remote, so future updates can be pulled.

When no argument is provided, Atmos initializes the single configured repository.

Arguments after -- are passed verbatim to the underlying git invocation
(git init, or the git clone of --from):
  atmos git init flux-deploy --from=https://github.com/acme/template.git -- --no-tags`,
	Args:              flags.SeparatorAwareValidator(cobra.MaximumNArgs(1)),
	ValidArgsFunction: completeRepoNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.init.RunE")()

		v := viper.GetViper()
		if err := initParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		positional, nativeArgs := flags.SplitArgsAtDash(cmd, args)
		opts := parseInitFlags(v)
		opts.ExtraArgs = nativeArgs
		return runInit(cmd.Context(), opts, positional)
	},
}

// initOptions holds parsed flags for the init subcommand.
type initOptions struct {
	From        string
	KeepHistory bool
	Branch      string
	Workdir     string
	DryRun      bool
	// ExtraArgs are native git arguments captured after "--" on the command line.
	ExtraArgs []string
}

func parseInitFlags(v *viper.Viper) *initOptions {
	return &initOptions{
		From:        v.GetString(viperKey(initViperPrefix, flagFrom)),
		KeepHistory: v.GetBool(viperKey(initViperPrefix, flagKeepHist)),
		Branch:      v.GetString(viperKey(initViperPrefix, flagBranch)),
		Workdir:     v.GetString(viperKey(initViperPrefix, flagWorkdir)),
		DryRun:      v.GetBool(viperKey(initViperPrefix, flagDryRun)),
	}
}

// runInit orchestrates the init subcommand.
func runInit(ctx context.Context, opts *initOptions, args []string) error {
	defer perf.Track(nil, "git.runInit")()

	if opts.KeepHistory && opts.From == "" {
		return errUtils.Build(errUtils.ErrInvalidFlag).
			WithHint("--keep-history only applies when seeding from another repository; pass --from=<uri> as well.").
			WithExitCode(2).
			Err()
	}

	name, err := resolveInitName(args)
	if err != nil {
		return err
	}

	cfg := gitConfig()
	resolved, err := resolveRepoByName(name, cfg)
	if err != nil {
		return wrapRepoNotFound(err, name)
	}
	if resolved.URI == "" {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("Repository %q has no uri configured under git.repositories; init needs it to wire up the remote.", name).
			WithExitCode(2).
			Err()
	}

	workdir := resolveWorkdir(opts.Workdir, resolved.Workdir)
	branch := resolveStringPrecedence(opts.Branch, resolved.Branch)

	if opts.DryRun {
		reportInitDryRun(name, workdir, branch, resolved.URI, opts)
		return nil
	}

	env, err := composeEnv(ctx, resolved.Identity)
	if err != nil {
		return err
	}

	exec, err := providerForName(resolved.Provider)
	if err != nil {
		return err
	}

	return exec.Init(ctx, &atmosgit.InitOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: workdir,
			Remote:  resolved.Remote,
			Branch:  branch,
			Env:     env,
		},
		URI:         resolved.URI,
		FromURI:     opts.From,
		KeepHistory: opts.KeepHistory,
		Signing:     resolved.Signing,
		Author:      resolved.Author,
		ExtraArgs:   opts.ExtraArgs,
	}, name)
}

// resolveInitName returns the positional repository name, falling back to the
// single configured repository when no argument is given.
func resolveInitName(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if name, ok := singleConfiguredRepositoryName(gitConfig()); ok {
		return name, nil
	}
	return "", errUtils.Build(errUtils.ErrGitRepositoryRequired).
		WithHint("Provide a repository name to initialize.").
		WithHint("When exactly one repository is configured under git.repositories, no-arg init uses that repository.").
		WithExitCode(2).
		Err()
}

// reportInitDryRun describes what init would do without doing it.
func reportInitDryRun(name, workdir, branch, uri string, opts *initOptions) {
	switch {
	case opts.From == "":
		ui.Infof("[dry-run] Would initialize empty repository %q at %s (branch %q, remote -> %s).", name, workdir, branch, uri)
	case opts.KeepHistory:
		ui.Infof("[dry-run] Would clone %s (full history) into %s, keep it pullable as the 'upstream' remote, and wire the configured remote to %s.", opts.From, workdir, uri)
	default:
		ui.Infof("[dry-run] Would import %s into %s as a single fresh initial commit (branch %q, remote -> %s).", opts.From, workdir, branch, uri)
	}
}

func init() {
	initParser.RegisterFlags(initCmd)
	if err := initParser.BindToViper(viper.GetViper()); err != nil {
		panic(fmt.Sprintf("git init: BindToViper: %v", err))
	}
}
