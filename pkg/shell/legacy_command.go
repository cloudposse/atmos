package shell

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteShellCommand prints and executes the provided command.
//
// Deprecated: Use ExecuteCommand with a CommandRequest.
//
//nolint:revive,gocritic // Preserve the legacy public signature; new callers use CommandRequest.
func ExecuteShellCommand(atmosConfig schema.AtmosConfiguration, command string, args []string, dir string, env []string, dryRun bool, redirectStdError string, opts ...ShellCommandOption) error {
	defer perf.Track(nil, "shell.ExecuteShellCommand")()

	return ExecuteCommand(&CommandRequest{AtmosConfig: atmosConfig, Command: command, Args: args, Dir: dir, Env: env, DryRun: dryRun, RedirectStderr: redirectStdError}, opts...)
}
