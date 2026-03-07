package skill

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	ai "github.com/cloudposse/atmos/cmd/ai"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// installParser handles flag parsing with Viper precedence for the install command.
var installParser *flags.StandardParser

//go:embed markdown/atmos_ai_skill_install.md
var installLongMarkdown string

//go:embed markdown/atmos_ai_skill_install_usage.md
var installUsageMarkdown string

// installCmd represents the 'atmos ai skill install' command.
var installCmd = &cobra.Command{
	Use:     "install <source>",
	Short:   "Install skills from a GitHub repository",
	Long:    installLongMarkdown,
	Example: installUsageMarkdown,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.aiSkillInstallCmd")()

		source := args[0]

		// Bind parsed flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := installParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flags from Viper (supports CLI > ENV > config > defaults).
		force := v.GetBool("force")
		skipConfirm := v.GetBool("yes")

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
	// Create parser with install-specific flags using functional options.
	installParser = flags.NewStandardParser(
		flags.WithBoolFlag("force", "", false, "Reinstall if skill is already installed"),
		flags.WithBoolFlag("yes", "y", false, "Skip confirmation prompt"),
		flags.WithEnvVars("force", "ATMOS_AI_SKILL_FORCE"),
		flags.WithEnvVars("yes", "ATMOS_AI_SKILL_YES"),
	)

	// Register flags on the command.
	installParser.RegisterFlags(installCmd)

	// Bind flags to Viper for environment variable support.
	if err := installParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add 'install' subcommand to 'skill' command.
	ai.SkillCmd.AddCommand(installCmd)
}
