package skill

import (
	"fmt"

	"github.com/spf13/cobra"

	ai "github.com/cloudposse/atmos/cmd/ai"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// uninstallCmd represents the 'atmos ai skill uninstall' command.
var uninstallCmd = &cobra.Command{
	Use:   "uninstall <name>",
	Short: "Remove an installed skill",
	Long: `Uninstall a community-contributed skill from this system.

This will remove the skill from ~/.atmos/skills/ and delete its registry entry.
You will be prompted to confirm the uninstallation unless --force is specified.

The skill name is the short identifier (not the display name).
Use 'atmos ai skill list' to see installed skill names.

Examples:
  # Uninstall a skill (with confirmation)
  atmos ai skill uninstall terraform-optimizer

  # Force uninstall without confirmation
  atmos ai skill uninstall terraform-optimizer --force`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.aiSkillUninstallCmd")()

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

		// Uninstall skill.
		if err := installer.Uninstall(name, force); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	// Add flags.
	uninstallCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	// Add 'uninstall' subcommand to 'skill' command.
	ai.SkillCmd.AddCommand(uninstallCmd)
}
