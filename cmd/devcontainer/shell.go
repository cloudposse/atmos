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
	shellUsePTY   bool // Experimental PTY mode flag.
	shellNew      bool // Create a new instance.
	shellReplace  bool // Replace existing instance.
	shellRm       bool // Remove container after exit.
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

Experimental: Use --pty for PTY mode with masking support (not available on Windows).

## Instance Management

- --new: Always create a new instance with auto-generated numbered name based on --instance value (e.g., default-1, default-2, or alice-1 with --instance alice)
- --replace: Destroy and recreate the instance specified by --instance flag (default "default")
- --rm: Automatically remove the container when you exit the shell (similar to 'docker run --rm')

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

		// Handle --replace: destroy and recreate the instance.
		if shellReplace {
			if err := e.ExecuteDevcontainerRebuild(atmosConfigPtr, name, shellInstance, shellIdentity, false); err != nil {
				return err
			}
			// Attach to the newly created container.
			err := e.ExecuteDevcontainerAttach(atmosConfigPtr, name, shellInstance, shellUsePTY)

			// If --rm flag is set, remove the container after exit.
			if shellRm {
				if rmErr := e.ExecuteDevcontainerRemove(atmosConfigPtr, name, shellInstance, true); rmErr != nil {
					if err != nil {
						return err
					}
					return rmErr
				}
			}

			return err
		}

		// Handle --new: create a new instance with auto-generated name.
		if shellNew {
			newInstance, err := e.GenerateNewDevcontainerInstance(atmosConfigPtr, name, shellInstance)
			if err != nil {
				return err
			}
			shellInstance = newInstance
		}

		// Start the container (creates if necessary).
		if err := e.ExecuteDevcontainerStart(atmosConfigPtr, name, shellInstance, shellIdentity); err != nil {
			return err
		}

		// Attach to the container.
		err = e.ExecuteDevcontainerAttach(atmosConfigPtr, name, shellInstance, shellUsePTY)

		// If --rm flag is set, remove the container after exit.
		if shellRm {
			// Remove the container (force=true to remove even if running).
			if rmErr := e.ExecuteDevcontainerRemove(atmosConfigPtr, name, shellInstance, true); rmErr != nil {
				// If attach failed, return attach error; otherwise return remove error.
				if err != nil {
					return err
				}
				return rmErr
			}
		}

		return err
	},
}

func init() {
	shellCmd.Flags().StringVar(&shellInstance, "instance", "default", "Instance name for this devcontainer")
	shellCmd.Flags().StringVarP(&shellIdentity, "identity", "i", "", "Authenticate with specified identity")
	shellCmd.Flags().BoolVar(&shellUsePTY, "pty", false, "Experimental: Use PTY mode with masking support (not available on Windows)")
	shellCmd.Flags().BoolVar(&shellNew, "new", false, "Create a new instance with auto-generated name")
	shellCmd.Flags().BoolVar(&shellReplace, "replace", false, "Destroy and recreate the current instance")
	shellCmd.Flags().BoolVar(&shellRm, "rm", false, "Automatically remove the container when the shell exits")

	// Mark flags as mutually exclusive.
	shellCmd.MarkFlagsMutuallyExclusive("new", "replace")

	devcontainerCmd.AddCommand(shellCmd)
}
