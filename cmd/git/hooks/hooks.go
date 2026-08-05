// Package hooks implements the `atmos git hooks` command group: thin cobra
// wiring over pkg/git/hooks, which owns the shim install/run/uninstall logic.
package hooks

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// hooksViperPrefix namespaces hooks flag keys on the global Viper (see the
// per-subcommand prefix rationale in cmd/git/flags.go).
const hooksViperPrefix = "git.hooks"

// viperKey returns the namespaced Viper key for a hooks flag.
func viperKey(flagName string) string {
	return hooksViperPrefix + "." + flagName
}

// atmosConfigPtr holds the Atmos configuration forwarded by cmd/git before
// any subcommand runs.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig is called from cmd/git when root.go injects the configuration,
// making it available to all hooks subcommands.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// gitConfig returns the Git section of the loaded Atmos configuration, or nil.
func gitConfig() *schema.GitConfig {
	if atmosConfigPtr == nil {
		return nil
	}
	return &atmosConfigPtr.Git
}

// Command is the `atmos git hooks` parent command (with subcommands attached),
// registered by cmd/git.
var Command = &cobra.Command{
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

	Command.AddCommand(installCmd)
	Command.AddCommand(uninstallCmd)
	Command.AddCommand(runCmd)
}
