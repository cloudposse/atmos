package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
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

type stderrSwapper interface {
	SwapStderr(io.Writer) func()
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

	progressMsg := fmt.Sprintf("Cloning %s", label)
	completedMsg := fmt.Sprintf("Cloned %s into %s.", label, opts.Workdir)
	stderr, err := e.captureStderr(func() error {
		return spinner.ExecWithSpinner(progressMsg, completedMsg, func() error {
			return e.provider.Clone(ctx, opts)
		})
	})
	return wrapCloneError(label, opts.Workdir, stderr, err)
}

func (e *Executor) CloneWithoutSpinner(ctx context.Context, opts *atmosgit.CloneOptions, label string) error {
	defer perf.Track(nil, "git.Executor.CloneWithoutSpinner")()

	stderr, err := e.captureStderr(func() error {
		return e.provider.Clone(ctx, opts)
	})
	if err != nil {
		return wrapCloneError(label, opts.Workdir, stderr, err)
	}

	ui.Successf("Cloned %s into %s.", label, opts.Workdir)
	return nil
}

func (e *Executor) captureStderr(operation func() error) (string, error) {
	swapper, ok := e.provider.(stderrSwapper)
	if !ok {
		return "", operation()
	}

	var stderr bytes.Buffer
	restore := swapper.SwapStderr(iolib.MaskWriter(&stderr))
	defer restore()

	err := operation()
	return strings.TrimSpace(stderr.String()), err
}

func wrapCloneError(label, workdir, stderr string, err error) error {
	if err == nil {
		return nil
	}

	base := errUtils.ErrGitCommandFailed
	wrapCause := true
	switch {
	case errors.Is(err, errUtils.ErrGitAuthFailed):
		base = errUtils.ErrGitAuthFailed
		wrapCause = false
	case errors.Is(err, errUtils.ErrGitCommandExited):
		base = errUtils.ErrGitCommandExited
		wrapCause = false
	}

	explanation := fmt.Sprintf("Failed to clone Git repository %q into %q.", label, workdir)
	if stderr != "" {
		explanation += "\n\nGit output:\n\n```text\n" + stderr + "\n```"
	}

	builder := errUtils.Build(base)
	if wrapCause {
		builder = builder.WithCause(err)
	}

	return builder.
		WithExplanation(explanation).
		WithHint("Run 'atmos git list' to confirm the configured URI, branch, and resolved workdir.").
		WithExitCode(2).
		Err()
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
