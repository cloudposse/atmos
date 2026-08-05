// Package cli implements the Git CLI provider: the universal, host-agnostic
// execution backend that shells out to git. It is the only v1 provider, chosen
// because GitHub STS materializes credentials as GIT_CONFIG_* environment
// variables, which subprocess git honors (and go-git ignores).
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	gitCommand = "git"
	// Permission for created workdir parent directories.
	workdirParentPerm = 0o755
)

func init() {
	atmosgit.RegisterProvider(atmosgit.DefaultProviderName, func() atmosgit.Provider {
		return New()
	})
}

// Provider executes Git operations via the git CLI.
type Provider struct {
	runner atmosgit.Runner
	stderr io.Writer
}

// Option configures the Provider (functional options pattern).
type Option func(*Provider)

// WithRunner substitutes the subprocess runner (used by tests).
func WithRunner(runner atmosgit.Runner) Option {
	return func(p *Provider) { p.runner = runner }
}

// WithStderr directs subprocess stderr to a writer. Production callers pass a
// masked writer from pkg/io so secrets are masked at write time; stderr is
// never embedded in error chains.
func WithStderr(w io.Writer) Option {
	return func(p *Provider) { p.stderr = w }
}

// New constructs a CLI provider.
func New(opts ...Option) *Provider {
	p := &Provider{runner: atmosgit.NewExecRunner()}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// SwapStderr replaces the stderr writer and returns a restore function.
func (p *Provider) SwapStderr(w io.Writer) func() {
	prev := p.stderr
	p.stderr = w
	return func() {
		p.stderr = prev
	}
}

// run invokes git with the repository context applied.
func (p *Provider) run(ctx context.Context, dir string, env []string, args ...string) (atmosgit.RunResult, error) {
	return p.runner.Run(ctx, gitCommand, args, atmosgit.RunOptions{Dir: dir, Env: env, Stderr: p.stderr})
}

func (p *Provider) runQuiet(ctx context.Context, dir string, env []string, args ...string) (atmosgit.RunResult, error) {
	return p.runner.Run(ctx, gitCommand, args, atmosgit.RunOptions{Dir: dir, Env: env, Stderr: p.stderr})
}

// Clone reconciles a repository workdir: a fresh clone when the workdir is
// absent, otherwise fetch and fast-forward to the expected ref. Reconcile
// makes clone behavior independent of cache freshness (CI cache restores may
// be stale).
func (p *Provider) Clone(ctx context.Context, opts *atmosgit.CloneOptions) error {
	defer perf.Track(nil, "cli.Provider.Clone")()

	if isGitWorktree(opts.Workdir) {
		return p.reconcile(ctx, opts)
	}
	return p.cloneFresh(ctx, opts)
}

// cloneFresh performs the initial clone into an absent workdir.
func (p *Provider) cloneFresh(ctx context.Context, opts *atmosgit.CloneOptions) error {
	if err := os.MkdirAll(filepath.Dir(opts.Workdir), workdirParentPerm); err != nil {
		return fmt.Errorf("creating workdir parent for %q: %w", opts.Workdir, err)
	}

	args := []string{"clone"}
	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}
	if opts.Filter != "" {
		args = append(args, "--filter", opts.Filter)
	}
	if opts.SingleBranch {
		args = append(args, "--single-branch")
	}
	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}
	if opts.Submodules {
		args = append(args, "--recurse-submodules")
	}
	// Native passthrough args are flags for git clone, so they must precede
	// the "--" separating flags from the URI/workdir positionals.
	args = append(args, opts.ExtraArgs...)
	args = append(args, "--", opts.URI, opts.Workdir)

	result, err := p.run(ctx, "", opts.Env, args...)
	if err != nil {
		return classify(err, result, "clone")
	}
	return nil
}

