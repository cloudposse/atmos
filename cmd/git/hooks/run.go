package hooks

import (
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	githooks "github.com/cloudposse/atmos/pkg/git/hooks"
	"github.com/cloudposse/atmos/pkg/perf"
)

// runCmd is the `atmos git hooks run` subcommand.
//
// DisableFlagParsing is set because hook arguments may look like flags
// (e.g. commit-msg passes "$1" which is the commit file path, and pre-push
// passes refs that start with refs/). Cobra must not attempt to parse them.
// We extract the hook name manually from args.
var runCmd = &cobra.Command{
	Use:                "run <hook-name> [args...]",
	Short:              "Execute the configured command for a named Git hook",
	Long:               `Execute the configured command for the named hook, forwarding any extra arguments and stdin.`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.hooks.run.RunE")()

		// DisableFlagParsing means cobra never auto-handles help, so a bare
		// `atmos git hooks run --help` would otherwise fall through to the
		// empty-hook-name path below and surface a config error. Handle it
		// explicitly so help is shown instead.
		if isHelpRequest(args) {
			_ = cmd.Help()
			return nil
		}

		hookName, hookArgs := extractHookNameAndArgs(args)
		if hookName == "" {
			return errUtils.Build(errUtils.ErrGitHookNotConfigured).
				WithHint("Usage: atmos git hooks run <hook-name> [args...]").
				WithExitCode(2).
				Err()
		}

		return githooks.Run(gitConfig(), hookName, hookArgs)
	},
}

// isHelpRequest reports whether args contain a help flag (-h/--help) before any
// "--" separator. Args after "--" belong to the downstream hook command and
// must not be interpreted as a help request for this command.
func isHelpRequest(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// extractHookNameAndArgs splits the raw args into hook name and remaining args.
// Skips any leading flags so the first non-flag token is treated as the hook
// name. Help flags are handled separately in RunE before this is called.
func extractHookNameAndArgs(args []string) (string, []string) {
	for i, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a, args[i+1:]
		}
	}
	return "", nil
}
