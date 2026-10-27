package shell

import (
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ExecuteShell runs a shell script.
//
// Deprecated: Use ExecuteShellMasked with an ExecuteShellSpec.
func ExecuteShell(
	command string,
	name string,
	dir string,
	envVars []string,
	dryRun bool,
) error {
	defer perf.Track(nil, "shell.ExecuteShell")()

	return ExecuteShellMasked(&ExecuteShellSpec{
		Command: command,
		Name:    name,
		Dir:     dir,
		EnvVars: envVars,
		DryRun:  dryRun,
	}, os.Stdout, os.Stderr, nil, nil)
}
