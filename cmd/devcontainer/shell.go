package devcontainer

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var shellInstance string

var shellCmd = &cobra.Command{
	Use:   "shell <name>",
	Short: "Launch a shell in a devcontainer (alias for 'start --attach')",
	Long: `Launch a shell in a devcontainer.

This is a convenience command that combines start and attach operations:
- Starts the container if it's stopped
- Creates the container if it doesn't exist
- Attaches to the container with an interactive shell

This command is consistent with other Atmos shell commands (terraform shell, auth shell)
and provides the quickest way to get into a devcontainer environment.`,
	Example: markdown.DevcontainerShellUsageMarkdown,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.shell.RunE")()

		name := args[0]

		// Start the container (creates if necessary).
		if err := e.ExecuteDevcontainerStart(atmosConfigPtr, name, shellInstance); err != nil {
			return err
		}

		// Attach to the container.
		return e.ExecuteDevcontainerAttach(atmosConfigPtr, name, shellInstance)
	},
}

func init() {
	shellCmd.Flags().StringVar(&shellInstance, "instance", "default", "Instance name for this devcontainer")
	devcontainerCmd.AddCommand(shellCmd)
}
