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

// CIGitCloneBootstrapRequestedFromRawArgs reports the same no-argument CI
// git-clone bootstrap condition as CICloneBootstrapRequested, but checked
// against raw, unparsed arguments instead of an already Cobra-resolved
// command, where rawArgs is the clone-specific arguments only, with the
// leading "atmos git clone" tokens already stripped by the caller.
//
// Execute() in cmd/root.go runs an initial cfg.InitCliConfig before Cobra
// ever parses the invoked command (see that function's config-init-error
// handling). A missing/invalid atmos.yaml or unresolved profile at that point
// currently aborts the process before PersistentPreRun -- and therefore
// before CICloneBootstrapRequested/applyCIGitCloneBootstrap ever run -- even
// for the CI bootstrap clone, which runs in an empty workspace where no
// atmos.yaml or profile can exist yet. This lets that earlier handler
// recognize the same bootstrap shape.
//
// This parses rawArgs against a throwaway command carrying the real clone
// flag set (a fresh newCloneParser() instance, never the shared package-level
// cloneParser singleton, to avoid disturbing its registered *cobra.Command)
// so flags that take a value, such as --depth and --branch, are correctly
// distinguished from a positional repo name/URI via real pflag parsing,
// instead of guessing from a "-"-prefix heuristic that a space-separated
// flag value such as the "0" in `--depth 0` would misread as a positional
// argument and wrongly disqualify the bootstrap. It also honors an explicit
// --ci/--ci=false on rawArgs, which a purely environment-based check could
// not see.
func CIGitCloneBootstrapRequestedFromRawArgs(rawArgs []string) bool {
	root := &cobra.Command{Use: "atmos"}
	git := &cobra.Command{Use: gitCmd.Name()}
	clone := &cobra.Command{Use: cloneCmd.Name()}
	newCloneParser().RegisterFlags(clone)
	root.AddCommand(git)
	git.AddCommand(clone)

	if err := clone.ParseFlags(rawArgs); err != nil {
		// Malformed flags: match CICloneBootstrapRequested's existing
		// philosophy (resolveCICloneMode) of deferring the actual error to
		// the command's own RunE rather than reporting it here.
		return false
	}
	return CICloneBootstrapRequested(clone, clone.Flags().Args())
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