// reconcile brings an existing workdir up to date: fetch, ensure the
// configured branch is checked out, then fast-forward only.
func (p *Provider) reconcile(ctx context.Context, opts *atmosgit.CloneOptions) error {
	if err := p.ensureCleanWorktree(ctx, opts.RepoContext); err != nil {
		return err
	}

	if err := p.fetch(ctx, opts); err != nil {
		return err
	}

	if opts.Branch != "" {
		if err := p.checkoutBranch(ctx, opts.RepoContext); err != nil {
			return err
		}
		return p.fastForward(ctx, opts.RepoContext, remoteRef(opts.Remote, opts.Branch))
	}

	return p.fastForward(ctx, opts.RepoContext, "FETCH_HEAD")
}

// fetch updates remote-tracking refs, honoring shallow/partial clone options.
func (p *Provider) fetch(ctx context.Context, opts *atmosgit.CloneOptions) error {
	args := []string{"fetch", remoteOrDefault(opts.Remote)}
	if opts.Branch != "" {
		args = append(args, fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", opts.Branch, remoteOrDefault(opts.Remote), opts.Branch))
	}
	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}
	if opts.Filter != "" {
		args = append(args, "--filter", opts.Filter)
	}

	result, err := p.run(ctx, opts.Workdir, opts.Env, args...)
	if err != nil {
		return classify(err, result, "fetch")
	}
	return nil
}

// checkoutBranch checks out the configured branch, creating it from the
// remote-tracking ref when it does not exist locally.
func (p *Provider) checkoutBranch(ctx context.Context, rc atmosgit.RepoContext) error {
	if _, err := p.run(ctx, rc.Workdir, rc.Env, "checkout", rc.Branch); err == nil {
		return nil
	}

	result, err := p.run(ctx, rc.Workdir, rc.Env, "checkout", "-b", rc.Branch, remoteRef(rc.Remote, rc.Branch))
	if err != nil {
		return classify(err, result, "checkout")
	}
	return nil
}

// fastForward merges the given ref fast-forward-only.
func (p *Provider) fastForward(ctx context.Context, rc atmosgit.RepoContext, ref string) error {
	result, err := p.run(ctx, rc.Workdir, rc.Env, "merge", "--ff-only", ref)
	if err != nil {
		return classify(err, result, "merge --ff-only")
	}
	return nil
}

// ensureCleanWorktree fails reconcile when uncommitted changes are present
// (e.g. a crashed prior run); recovery is intentionally explicit, not silent.
func (p *Provider) ensureCleanWorktree(ctx context.Context, rc atmosgit.RepoContext) error {
	status, err := p.Status(ctx, &atmosgit.StatusOptions{RepoContext: rc})
	if err != nil {
		return err
	}
	if !status.Clean {
		return fmt.Errorf("%w: workdir %q has %d uncommitted change(s); commit, stash, or remove the workdir to re-clone", errUtils.ErrGitDirtyUnmanagedFiles, rc.Workdir, len(status.Entries))
	}
	return nil
}

// Pull fast-forwards the current branch from the remote. Fast-forward-only is
// a safety rule, not an option.
func (p *Provider) Pull(ctx context.Context, opts *atmosgit.PullOptions) error {
	defer perf.Track(nil, "cli.Provider.Pull")()

	args := []string{"pull", "--ff-only", remoteOrDefault(opts.Remote)}
	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}
	args = append(args, opts.ExtraArgs...)

	result, err := p.run(ctx, opts.Workdir, opts.Env, args...)
	if err != nil {
		if isNoTrackingBranch(result) {
			return fmt.Errorf("%w: %w", errUtils.ErrGitNoTrackingBranch, err)
		}
		return classify(err, result, "pull --ff-only")
	}
	return nil
}

// isGitWorktree reports whether dir contains a .git entry (dir or file —
// linked worktrees use a .git file).
func isGitWorktree(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// remoteOrDefault applies the default remote name.
func remoteOrDefault(remote string) string {
	if remote == "" {
		return atmosgit.DefaultRemote
	}
	return remote
}

// remoteRef builds a remote-tracking ref like "origin/main".
func remoteRef(remote, branch string) string {
	return remoteOrDefault(remote) + "/" + branch
}
