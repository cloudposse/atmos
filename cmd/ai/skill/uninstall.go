package skill

import (
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

// uninstallParser handles flag parsing with Viper precedence for the uninstall command.
var uninstallParser *flags.StandardParser

//go:embed markdown/atmos_ai_skill_uninstall.md
var uninstallLongMarkdown string

//go:embed markdown/atmos_ai_skill_uninstall_usage.md
var uninstallUsageMarkdown string

// uninstallCmd represents the 'atmos ai skill uninstall' command.
var uninstallCmd = &cobra.Command{
	Use:     "uninstall <name>",
	Short:   "Remove an installed skill",
	Long:    uninstallLongMarkdown,
	Example: uninstallUsageMarkdown,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.aiSkillUninstallCmd")()

		name := args[0]

		// Bind parsed flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := uninstallParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flags from Viper (supports CLI > ENV > config > defaults).
		force := v.GetBool("force")

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
	// Create parser with uninstall-specific flags using functional options.
	uninstallParser = flags.NewStandardParser(
		flags.WithBoolFlag("force", "f", false, "Skip confirmation prompt"),
		flags.WithEnvVars("force", "ATMOS_AI_SKILL_FORCE"),
	)

	// Register flags on the command.
	uninstallParser.RegisterFlags(uninstallCmd)

	// Bind flags to Viper for environment variable support.
	if err := uninstallParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add 'uninstall' subcommand to 'skill' command.
	ai.SkillCmd.AddCommand(uninstallCmd)
}
