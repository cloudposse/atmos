package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/flags"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"

	// Register CI providers for detection in no-arg clone.
	_ "github.com/cloudposse/atmos/pkg/ci/providers/github"
)

// cloneParser handles flag parsing for `atmos git clone`.
var cloneParser = newCloneParser()

// cloneCmd is the `atmos git clone` subcommand.
var cloneCmd = &cobra.Command{
	Use:   "clone [name-or-uri]",
	Short: "Clone or reconcile a managed Git repository",
	Long: `Clone a named Git repository (configured under git.repositories) or an ad hoc URI.

When no argument is provided, Atmos clones the single configured repository.
When ci.enabled is true and a CI provider is detected, the current CI repository
is cloned into the working directory instead (replaces actions/checkout).

URI forms supported:
  - Named repository:   atmos git clone flux-deploy
  - HTTPS URI:          atmos git clone https://github.com/acme/repo.git
  - SCP-style:          atmos git clone git@github.com:acme/repo.git
  - go-getter style:    atmos git clone git::https://github.com/acme/repo.git?ref=main&depth=1
  - Single configured:  atmos git clone
  - Bulk operation:     atmos git clone --all

Arguments after -- are passed verbatim to the underlying git clone invocation:
  atmos git clone flux-deploy -- --no-tags`,
	Args:              flags.SeparatorAwareValidator(cobra.MaximumNArgs(1)),
	ValidArgsFunction: completeRepoNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.clone.RunE")()

		v := viper.GetViper()
		if err := cloneParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		positional, nativeArgs := flags.SplitArgsAtDash(cmd, args)
		opts := parseCloneFlags(v)
		opts.ExtraArgs = nativeArgs
		return runClone(cmd.Context(), opts, positional)
	},
}

// cloneOptions holds parsed flags for the clone subcommand.
type cloneOptions struct {
	RepoURI      string
	Branch       string
	Remote       string
	Workdir      string
	Filter       string
	Depth        int
	SingleBranch bool
	Submodules   bool
	All          bool
	// ExtraArgs are native git arguments captured after "--" on the command line.
	ExtraArgs []string
}

func parseCloneFlags(v *viper.Viper) *cloneOptions {
	return &cloneOptions{
		RepoURI:      v.GetString(viperKey(cloneViperPrefix, flagRepoURI)),
		Branch:       v.GetString(viperKey(cloneViperPrefix, flagBranch)),
		Remote:       v.GetString(viperKey(cloneViperPrefix, flagRemote)),
		Workdir:      v.GetString(viperKey(cloneViperPrefix, flagWorkdir)),
		Depth:        v.GetInt(viperKey(cloneViperPrefix, flagDepth)),
		Filter:       v.GetString(viperKey(cloneViperPrefix, flagFilter)),
		SingleBranch: v.GetBool(viperKey(cloneViperPrefix, flagSingleBr)),
		Submodules:   v.GetBool(viperKey(cloneViperPrefix, flagSubmodules)),
		All:          v.GetBool(viperKey(cloneViperPrefix, flagAll)),
	}
}

// runClone orchestrates the clone subcommand.
func runClone(ctx context.Context, opts *cloneOptions, args []string) error {
	defer perf.Track(nil, "git.runClone")()

	if opts.All && len(args) > 0 {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHint("--all is mutually exclusive with a positional repository name.").
			WithExitCode(2).
			Err()
	}

	cfg := gitConfig()

	if opts.All {
		return runCloneAll(ctx, opts)
	}

	if len(args) == 0 {
		return runCloneNoArg(ctx, opts)
	}

	arg := args[0]
	kind := classifyArg(arg, cfg)

	switch kind {
	case argKindName:
		return runCloneNamed(ctx, arg, opts)
	case argKindURI:
		return runCloneURI(ctx, arg, opts)
	default:
		// Path does not make sense for clone.
		return errUtils.Build(errUtils.ErrGitRepositoryNotFound).
			WithHintf("Repository %q is not configured under git.repositories and is not a valid URI.", arg).
			WithHint("Run 'atmos git list' to see configured repositories, or provide a full URI.").
			WithExitCode(2).
			Err()
	}
}

