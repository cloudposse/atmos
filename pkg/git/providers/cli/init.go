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

// Init creates a new repository workdir from scratch — the inverse of Clone,
// for GitOps repositories whose remote has no content yet.
func (p *Provider) Init(ctx context.Context, opts *atmosgit.InitOptions) error {
	defer perf.Track(nil, "cli.Provider.Init")()

	if err := ensureInitTarget(opts.Workdir); err != nil {
		return err
	}

	if opts.FromURI == "" {
		return p.initEmpty(ctx, opts)
	}
	if opts.KeepHistory {
		return p.initFromSourceKeepHistory(ctx, opts)
	}
	return p.initFromSourceFresh(ctx, opts)
}

// ensureInitTarget refuses to initialize into an existing non-empty directory;
// init never overwrites content (clone owns reconciling existing workdirs).
func ensureInitTarget(workdir string) error {
	entries, err := os.ReadDir(workdir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking init target %q: %w", workdir, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("%w: %q is not empty", errUtils.ErrGitWorkdirExists, workdir)
	}
	return nil
}

// initEmpty creates an empty repository on the configured branch with the
// configured remote wired up, ready for `atmos git commit` + `atmos git push`
// against an empty remote.
func (p *Provider) initEmpty(ctx context.Context, opts *atmosgit.InitOptions) error {
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

	if result, err := p.run(ctx, opts.Workdir, opts.Env, "remote", "rename", atmosgit.DefaultRemote, source); err != nil {
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
	result, err := p.run(ctx, rc.Workdir, rc.Env, "remote", "add", name, uri)
	if err != nil {
		return classify(err, result, "remote add")
	}
	return nil
}
