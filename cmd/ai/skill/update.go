package skill

import (
	"context"
	_ "embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	ai "github.com/cloudposse/atmos/cmd/ai"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// updateParser handles flag parsing with Viper precedence for the update command.
var updateParser *flags.StandardParser

//go:embed markdown/atmos_ai_skill_update.md
var updateLongMarkdown string

//go:embed markdown/atmos_ai_skill_update_usage.md
var updateUsageMarkdown string

// updateCmd represents the 'atmos ai skill update' command.
var updateCmd = &cobra.Command{
	Use:     "update [name]",
	Short:   "Update installed bundled skills to their latest catalog version",
	Long:    updateLongMarkdown,
	Example: updateUsageMarkdown,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.aiSkillUpdateCmd")()

		// Bind parsed flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := updateParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Reject an unsupported --client before doing any work; BindFlagsToViper
		// alone doesn't validate ValidValues (that only happens inside Parse()),
		// so this command validates explicitly, same as install/uninstall.
		if err := updateParser.ValidateFlagValues(cmd); err != nil {
			return err
		}

		skipConfirm := v.GetBool("yes")
		path := v.GetString("path")

		installer, err := marketplace.NewInstaller(version.Version)
		if err != nil {
			return fmt.Errorf("failed to initialize installer: %w", err)
		}

		basePath, err := os.Getwd()
		if err != nil {
			basePath = "."
		}

		// Same --path-takes-full-manual-control behavior as install: skip
		// resolving (and possibly prompting for) scope and clients.
		scope := v.GetString(scopeFlag)
		var clients []string
		if path == "" {
			scope, err = resolveSkillScope(cmd, v, skipConfirm)
			if err != nil {
				return err
			}
			clients, err = resolveSkillClients(basePath, v, skipConfirm, scope)
			if err != nil {
				return err
			}
		} else {
			warnIgnoredDistributionFlags(cmd)
		}

		opts := marketplace.InstallOptions{
			SkipConfirm: skipConfirm,
			Path:        path,
			BasePath:    basePath,
			Scope:       scope,
			Clients:     clients,
			AllClients:  v.GetBool("all-clients"),
		}

		// With no <name> given, update every installed bundled skill that has
		// a newer catalog version, mirroring install's "no source = act on
		// the whole bundled set" convention.
		if len(args) == 0 {
			return installer.UpdateAllBundled(&opts)
		}

		ctx := context.Background()
		return installer.UpdateSkill(ctx, args[0], &opts)
	},
}

func init() {
	// Create parser with update-specific flags, mirroring install's
	// distribution flags -- update re-runs the same install/distribution
	// logic under the hood once it's decided a reinstall is actually needed.
	updateParser = flags.NewStandardParser(
		flags.WithBoolFlag("yes", "y", false, "Skip confirmation prompt"),
		flags.WithEnvVars("yes", "ATMOS_AI_SKILL_YES"),
		flags.WithStringFlag("path", "", "", "Override the skill install directory (default: ~/.atmos/skills). Relative paths resolve against CWD."),
		flags.WithEnvVars("path", "ATMOS_AI_SKILL_PATH"),
		flags.WithStringSliceFlag(clientFlag, "c", nil, "AI client to distribute the updated skill to (repeatable): claude-code, vscode, gemini"),
		flags.WithEnvVars(clientFlag, "ATMOS_AI_SKILL_CLIENT"),
		flags.WithValidValues(clientFlag, marketplace.SupportedClients...),
		flags.WithBoolFlag("all-clients", "", false, "Distribute the updated skill to every supported AI client"),
		flags.WithEnvVars("all-clients", "ATMOS_AI_SKILL_ALL_CLIENTS"),
		flags.WithStringFlag(scopeFlag, "", marketplace.ScopeProject, "Distribution scope: project or user (wins over --global if both are set)"),
		flags.WithEnvVars(scopeFlag, "ATMOS_AI_SKILL_SCOPE"),
		flags.WithValidValues(scopeFlag, marketplace.ScopeProject, marketplace.ScopeUser),
		flags.WithBoolFlag("global", "g", false, "Alias for --scope user"),
		flags.WithEnvVars("global", "ATMOS_AI_SKILL_GLOBAL"),
	)

	// Register flags on the command.
	updateParser.RegisterFlags(updateCmd)

	// Bind flags to Viper for environment variable support.
	if err := updateParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add 'update' subcommand to 'skill' command.
	ai.SkillCmd.AddCommand(updateCmd)
}
