package version

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/toolchain"
)

// uninstallCmd represents the `atmos version uninstall` command.
var uninstallCmd = &cobra.Command{
	Use:   "uninstall <version>",
	Short: "Uninstall a specific version of Atmos",
	Long: `Uninstall a specific version of Atmos.

This is a convenience wrapper around: atmos toolchain uninstall atmos@<version>

The version will be removed from ~/.atmos/bin/cloudposse/atmos/<version>/
`,
	Example: `  # Uninstall a specific version
  atmos version uninstall 1.160.0

  # Uninstall all installed versions
  atmos version uninstall`,
	Args:          cobra.MaximumNArgs(1),
	RunE:          runUninstall,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	versionCmd.AddCommand(uninstallCmd)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "version.runUninstall")()

	toolSpec := "atmos"
	if len(args) > 0 {
		toolSpec = "atmos@" + args[0]
	}

	// Delegate to toolchain uninstaller.
	// The built-in alias "atmos" -> "cloudposse/atmos" handles the resolution.
	return toolchain.RunUninstall(toolSpec)
}
