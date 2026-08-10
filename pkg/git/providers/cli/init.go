package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
)

// sourceRemoteName is the remote name under which the --from repository stays
// reachable in keep-history mode, so future updates can still be pulled.
const sourceRemoteName = "upstream"

// initSubcommand is the git subcommand (and classify label) for `git init`.
const initSubcommand = "init"

// remoteSubcommand is the git subcommand for managing remotes.
const remoteSubcommand = "remote"

// Init creates a repository workdir — the inverse of Clone, for GitOps
// repositories whose remote has no content yet.
//
// Init is idempotent for the empty (no --from) mode: re-running it against an
// already-initialized repository reconciles in place (`git init` is idempotent
// and the configured remote is updated rather than duplicated). --force means
// "redo from scratch": the existing workdir is deleted and re-initialized.
func (p *Provider) Init(ctx context.Context, opts *atmosgit.InitOptions) error {
	defer perf.Track(nil, "cli.Provider.Init")()

	if opts.Force {
		// Destructive by design: discard any existing workdir, then re-create.
		if err := os.RemoveAll(opts.Workdir); err != nil {
			return fmt.Errorf("removing existing workdir %q: %w", opts.Workdir, err)
		}
		return p.initByMode(ctx, opts, false)
	}

	existed, isRepo, err := inspectInitTarget(opts.Workdir)
	if err != nil {
		return err
	}

	// An already-initialized repository reconciles idempotently in either mode:
	// the repository already exists, so there is nothing to seed — re-running
	// init just re-runs `git init` and re-points the configured remote. Seeding
	// from --from happens only on a fresh workdir; --force re-seeds from scratch.
	if isRepo {
		return p.initEmpty(ctx, opts, true)
	}
	if existed {
		return fmt.Errorf("%w: %q is not empty; use --force to re-create it",
			errUtils.ErrGitWorkdirExists, opts.Workdir)
	}
	return p.initByMode(ctx, opts, false)
}

// initByMode dispatches to the empty or seeded init path. The reconcile flag
// applies only to the empty path (idempotent re-init of an existing repository).
func (p *Provider) initByMode(ctx context.Context, opts *atmosgit.InitOptions, reconcile bool) error {
	if opts.FromURI == "" {
		return p.initEmpty(ctx, opts, reconcile)
	}
	return p.initSeed(ctx, opts)
}

// initSeed dispatches between the fresh-history and keep-history seeding paths.
func (p *Provider) initSeed(ctx context.Context, opts *atmosgit.InitOptions) error {
	if opts.KeepHistory {
		return p.initFromSourceKeepHistory(ctx, opts)
	}
	return p.initFromSourceFresh(ctx, opts)
}

// inspectInitTarget reports whether the workdir already has content and whether
// that content is a Git repository (a ".git" entry is present).
func inspectInitTarget(workdir string) (existed, isRepo bool, err error) {
	entries, err := os.ReadDir(workdir)
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("checking init target %q: %w", workdir, err)
	}
	if len(entries) == 0 {
		return false, false, nil
	}
	return true, hasGitDir(entries), nil
}

// hasGitDir reports whether a directory listing contains a ".git" entry,
// indicating the workdir is already a Git repository.
func hasGitDir(entries []os.DirEntry) bool {
	for _, e := range entries {
		if e.Name() == ".git" {
			return true
		}
	}
	return false
}

// initEmpty creates an empty repository on the configured branch with the
// configured remote wired up, ready for `atmos git commit` + `atmos git push`
// against an empty remote. When reconciling (the workdir already existed, under
// --force), `git init` runs idempotently and the configured remote is updated
// in place rather than duplicated.
func (p *Provider) initEmpty(ctx context.Context, opts *atmosgit.InitOptions, reconcile bool) error {
	if err := os.MkdirAll(opts.Workdir, workdirParentPerm); err != nil {
		return fmt.Errorf("creating workdir %q: %w", opts.Workdir, err)
	}

	args := []string{initSubcommand}
	if opts.Branch != "" {
		args = append(args, "-b", opts.Branch)
	}
	args = append(args, opts.ExtraArgs...)
	if result, err := p.run(ctx, opts.Workdir, opts.Env, args...); err != nil {
		return classify(err, result, initSubcommand)
	}

	if reconcile {
		return p.wireRemote(ctx, opts.RepoContext, remoteOrDefault(opts.Remote), opts.URI)
	}
	return p.addRemote(ctx, opts.RepoContext, remoteOrDefault(opts.Remote), opts.URI)
}

