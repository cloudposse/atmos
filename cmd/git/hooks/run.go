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

// extractHookNameAndArgs splits the raw args into hook name and remaining args.
// Skips any leading flags (e.g. --help) so a bare call like
// `atmos git hooks run --help` still works for documentation purposes.
func extractHookNameAndArgs(args []string) (string, []string) {
	for i, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a, args[i+1:]
		}
	}
	return "", nil
}
