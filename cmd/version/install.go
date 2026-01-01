package version

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/toolchain"
)

// installCmd represents the `atmos version install` command.
var installCmd = &cobra.Command{
	Use:   "install <version>",
	Short: "Install a specific version of Atmos",
	Long: `Install a specific version of Atmos for use with version.use or manual switching.

This is a convenience wrapper around: atmos toolchain install atmos@<version>

The installed version will be stored in ~/.atmos/bin/cloudposse/atmos/<version>/
`,
	Example: `  # Install a specific version
  atmos version install 1.160.0

  # Install the latest version
  atmos version install latest`,
	Args:          cobra.ExactArgs(1),
	RunE:          runInstall,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	versionCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "version.runInstall")()

	version := args[0]
	toolSpec := "atmos@" + version

	// Delegate to toolchain installer.
	// The built-in alias "atmos" -> "cloudposse/atmos" handles the resolution.
	return toolchain.RunInstall(toolSpec, false, false)
}
