package git

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// hooksCmd is the `atmos git hooks` parent command.
var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Manage local Git hook shims for the current repository",
	Long: `Manage .git/hooks shims that delegate to atmos git hooks run.

Subcommands:
  install    Write shim scripts into .git/hooks for configured hooks.
  uninstall  Remove Atmos-generated shims from .git/hooks.
  run        Execute the configured command for a named hook.`,
}

func init() {
	defer perf.Track(nil, "git.hooks.init")()

	hooksCmd.AddCommand(hooksInstallCmd)
	hooksCmd.AddCommand(hooksUninstallCmd)
	hooksCmd.AddCommand(hooksRunCmd)

	gitCmd.AddCommand(hooksCmd)
}
