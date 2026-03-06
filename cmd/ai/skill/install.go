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
	Short: "Install skills from a GitHub repository",
	Long: `Install AI skills from a GitHub repository.

Supports both single-skill repos (with SKILL.md at root) and multi-skill
packages (with skills/*/SKILL.md pattern). The official Atmos skills package
contains 21 specialized skills for infrastructure orchestration.

Skills will be downloaded, validated, and installed to ~/.atmos/skills/.
You can then use them in the AI TUI by switching with Ctrl+A.

Skills follow the Agent Skills open standard (https://agentskills.io)
and use the SKILL.md format with YAML frontmatter.

Source formats:
  user/repo                         GitHub shorthand (GitHub assumed)
  user/repo@v1.2.3                  Specific version tag
  github.com/user/repo              Full GitHub path
  github.com/user/repo@v1.2.3       Full path with version
  https://github.com/user/repo.git  Full HTTPS URL

Security:
  - Skills cannot execute arbitrary code
  - Tool access is explicitly declared in skill metadata
  - You will be prompted to confirm installation before proceeding
  - Use --yes to skip confirmation (for automation)

Examples:
  # Install all Atmos agent skills (21 skills)
  atmos ai skill install cloudposse/atmos

  # Install with a specific version
  atmos ai skill install cloudposse/atmos@v1.200.0

  # Install a third-party skill
  atmos ai skill install yourorg/your-skill

  # Force reinstall (overwrite existing installation)
  atmos ai skill install cloudposse/atmos --force

  # Skip confirmation prompt
  atmos ai skill install cloudposse/atmos --yes`,
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
