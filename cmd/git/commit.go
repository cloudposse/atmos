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

// commitParser handles flag parsing for `atmos git commit`.
var commitParser = newCommitParser()

// commitCmd is the `atmos git commit` subcommand.
var commitCmd = &cobra.Command{
	Use:   "commit <name-or-path>",
	Short: "Stage managed paths and create a commit in a managed Git repository",
	Long: `Stage the specified paths (--path) and commit the changes in a named repository
(configured under git.repositories) or a filesystem path.

When --path is provided, only those paths are staged; dirty files outside the
managed paths fail the commit. When --dry-run is set, the command reports what
would be staged and committed without actually committing.

--sign and --no-sign are mutually exclusive.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.commit.RunE")()

		v := viper.GetViper()
		if err := commitParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := parseCommitFlags(v)
		return runCommit(cmd.Context(), args[0], opts)
	},
}

// commitOptions holds parsed flags for the commit subcommand.
type commitOptions struct {
	Message string
	Paths   []string
	Sign    bool
	NoSign  bool
	DryRun  bool
}

func parseCommitFlags(v *viper.Viper) *commitOptions {
	return &commitOptions{
		Message: v.GetString(flagMessage),
		Paths:   v.GetStringSlice(flagPath),
		Sign:    v.GetBool(flagSign),
		NoSign:  v.GetBool(flagNoSign),
		DryRun:  v.GetBool(flagDryRun),
	}
}

// runCommit orchestrates the commit subcommand.
func runCommit(ctx context.Context, arg string, opts *commitOptions) error {
	defer perf.Track(nil, "git.runCommit")()

	if opts.Message == "" && !opts.DryRun {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithHint("Provide a commit message with --message=<msg>.").
			WithExitCode(2).
			Err()
	}

	cfg := gitConfig()
	kind := classifyArg(arg, cfg)

	var workdir, identity string
	var resolved *atmosgit.ResolvedRepository

	if kind == argKindName {
		var err error
		resolved, err = resolveRepoByName(arg, cfg)
		if err != nil {
			return wrapRepoNotFound(err, arg)
		}
		workdir = resolved.Workdir
		identity = resolved.Identity
	} else {
		workdir = arg
	}

	if opts.DryRun {
		return runCommitDryRun(ctx, workdir, identity, opts)
	}

	return executeCommit(ctx, workdir, identity, resolved, opts)
}

// executeCommit performs the actual commit after validation.
func executeCommit(ctx context.Context, workdir, identity string, resolved *atmosgit.ResolvedRepository, opts *commitOptions) error {
	defer perf.Track(nil, "git.executeCommit")()

	env, err := composeEnv(ctx, identity)
	if err != nil {
		return err
	}

	exec, err := providerForName("")
	if err != nil {
		return err
	}

	return executeCommitWithResult(ctx, exec, &atmosgit.CommitOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: workdir,
			Env:     env,
		},
		Message: opts.Message,
		Paths:   opts.Paths,
		Signing: resolveSigningMode(opts.Sign, opts.NoSign, resolved),
		Author:  resolveAuthorFromResolved(resolved),
	})
}

// runCommitDryRun reports what would be staged/committed without committing.
func runCommitDryRun(ctx context.Context, workdir, identity string, opts *commitOptions) error {
	defer perf.Track(nil, "git.runCommitDryRun")()

	env, err := composeEnv(ctx, identity)
	if err != nil {
		return err
	}

	exec, err := providerForName("")
	if err != nil {
		return err
	}

	result, err := exec.Status(ctx, &atmosgit.StatusOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: workdir,
			Env:     env,
		},
		Paths: opts.Paths,
	})
	if err != nil {
		return err
	}

	if result.Clean {
		ui.Info("[dry-run] Nothing to commit; working tree is clean.")
		return nil
	}

	ui.Info("[dry-run] The following changes would be committed:")
	for _, entry := range result.Entries {
		ui.Infof("  %s %s", entry.Code, entry.Path)
	}

	return nil
}

// resolveSigningMode maps the --sign/--no-sign flags and repository config
// to a SigningMode value.
func resolveSigningMode(sign, noSign bool, resolved *atmosgit.ResolvedRepository) atmosgit.SigningMode {
	if sign {
		return atmosgit.SigningAlways
	}
	if noSign {
		return atmosgit.SigningNever
	}
	if resolved != nil {
		return resolved.Signing
	}
	return atmosgit.SigningAuto
}

// resolveAuthorFromResolved returns the Author from a resolved repository, or nil.
func resolveAuthorFromResolved(resolved *atmosgit.ResolvedRepository) *atmosgit.Author {
	if resolved == nil {
		return nil
	}
	return resolved.Author
}

func init() {
	commitParser.RegisterFlags(commitCmd)
	// Mark --sign and --no-sign as mutually exclusive per PRD safety rules.
	commitCmd.MarkFlagsMutuallyExclusive(flagSign, flagNoSign)
	if err := commitParser.BindToViper(viper.GetViper()); err != nil {
		panic(fmt.Sprintf("git commit: BindToViper: %v", err))
	}
}
