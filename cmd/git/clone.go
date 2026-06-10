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
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
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

When no argument is provided and ci.enabled is true, the current CI repository
is cloned into the working directory (replaces actions/checkout).

URI forms supported:
  - Named repository:   atmos git clone flux-deploy
  - HTTPS URI:          atmos git clone https://github.com/acme/repo.git
  - SCP-style:          atmos git clone git@github.com:acme/repo.git
  - go-getter style:    atmos git clone git::https://github.com/acme/repo.git?ref=main&depth=1
  - Bulk operation:     atmos git clone --all`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.clone.RunE")()

		v := viper.GetViper()
		if err := cloneParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := parseCloneFlags(v)
		return runClone(cmd.Context(), opts, args)
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
}

func parseCloneFlags(v *viper.Viper) *cloneOptions {
	return &cloneOptions{
		RepoURI:      v.GetString(flagRepoURI),
		Branch:       v.GetString(flagBranch),
		Remote:       v.GetString(flagRemote),
		Workdir:      v.GetString(flagWorkdir),
		Depth:        v.GetInt(flagDepth),
		Filter:       v.GetString(flagFilter),
		SingleBranch: v.GetBool(flagSingleBr),
		Submodules:   v.GetBool(flagSubmodules),
		All:          v.GetBool(flagAll),
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

	if err := checkCIEnabled(); err != nil {
		return err
	}

	detected := ci.Detect()
	if detected == nil {
		return errUtils.Build(errUtils.ErrGitRepositoryRequired).
			WithHint("No CI provider was detected in the current environment.").
			WithHint("Provide a repository name or URI to clone.").
			WithExitCode(2).
			Err()
	}

	ciCtx, err := detected.Context()
	if err != nil {
		return fmt.Errorf("reading CI context for no-arg clone: %w", err)
	}

	return runCICheckout(ctx, detected.Name(), ciCtx.Repository, ciCtx.Ref, opts)
}

// checkCIEnabled returns an error when ci.enabled is false or config is nil.
func checkCIEnabled() error {
	if atmosConfigPtr == nil || !atmosConfigPtr.CI.Enabled {
		return errUtils.Build(errUtils.ErrGitRepositoryRequired).
			WithHint("Provide a repository name or URI to clone.").
			WithHint("Set 'ci.enabled: true' in atmos.yaml to enable CI current-repository checkout replacement.").
			WithExitCode(2).
			Err()
	}
	return nil
}

// runCICheckout performs the no-arg CI current-repository checkout.
func runCICheckout(ctx context.Context, providerName, repository, ref string, opts *cloneOptions) error {
	defer perf.Track(nil, "git.runCICheckout")()

	if repository == "" {
		return errUtils.Build(errUtils.ErrGitRepositoryRequired).
			WithHint("CI provider did not supply a repository name. Provide a URI or configured name.").
			WithExitCode(2).
			Err()
	}

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
			Branch:  resolveStringPrecedence(opts.Branch, ref),
			Env:     env,
		},
		URI:   ciRepoURI(repository),
		Depth: opts.Depth,
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

// ciRepoURI constructs a clone URI from a "owner/repo" CI repository string.
// Using https:// so ambient credentials (GITHUB_TOKEN via git config) apply.
func ciRepoURI(repo string) string {
	return "https://github.com/" + repo + ".git"
}

func init() {
	cloneParser.RegisterFlags(cloneCmd)
	if err := cloneParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
