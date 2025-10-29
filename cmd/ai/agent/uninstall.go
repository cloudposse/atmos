package agent

import (
	"fmt"

	"github.com/spf13/cobra"

	ai "github.com/cloudposse/atmos/cmd/ai"

	"github.com/cloudposse/atmos/pkg/ai/agents/marketplace"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// aiAgentUninstallCmd represents the 'atmos ai agent uninstall' command.
var uninstallCmd = &cobra.Command{
	Use:   "uninstall <name>",
	Short: "Remove an installed agent",
	Long: `Uninstall a community-contributed agent from this system.

This will remove the agent from ~/.atmos/agents/ and delete its registry entry.
You will be prompted to confirm the uninstallation unless --force is specified.

The agent name is the short identifier (not the display name).
Use 'atmos ai agent list' to see installed agent names.

Examples:
  # Uninstall an agent (with confirmation)
  atmos ai agent uninstall terraform-optimizer

  # Force uninstall without confirmation
  atmos ai agent uninstall terraform-optimizer --force`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.aiAgentUninstallCmd")()

		name := args[0]

		// Get flags.
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			return fmt.Errorf("failed to get --force flag: %w", err)
		}

		// Create installer (which manages registry).
		installer, err := marketplace.NewInstaller(version.Version)
		if err != nil {
			return fmt.Errorf("failed to initialize installer: %w", err)
		}

		// Uninstall agent.
		if err := installer.Uninstall(name, force); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	// Add flags.
	uninstallCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	// Add 'uninstall' subcommand to 'agent' command.
	ai.AgentCmd.AddCommand(uninstallCmd)
}
