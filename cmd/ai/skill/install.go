package skill

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	ai "github.com/cloudposse/atmos/cmd/ai"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// installCmd represents the 'atmos ai skill install' command.
var installCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install a skill from a GitHub repository",
	Long: `Install a community-contributed skill from a GitHub repository.

The skill will be downloaded, validated, and installed to ~/.atmos/skills/.
You can then use the skill in the AI TUI by switching to it with Ctrl+A.

Skills follow the Agent Skills open standard (https://agentskills.io)
and use the SKILL.md format with YAML frontmatter.

Source formats:
  github.com/user/repo              Install latest version from main branch
  github.com/user/repo@v1.2.3       Install specific version tag
  github.com/user/repo@branch       Install from specific branch
  https://github.com/user/repo.git  Full HTTPS URL

Security:
  - Skills cannot execute arbitrary code
  - Tool access is explicitly declared in skill metadata
  - You will be prompted to confirm installation before proceeding
  - Use --yes to skip confirmation (for automation)

Examples:
  # Install the latest version of a skill
  atmos ai skill install github.com/cloudposse/atmos-skill-terraform

  # Install a specific version
  atmos ai skill install github.com/cloudposse/atmos-skill-terraform@v1.2.3

  # Force reinstall (overwrite existing installation)
  atmos ai skill install github.com/cloudposse/atmos-skill-terraform --force

  # Skip confirmation prompt
  atmos ai skill install github.com/cloudposse/atmos-skill-terraform --yes`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.aiSkillInstallCmd")()

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

		// Install skill.
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
	installCmd.Flags().Bool("force", false, "Reinstall if skill is already installed")
	installCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Add 'install' subcommand to 'skill' command.
	ai.SkillCmd.AddCommand(installCmd)
}
