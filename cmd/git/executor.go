package git

import (
	"context"
	"fmt"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// testProviderOverride, when non-nil, is used by providerForName() instead of
// looking up the real registry. Set only from tests via setTestProvider().
var testProviderOverride atmosgit.Provider

// setTestProvider installs a test-double provider that all provider lookups
// will use instead of the real registry. Returns a cleanup function that
// restores the previous value.
func setTestProvider(p atmosgit.Provider) func() {
	prev := testProviderOverride
	testProviderOverride = p
	return func() { testProviderOverride = prev }
}

// Executor holds the resolved inputs for a single Git operation and delegates
// to an injected Provider. This enables unit testing without invoking real git
// subprocesses: tests pass a stub provider; production passes the real one.
type Executor struct {
	provider atmosgit.Provider
}

// newExecutor builds an Executor using the named provider from the registry.
// Pass an empty string to use the default "cli" provider.
func newExecutor(providerName string) (*Executor, error) {
	p, err := atmosgit.NewProvider(providerName)
	if err != nil {
		return nil, err
	}
	return &Executor{provider: p}, nil
}

// newExecutorWithProvider builds an Executor using an already-constructed
// Provider (used in tests).
func newExecutorWithProvider(p atmosgit.Provider) *Executor {
	return &Executor{provider: p}
}

// Clone delegates to the provider.
func (e *Executor) Clone(ctx context.Context, opts *atmosgit.CloneOptions, label string) error {
	defer perf.Track(nil, "git.Executor.Clone")()

	if err := e.provider.Clone(ctx, opts); err != nil {
		return err
	}

	ui.Successf("Cloned %s into %s.", label, opts.Workdir)
	return nil
}

// Pull delegates to the provider.
func (e *Executor) Pull(ctx context.Context, opts *atmosgit.PullOptions) error {
	defer perf.Track(nil, "git.Executor.Pull")()

	if err := e.provider.Pull(ctx, opts); err != nil {
		return err
	}

	ui.Successf("Pulled repository at %s.", opts.Workdir)
	return nil
}

// Status delegates to the provider and prints the result.
func (e *Executor) Status(ctx context.Context, opts *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
	defer perf.Track(nil, "git.Executor.Status")()

	return e.provider.Status(ctx, opts)
}

// Diff delegates to the provider.
func (e *Executor) Diff(ctx context.Context, opts *atmosgit.DiffOptions) (*atmosgit.DiffResult, error) {
	defer perf.Track(nil, "git.Executor.Diff")()

	return e.provider.Diff(ctx, opts)
}

// Commit delegates to the provider.
func (e *Executor) Commit(ctx context.Context, opts *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
	defer perf.Track(nil, "git.Executor.Commit")()

	return e.provider.Commit(ctx, opts)
}

// Push delegates to the provider.
func (e *Executor) Push(ctx context.Context, opts *atmosgit.PushOptions) error {
	defer perf.Track(nil, "git.Executor.Push")()

	if err := e.provider.Push(ctx, opts); err != nil {
		return err
	}

	ui.Successf("Pushed %s to %s/%s.", opts.Workdir, opts.Remote, opts.Branch)
	return nil
}

// executeStatusAndPrint runs status and prints results.
func executeStatusAndPrint(ctx context.Context, exec *Executor, workdir string, env []string) error {
	defer perf.Track(nil, "git.executeStatusAndPrint")()

	result, err := exec.Status(ctx, &atmosgit.StatusOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: workdir,
			Env:     env,
		},
	})
	if err != nil {
		return err
	}

	return printStatus(workdir, result)
}

// executeDiffAndPrint runs diff and prints results.
func executeDiffAndPrint(ctx context.Context, exec *Executor, workdir string, env, paths []string) error {
	defer perf.Track(nil, "git.executeDiffAndPrint")()

	result, err := exec.Diff(ctx, &atmosgit.DiffOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: workdir,
			Env:     env,
		},
		Paths: paths,
	})
	if err != nil {
		return err
	}

	return printDiff(workdir, result)
}

// executeCommitWithResult runs commit and reports outcome.
func executeCommitWithResult(ctx context.Context, exec *Executor, opts *atmosgit.CommitOptions) error {
	defer perf.Track(nil, "git.executeCommitWithResult")()

	result, err := exec.Commit(ctx, opts)
	if err != nil {
		return err
	}

	if !result.Committed {
		ui.Info("Nothing to commit; working tree is clean.")
		return nil
	}

	ui.Successf("Committed %s in %s.", result.SHA, opts.Workdir)
	return nil
}

// buildRepoContext assembles a RepoContext from resolved values and composed env.
func buildRepoContext(workdir, remote, branch string, env []string) atmosgit.RepoContext {
	return atmosgit.RepoContext{
		Workdir: workdir,
		Remote:  remote,
		Branch:  branch,
		Env:     env,
	}
}

// providerForName looks up the named provider from the registry and wraps it
// in an Executor. Pass an empty string to use the default "cli" provider.
// When testProviderOverride is non-nil (set via setTestProvider in tests), it
// is returned directly without consulting the registry, allowing unit tests to
// run without a real Git subprocess.
func providerForName(name string) (*Executor, error) {
	if testProviderOverride != nil {
		return newExecutorWithProvider(testProviderOverride), nil
	}

	exec, err := newExecutor(name)
	if err != nil {
		return nil, fmt.Errorf("initializing git provider %q: %w", name, err)
	}

	return exec, nil
}
