package devcontainer

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	shellInstance string
	shellIdentity string
)

var shellCmd = &cobra.Command{
	Use:   "shell [name]",
	Short: "Launch a shell in a devcontainer (alias for 'start --attach')",
	Long: `Launch a shell in a devcontainer.

This is a convenience command that combines start and attach operations:
- Starts the container if it's stopped
- Creates the container if it doesn't exist
- Attaches to the container with an interactive shell

If no devcontainer name is provided, you will be prompted to select one interactively.

This command is consistent with other Atmos shell commands (terraform shell, auth shell)
and provides the quickest way to get into a devcontainer environment.

## Using Authenticated Identities

Launch a devcontainer with Atmos-managed credentials:

  atmos devcontainer shell <name> --identity <identity-name>

Inside the container, cloud provider SDKs automatically use the authenticated identity.`,
	Example:           markdown.DevcontainerShellUsageMarkdown,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.shell.RunE")()

		// Get devcontainer name from args or prompt user.
		name, err := getDevcontainerName(args)
		if err != nil {
			return err
		}

		// Start the container (creates if necessary).
		if err := e.ExecuteDevcontainerStart(atmosConfigPtr, name, shellInstance, shellIdentity); err != nil {
			return err
		}

		// Attach to the container.
		return e.ExecuteDevcontainerAttach(atmosConfigPtr, name, shellInstance)
	},
}

func init() {
	shellCmd.Flags().StringVar(&shellInstance, "instance", "default", "Instance name for this devcontainer")
	shellCmd.Flags().StringVarP(&shellIdentity, "identity", "i", "", "Authenticate with specified identity")
	devcontainerCmd.AddCommand(shellCmd)
}
