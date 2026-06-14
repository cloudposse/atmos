package git

import (
	"bytes"
	"context"
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

// Init delegates to the provider.
func (e *Executor) Init(ctx context.Context, opts *atmosgit.InitOptions, label string) error {
	defer perf.Track(nil, "git.Executor.Init")()

	progressMsg := fmt.Sprintf("Initializing %s", label)
	completedMsg := fmt.Sprintf("Initialized %s in %s.", label, opts.Workdir)
	stderr, err := e.captureStderr(func() error {
		return spinner.ExecWithSpinner(progressMsg, completedMsg, func() error {
			return e.provider.Init(ctx, opts)
		})
	})
	return wrapGitOperationError(
		fmt.Sprintf("initialize Git repository %q", label),
		opts.Workdir,
		stderr,
		err,
		"Run 'atmos git list' to confirm the configured URI, branch, and resolved workdir.",
	)
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
	return wrapGitOperationError(
		fmt.Sprintf("clone Git repository %q", label),
		workdir,
		stderr,
		err,
		"Run 'atmos git list' to confirm the configured URI, branch, and resolved workdir.",
	)
}

func wrapGitOperationError(action, workdir, stderr string, err error, hint string) error {
	if err == nil {
		return nil
	}

	explanation := fmt.Sprintf("Failed to %s.", action)
	if workdir != "" {
		explanation = fmt.Sprintf("Failed to %s in %q.", action, workdir)
	}
	explanation += "\n\nUnderlying error:\n\n```text\n" + err.Error() + "\n```"
	if stderr != "" {
		explanation += "\n\nGit output:\n\n```text\n" + stderr + "\n```"
	}

	builder := errUtils.Build(err).
		WithExplanation(explanation).
		WithExitCode(2)
	if hint != "" {
		builder = builder.WithHint(hint)
	}
	return builder.Err()
}

// Pull delegates to the provider.
func (e *Executor) Pull(ctx context.Context, opts *atmosgit.PullOptions) error {
	defer perf.Track(nil, "git.Executor.Pull")()

	stderr, err := e.captureStderr(func() error {
		return e.provider.Pull(ctx, opts)
	})
	if err != nil {
		return wrapGitOperationError(
			"pull Git repository",
			opts.Workdir,
			stderr,
			err,
			"Run 'atmos git list' to confirm the configured branch and resolved workdir.",
		)
	}

	ui.Successf("Pulled repository at %s.", opts.Workdir)
	return nil
}

// Status delegates to the provider and prints the result.
func (e *Executor) Status(ctx context.Context, opts *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
	defer perf.Track(nil, "git.Executor.Status")()

	var result *atmosgit.StatusResult
	stderr, err := e.captureStderr(func() error {
		var opErr error
		result, opErr = e.provider.Status(ctx, opts)
		return opErr
	})
	if err != nil {
		return nil, wrapGitOperationError("read Git status", opts.Workdir, stderr, err, "")
	}
	return result, nil
}

// Diff delegates to the provider.
func (e *Executor) Diff(ctx context.Context, opts *atmosgit.DiffOptions) (*atmosgit.DiffResult, error) {
	defer perf.Track(nil, "git.Executor.Diff")()

	var result *atmosgit.DiffResult
	stderr, err := e.captureStderr(func() error {
		var opErr error
		result, opErr = e.provider.Diff(ctx, opts)
		return opErr
	})
	if err != nil {
		return nil, wrapGitOperationError("show Git diff", opts.Workdir, stderr, err, "")
	}
	return result, nil
}

// Commit delegates to the provider.
func (e *Executor) Commit(ctx context.Context, opts *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
	defer perf.Track(nil, "git.Executor.Commit")()

	var result *atmosgit.CommitResult
	stderr, err := e.captureStderr(func() error {
		var opErr error
		result, opErr = e.provider.Commit(ctx, opts)
		return opErr
	})
	if err != nil {
		return nil, wrapGitOperationError("commit Git changes", opts.Workdir, stderr, err, "")
	}
	return result, nil
}

// Push delegates to the provider.
func (e *Executor) Push(ctx context.Context, opts *atmosgit.PushOptions) error {
	defer perf.Track(nil, "git.Executor.Push")()

	stderr, err := e.captureStderr(func() error {
		return e.provider.Push(ctx, opts)
	})
	if err != nil {
		return wrapGitOperationError(
			"push Git repository",
			opts.Workdir,
			stderr,
			err,
			"Run 'atmos git status' and 'atmos git pull' before retrying the push.",
		)
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
