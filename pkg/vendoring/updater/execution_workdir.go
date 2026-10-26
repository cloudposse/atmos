package updater

import (
	"context"
	"os"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ExecutionWorkdir is the outcome of ResolveExecutionWorkdir.
type ExecutionWorkdir struct {
	// Workdir is the git workdir the --pull-request publish path's branch/commit/push operations
	// use: currentWorkdir unchanged for the default execution mode, or the isolated worktree's path
	// for vendor.update.execution.mode: worktree.
	Workdir string
	// ResolvedBase is the concrete base branch worktree mode already resolved (so the caller can
	// reuse it instead of re-resolving via a second remote call); empty for the default execution
	// mode, where the caller keeps using its own vendor.ci.pull_request.base_branch config,
	// unresolved, exactly as before this feature existed.
	ResolvedBase string
	// Cleanup unwinds whatever ResolveExecutionWorkdir set up. Always non-nil, including on error,
	// so callers can unconditionally `defer execWorkdir.Cleanup()` right after the call; it is a
	// no-op for the default execution mode.
	Cleanup func()
}

// ResolveExecutionWorkdir sets up the git workdir (and, for worktree mode, the BasePath redirect)
// the --pull-request publish path uses for its whole discover -> bump -> branch -> commit -> push
// cycle. When vendor.update.execution.mode is "worktree", that entire cycle runs inside an isolated
// git worktree, leaving the invoking checkout's working tree byte-for-byte unchanged (no modified
// files, no branch switch, no new local branch). When execution mode is not "worktree" (empty or
// "current"), the returned ExecutionWorkdir.Workdir is currentWorkdir unchanged, ResolvedBase is
// empty, and Cleanup is a no-op -- this is a strict superset feature and never changes the default
// path's behavior.
//
// The redirect combines two mechanisms, both necessary (confirmed empirically, not just by reading
// cfg.InitCliConfig):
//
//  1. Temporarily pointing the ATMOS_BASE_PATH environment variable at the worktree. Every
//     path-resolution call in pkg/vendoring's discovery/resolve/write chain
//     (DiscoverAllComponentManifests, DefaultComponentDirResolver.ComponentDir,
//     ResolveComponentSource's component.yaml fallback) calls cfg.InitCliConfig fresh with an empty
//     schema.ConfigAndStacksInfo{}, so nothing in that chain supplies a stronger override -- only an
//     explicit ConfigAndStacksInfo.AtmosBasePath (the CLI --base-path flag or Terraform provider
//     param, never set by those call sites) would outrank the env var.
//  2. Temporarily changing the process's actual working directory to the worktree, via pkg/env's
//     existing Chdir primitive (the same one --chdir itself uses). This is required because
//     VendorFilePresent's default lookup (no --file override, no vendor.base_path configured in
//     atmos.yaml -- the common case) checks for "./vendor.yaml" relative to the process's real
//     working directory *before* ever consulting atmosConfig.BasePath -- ATMOS_BASE_PATH alone does
//     not redirect that check, so without also moving the process cwd, that lookup would keep
//     resolving (and writing) vendor.yaml in the invoking checkout instead of the worktree.
func ResolveExecutionWorkdir(ctx context.Context, v *viper.Viper, currentWorkdir string) (*ExecutionWorkdir, error) {
	defer perf.Track(nil, "updater.ResolveExecutionWorkdir")()

	if v.GetString("vendor.update.execution.mode") != "worktree" {
		return &ExecutionWorkdir{Workdir: currentWorkdir, Cleanup: func() {}}, nil
	}

	prepared, wErr := PrepareUpdateWorktree(ctx, currentWorkdir, "origin", v.GetString("vendor.ci.pull_request.base_branch"))
	if wErr != nil {
		return &ExecutionWorkdir{Cleanup: func() {}}, wErr
	}

	restoreBasePath, envErr := env.SetWithRestore(map[string]string{"ATMOS_BASE_PATH": prepared.Path})
	if envErr != nil {
		prepared.Cleanup()
		return &ExecutionWorkdir{Cleanup: func() {}}, envErr
	}

	originalWd, wdErr := os.Getwd()
	if wdErr != nil {
		restoreBasePath()
		prepared.Cleanup()
		return &ExecutionWorkdir{Cleanup: func() {}}, wdErr
	}
	if chdirErr := env.Chdir(prepared.Path); chdirErr != nil {
		restoreBasePath()
		prepared.Cleanup()
		return &ExecutionWorkdir{Cleanup: func() {}}, chdirErr
	}

	cleanup := func() {
		_ = env.Chdir(originalWd)
		restoreBasePath()
		prepared.Cleanup()
	}
	return &ExecutionWorkdir{Workdir: prepared.Path, ResolvedBase: prepared.Base, Cleanup: cleanup}, nil
}
