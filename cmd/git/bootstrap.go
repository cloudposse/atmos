package git

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/flags"
)

// CICloneBootstrapRequested reports whether the invoked command is a
// no-argument `atmos git clone` running under a detected CI provider that has
// not explicitly opted out of CI checkout (--ci=false / ATMOS_CI=false).
//
// RootCmd's config-init-error path (cmd/root.go) uses this to tolerate a
// missing or malformed atmos.yaml: the CI bootstrap clone runs in an empty
// workspace (e.g. replacing actions/checkout) where no atmos.yaml can exist
// yet. This must only be called after Cobra has parsed cmd's flags (true from
// PersistentPreRun onward), since it reads the real --ci flag via
// resolveCICloneMode instead of re-parsing os.Args by hand.
func CICloneBootstrapRequested(cmd *cobra.Command, args []string) bool {
	if !isCloneCommand(cmd) {
		return false
	}

	// --all bulk-clones every configured repository -- never the single-repo
	// CI bootstrap case, even with zero positional args.
	if all, _ := cmd.Flags().GetBool(flagAll); all {
		return false
	}

	// A native-arg separator (`clone -- --no-tags`) signals a deliberate,
	// hand-crafted invocation, not the zero-argument auto-bootstrap case.
	positional, separated := flags.SplitArgsAtDash(cmd, args)
	if len(positional) != 0 || len(separated) != 0 {
		return false
	}

	if ci.Detect() == nil {
		return false
	}

	// resolveCICloneMode returns ciCloneModeAuto even when --ci/ATMOS_CI is
	// malformed (its error is for the caller to report); treating that the
	// same as "auto" here defers the actual error to parseCloneFlags in the
	// command's own RunE, where it belongs.
	mode, _ := resolveCICloneMode(cmd)
	return mode != ciCloneModeDisabled
}

// isCloneCommand reports whether cmd is the `atmos git clone` leaf, checked
// by name/parent rather than pointer identity so callers can exercise this
// with a lightweight test command tree instead of the package's real
// singletons.
func isCloneCommand(cmd *cobra.Command) bool {
	return cmd != nil &&
		cmd.Name() == cloneCmd.Name() &&
		cmd.Parent() != nil &&
		cmd.Parent().Name() == gitCmd.Name()
}
