package hooks

import (
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Run executes the configured command for the named hook, forwarding the hook
// arguments and stdin.
func Run(cfg *schema.GitConfig, hookName string, hookArgs []string, opts ...RunOption) error {
	defer perf.Track(nil, "hooks.Run")()

	if cfg == nil || cfg.Hooks == nil {
		return NotConfiguredError(hookName, nil)
	}

	entry, ok := cfg.Hooks[hookName]
	if !ok {
		return NotConfiguredError(hookName, cfg.Hooks)
	}

	if err := validateHookConfig(hookName, entry); err != nil {
		return wrapHookError(hookName, err)
	}
	if len(entry.Steps) > 0 {
		return runSteps(hookName, entry, hookArgs, opts)
	}

	dir, err := resolveWorkingDir()
	if err != nil {
		return wrapHookError(hookName, err)
	}

	command := buildHookCommand(entry.Command, hookArgs)

	// Execute via the shared shell runner that workflows and custom commands use.
	// ShellRunner inherits os.Stdin via interp.StdIO(os.Stdin, ...) so hooks that
	// read from stdin (pre-push, pre-receive) work correctly.
	// ExitCodeError is returned when the child exits non-zero, preserving the code.
	mergedEnv := os.Environ()
	if err := u.ShellRunner(command, hookName, dir, mergedEnv, os.Stdout); err != nil {
		return wrapHookError(hookName, err)
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
