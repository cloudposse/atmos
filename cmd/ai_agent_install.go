package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/ai/agents/marketplace"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// aiAgentInstallCmd represents the 'atmos ai agent install' command.
var aiAgentInstallCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install an agent from a GitHub repository",
	Long: `Install a community-contributed agent from a GitHub repository.

The agent will be downloaded, validated, and installed to ~/.atmos/agents/.
You can then use the agent in the AI TUI by switching to it with Ctrl+A.

Source formats:
  github.com/user/repo              Install latest version from main branch
  github.com/user/repo@v1.2.3       Install specific version tag
  github.com/user/repo@branch       Install from specific branch
  https://github.com/user/repo.git  Full HTTPS URL

Security:
  - Agents cannot execute arbitrary code
  - Tool access is explicitly declared in agent metadata
  - You will be prompted to confirm installation before proceeding
  - Use --yes to skip confirmation (for automation)

Examples:
  # Install the latest version of an agent
  atmos ai agent install github.com/cloudposse/atmos-agent-terraform

  # Install a specific version
  atmos ai agent install github.com/cloudposse/atmos-agent-terraform@v1.2.3

  # Force reinstall (overwrite existing installation)
  atmos ai agent install github.com/cloudposse/atmos-agent-terraform --force

  # Skip confirmation prompt
  atmos ai agent install github.com/cloudposse/atmos-agent-terraform --yes`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.aiAgentInstallCmd")()

		source := args[0]

		// Get flags.
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			return fmt.Errorf("failed to get --force flag: %w", err)
		}

		skipConfirm, err := cmd.Flags().GetBool("yes")
		if err != nil {
			return fmt.Errorf("failed to get --yes flag: %w", err)
		}

		// Create installer.
		installer, err := marketplace.NewInstaller(version.Version)
		if err != nil {
			return fmt.Errorf("failed to initialize installer: %w", err)
		}

		// Install agent.
		opts := marketplace.InstallOptions{
			Force:       force,
			SkipConfirm: skipConfirm,
		}

		ctx := context.Background()
		if err := installer.Install(ctx, source, opts); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	// Add flags.
	aiAgentInstallCmd.Flags().Bool("force", false, "Reinstall if agent is already installed")
	aiAgentInstallCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Add 'install' subcommand to 'agent' command.
	aiAgentCmd.AddCommand(aiAgentInstallCmd)
}