// runCloneNamed clones a named managed repository.
func runCloneNamed(ctx context.Context, name string, opts *cloneOptions) error {
	defer perf.Track(nil, "git.runCloneNamed")()

	cfg := gitConfig()
	resolved, err := resolveRepoByName(name, cfg)
	if err != nil {
		return wrapRepoNotFound(err, name)
	}

	env, err := composeEnv(ctx, resolved.Identity)
	if err != nil {
		return err
	}

	exec, err := providerForName(resolved.Provider)
	if err != nil {
		return err
	}

	workdir := resolveWorkdir(opts.Workdir, resolved.Workdir)
	branch := resolveStringPrecedence(opts.Branch, resolved.Branch)
	remote := resolveStringPrecedence(opts.Remote, resolved.Remote)
	depth := resolveIntPrecedence(opts.Depth, resolved.Clone.Depth)
	filter := resolveStringPrecedence(opts.Filter, resolved.Clone.Filter)

	cloneOpts := &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: workdir,
			Remote:  remote,
			Branch:  branch,
			Env:     env,
		},
		URI:          resolved.URI,
		Depth:        depth,
		Filter:       filter,
		SingleBranch: opts.SingleBranch || resolved.Clone.SingleBranch,
		Submodules:   opts.Submodules || resolved.Clone.Submodules,
		ExtraArgs:    opts.ExtraArgs,
	}

	if opts.All {
		return exec.CloneWithoutSpinner(ctx, cloneOpts, name)
	}
	return exec.Clone(ctx, cloneOpts, name)
}

// runCloneURI clones an ad hoc URI (not a configured repository).
func runCloneURI(ctx context.Context, rawURI string, opts *cloneOptions) error {
	defer perf.Track(nil, "git.runCloneURI")()

	parsed, err := ParseCloneURI(rawURI)
	if err != nil {
		return err
	}

	// Flag precedence: flag > query param > 0/empty.
	branch := resolveStringPrecedence(opts.Branch, parsed.Branch)
	depth := resolveIntPrecedence(opts.Depth, parsed.Depth)

	// Ad hoc clones go to <cwd>/<repo-name>; --workdir overrides.
	workdir, err := resolveAdHocWorkdir(opts.Workdir, parsed.RepoName)
	if err != nil {
		return err
	}

	exec, err := providerForName("")
	if err != nil {
		return err
	}

	env, err := composeEnv(ctx, "")
	if err != nil {
		return err
	}

	cloneOpts := &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: workdir,
			Remote:  resolveStringPrecedence(opts.Remote, atmosgit.DefaultRemote),
			Branch:  branch,
			Env:     env,
		},
		URI:          parsed.URI,
		Depth:        depth,
		Filter:       opts.Filter,
		SingleBranch: opts.SingleBranch,
		Submodules:   opts.Submodules,
		ExtraArgs:    opts.ExtraArgs,
	}

	return exec.Clone(ctx, cloneOpts, parsed.URI)
}

// resolveAdHocWorkdir returns workdirFlag if non-empty, otherwise <cwd>/<repoName>.
func resolveAdHocWorkdir(workdirFlag, repoName string) (string, error) {
	if workdirFlag != "" {
		return workdirFlag, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting current directory: %w", err)
	}
	return filepath.Join(cwd, repoName), nil
}

// runCloneNoArg handles the no-argument form: CI current-repository checkout replacement.
func runCloneNoArg(ctx context.Context, opts *cloneOptions) error {
	defer perf.Track(nil, "git.runCloneNoArg")()

	if isCICloneEnabled() {
		detected := ci.Detect()
		if detected != nil {
			ciCtx, err := detected.Context()
			if err != nil {
				return fmt.Errorf("reading CI context for no-arg clone: %w", err)
			}
			return runCICheckout(ctx, detected.Name(), ciCtx, opts)
		}
	}

	if name, ok := singleConfiguredRepositoryName(gitConfig()); ok {
		return runCloneNamed(ctx, name, opts)
	}

	return errUtils.Build(errUtils.ErrGitRepositoryRequired).
		WithHint("Provide a repository name or URI to clone.").
		WithHint("Use 'atmos git clone --all' to clone all configured repositories.").
		WithHint("When exactly one repository is configured under git.repositories, no-arg clone uses that repository.").
		WithExitCode(2).
		Err()
}

