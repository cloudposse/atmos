package updater

import (
	"context"
	"strings"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
)

// PrepareBranch resolves the base branch (falling back to workdir's default branch on remote when
// baseBranch is empty) and prepares a component-update branch named
// "<branchPrefix|atmos/component-updater>/<scope>" off it. Workdir and remote are explicit
// parameters (not package-level vars) so tests can substitute a fixture repo -- this replaces
// today's package-level currentWorkdir test-seam var in cmd/vendor.
//
//nolint:revive // argument-limit: workdir, remote, baseBranch, branchPrefix, and scope are each independent branch-naming inputs read positionally at cmd/vendor's single call site.
func PrepareBranch(ctx context.Context, workdir, remote, baseBranch, branchPrefix, scope string) (string, string, error) {
	defer perf.Track(nil, "updater.PrepareBranch")()

	base := baseBranch
	if base == "" {
		var err error
		base, err = atmosgit.DefaultBranch(ctx, workdir, remote)
		if err != nil {
			return "", "", err
		}
	}
	prefix := strings.Trim(branchPrefix, "/")
	if prefix == "" {
		prefix = "atmos/component-updater"
	}
	branch := prefix + "/" + scope
	if err := atmosgit.PrepareBranch(ctx, atmosgit.PrepareBranchOptions{Workdir: workdir, Remote: remote, Base: base, Branch: branch}); err != nil {
		return "", "", err
	}
	return branch, base, nil
}