// initFromSourceFresh imports the source repository's content as a single
// fresh initial commit: clone, discard the source history, re-init on the
// configured branch, and commit everything. No link to the source remains.
func (p *Provider) initFromSourceFresh(ctx context.Context, opts *atmosgit.InitOptions) error {
	// History is discarded, so a shallow clone minimizes transfer.
	if err := p.cloneSource(ctx, opts, false, "--depth", "1"); err != nil {
		return err
	}

	if err := os.RemoveAll(filepath.Join(opts.Workdir, ".git")); err != nil {
		return fmt.Errorf("removing source history in %q: %w", opts.Workdir, err)
	}

	initArgs := []string{initSubcommand}
	if opts.Branch != "" {
		initArgs = append(initArgs, "-b", opts.Branch)
	}
	if result, err := p.run(ctx, opts.Workdir, opts.Env, initArgs...); err != nil {
		return classify(err, result, initSubcommand)
	}

	if err := p.addRemote(ctx, opts.RepoContext, remoteOrDefault(opts.Remote), opts.URI); err != nil {
		return err
	}

	if result, err := p.run(ctx, opts.Workdir, opts.Env, "add", "-A"); err != nil {
		return classify(err, result, "add")
	}

	// --allow-empty keeps an empty template from failing the initial commit.
	commitArgs := buildCommitArgs(&atmosgit.CommitOptions{
		Message: "Initialize from " + opts.FromURI,
		Signing: opts.Signing,
		Author:  opts.Author,
	})
	commitArgs = append(commitArgs, "--allow-empty")
	if result, err := p.run(ctx, opts.Workdir, opts.Env, commitArgs...); err != nil {
		return classify(err, result, "commit")
	}

	return nil
}

// initFromSourceKeepHistory clones the source with its full history, keeps the
// source reachable under the "upstream" remote (so updates can be pulled), and
// wires the configured remote to the configured URI.
func (p *Provider) initFromSourceKeepHistory(ctx context.Context, opts *atmosgit.InitOptions) error {
	if err := p.cloneSource(ctx, opts, true); err != nil {
		return err
	}

	configured := remoteOrDefault(opts.Remote)
	source := sourceRemoteName
	if source == configured {
		// The configured remote claims "upstream"; keep the source under
		// "source" so both stay addressable.
		source = "source"
	}

	if result, err := p.run(ctx, opts.Workdir, opts.Env, remoteSubcommand, "rename", atmosgit.DefaultRemote, source); err != nil {
		return classify(err, result, "remote rename")
	}

	return p.addRemote(ctx, opts.RepoContext, configured, opts.URI)
}

// cloneSource clones the --from repository into the workdir. The configured
// branch is requested only in keep-history mode, where it must exist in the
// source; in fresh mode the branch names the new history instead.
func (p *Provider) cloneSource(ctx context.Context, opts *atmosgit.InitOptions, includeBranch bool, extra ...string) error {
	if err := os.MkdirAll(filepath.Dir(opts.Workdir), workdirParentPerm); err != nil {
		return fmt.Errorf("creating workdir parent for %q: %w", opts.Workdir, err)
	}

	args := []string{"clone"}
	args = append(args, extra...)
	if includeBranch && opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}
	args = append(args, opts.ExtraArgs...)
	args = append(args, "--", opts.FromURI, opts.Workdir)

	result, err := p.run(ctx, "", opts.Env, args...)
	if err != nil {
		return classify(err, result, "clone")
	}
	return nil
}

// addRemote registers a remote, pointing name at uri.
func (p *Provider) addRemote(ctx context.Context, rc atmosgit.RepoContext, name, uri string) error {
	result, err := p.run(ctx, rc.Workdir, rc.Env, remoteSubcommand, "add", name, uri)
	if err != nil {
		return classify(err, result, "remote add")
	}
	return nil
}

// wireRemote points name at uri, updating the remote when it already exists
// (reconcile path) instead of failing on a duplicate `remote add`.
func (p *Provider) wireRemote(ctx context.Context, rc atmosgit.RepoContext, name, uri string) error {
	if _, err := p.run(ctx, rc.Workdir, rc.Env, remoteSubcommand, "get-url", name); err != nil {
		// Remote not present yet: add it.
		return p.addRemote(ctx, rc, name, uri)
	}
	result, err := p.run(ctx, rc.Workdir, rc.Env, remoteSubcommand, "set-url", name, uri)
	if err != nil {
		return classify(err, result, "remote set-url")
	}
	return nil
}