func isCICloneEnabled() bool {
	return atmosConfigPtr != nil && atmosConfigPtr.CI.Enabled
}

func singleConfiguredRepositoryName(cfg *schema.GitConfig) (string, bool) {
	names := atmosgit.ConfiguredRepositoryNames(cfg)
	if len(names) != 1 {
		return "", false
	}
	return names[0], true
}

// runCICheckout performs the no-arg CI current-repository checkout. The clone
// URL comes from the CI provider's Context (each provider constructs it from
// its own metadata — e.g. GITHUB_SERVER_URL for GitHub Enterprise); cmd/git
// never assumes a host.
func runCICheckout(ctx context.Context, providerName string, ciCtx *ci.Context, opts *cloneOptions) error {
	defer perf.Track(nil, "git.runCICheckout")()

	if ciCtx.CloneURL == "" {
		return errUtils.Build(errUtils.ErrGitRepositoryRequired).
			WithHintf("CI provider %q did not supply a clone URL for the current repository.", providerName).
			WithHint("Provide a repository name or URI to clone explicitly.").
			WithExitCode(2).
			Err()
	}
	repository := ciCtx.Repository

	workdir, err := resolveCIWorkdir(opts.Workdir)
	if err != nil {
		return err
	}

	env, err := composeEnv(ctx, "")
	if err != nil {
		return err
	}

	exec, err := providerForName("")
	if err != nil {
		return err
	}

	cloneOpts := &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: workdir,
			Remote:  resolveStringPrecedence(opts.Remote, atmosgit.DefaultRemote),
			Branch:  resolveStringPrecedence(opts.Branch, ciCtx.Ref),
			Env:     env,
		},
		URI:       ciCtx.CloneURL,
		Depth:     opts.Depth,
		ExtraArgs: opts.ExtraArgs,
	}

	if err := exec.Clone(ctx, cloneOpts, repository); err != nil {
		return err
	}

	ui.Successf("Checked out CI repository %q into %s (provider: %s).", repository, workdir, providerName)
	return nil
}

// resolveCIWorkdir returns workdirFlag if non-empty, otherwise the process cwd.
func resolveCIWorkdir(workdirFlag string) (string, error) {
	if workdirFlag != "" {
		return workdirFlag, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory for CI checkout: %w", err)
	}
	return cwd, nil
}

// runCloneAll clones/reconciles all configured repositories concurrently.
func runCloneAll(ctx context.Context, opts *cloneOptions) error {
	defer perf.Track(nil, "git.runCloneAll")()

	cfg := gitConfig()
	if cfg == nil || len(cfg.Repositories) == 0 {
		ui.Info("No repositories configured under git.repositories.")
		return nil
	}

	names := atmosgit.ConfiguredRepositoryNames(cfg)
	return runConcurrent(ctx, names, func(ctx context.Context, name string) error {
		return runCloneNamed(ctx, name, opts)
	})
}

// runConcurrent runs f for each name with bounded concurrency (4 workers)
// and collects all errors via errors.Join (attempt-all, not fail-fast).
func runConcurrent(ctx context.Context, names []string, f func(context.Context, string) error) error {
	const workers = 4
	sem := make(chan struct{}, workers)
	var mu sync.Mutex
	var errs []error
	var wg sync.WaitGroup

	for _, name := range names {
		wg.Add(1)
		sem <- struct{}{}
		go func(n string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := f(ctx, n); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("repository %q: %w", n, err))
				mu.Unlock()
			}
		}(name)
	}
	wg.Wait()

	return errors.Join(errs...)
}

func init() {
	cloneParser.RegisterFlags(cloneCmd)
	if err := cloneParser.BindToViper(viper.GetViper()); err != nil {
		panic(fmt.Sprintf("git clone: BindToViper: %v", err))
	}
}
