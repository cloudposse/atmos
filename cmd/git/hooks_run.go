package git

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// hooksRunCmd is the `atmos git hooks run` subcommand.
//
// DisableFlagParsing is set because hook arguments may look like flags
// (e.g. commit-msg passes "$1" which is the commit file path, and pre-push
// passes refs that start with refs/). Cobra must not attempt to parse them.
// We extract the hook name manually from args.
var hooksRunCmd = &cobra.Command{
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

		return runHooksRun(hookName, hookArgs)
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

// runHooksRun implements `atmos git hooks run <hook-name> [args...]`.
func runHooksRun(hookName string, hookArgs []string) error {
	defer perf.Track(nil, "git.runHooksRun")()

	cfg := gitConfig()
	if cfg == nil || cfg.Hooks == nil {
		return hookNotConfiguredErr(hookName, nil)
	}

	entry, ok := cfg.Hooks[hookName]
	if !ok {
		return hookNotConfiguredErr(hookName, cfg.Hooks)
	}

	command := buildHookCommand(entry.Command, hookArgs)

	// Execute via the shared shell runner that workflows and custom commands use.
	// ShellRunner inherits os.Stdin via interp.StdIO(os.Stdin, ...) so hooks that
	// read from stdin (pre-push, pre-receive) work correctly.
	// ExitCodeError is returned when the child exits non-zero, preserving the code.
	mergedEnv := os.Environ()
	if err := u.ShellRunner(command, hookName, ".", mergedEnv, os.Stdout); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// buildHookCommand appends extra args to the configured command string.
// Args are shell-quoted by wrapping each in single quotes (with internal
// single-quotes escaped), consistent with POSIX sh expansion semantics.
func buildHookCommand(base string, args []string) string {
	if len(args) == 0 {
		return base
	}

	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + strings.ReplaceAll(a, "'", "'\\''") + "'"
	}

	return base + " " + strings.Join(quoted, " ")
}
