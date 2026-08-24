package updater

import (
	"context"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
)

// PreparedWorktree is the outcome of PrepareUpdateWorktree.
type PreparedWorktree struct {
	// Path is the isolated worktree's directory. Empty when worktree creation itself failed.
	Path string
	// Base is the concrete base branch actually resolved/used -- set as soon as it's known, even if
	// worktree creation subsequently fails, so a caller can still report which base was attempted.
	Base string
	// Cleanup removes the worktree. Always non-nil, including on error, so callers can
	// unconditionally `defer result.Cleanup()` right after the call; it is a safe no-op for
	// anything that was never actually created.
	Cleanup func()
}

// PrepareUpdateWorktree resolves baseBranch (falling back to workdir's advertised remote default
// branch when empty, mirroring PrepareBranch's own fallback) and creates an isolated git worktree
// checked out at "<remote>/<base>", for vendor.update.execution.mode: worktree. Unlike PrepareBranch
// (which checks out a feature branch inside an existing workdir), this creates a brand-new,
// detached-HEAD working tree so the entire discover -> bump -> branch -> commit -> push cycle can
// run without ever touching workdir's own files, branch, or index.
//
// The returned PreparedWorktree.Base is the concrete base branch actually resolved, so a caller
// with its own possibly-empty base-branch config can reuse it instead of re-resolving it a second
// time against the same remote.
func PrepareUpdateWorktree(ctx context.Context, workdir, remote, baseBranch string) (*PreparedWorktree, error) {
	defer perf.Track(nil, "updater.PrepareUpdateWorktree")()

	result := &PreparedWorktree{Cleanup: func() {}}

	base := baseBranch
	if base == "" {
		resolved, err := atmosgit.DefaultBranch(ctx, workdir, remote)
		if err != nil {
			return result, err
		}
		base = resolved
	}
	result.Base = base

	worktreePath, err := atmosgit.CreateWorktreeWithFetchRecovery(workdir, remote+"/"+base, base)
	if err != nil {
		return result, err
	}

	result.Path = worktreePath
	result.Cleanup = func() { atmosgit.RemoveWorktree(workdir, worktreePath) }
	return result, nil
}
