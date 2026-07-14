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

// installParser handles flag parsing with Viper precedence for the install command.
var installParser *flags.StandardParser

//go:embed markdown/atmos_ai_skill_install.md
var installLongMarkdown string

//go:embed markdown/atmos_ai_skill_install_usage.md
var installUsageMarkdown string

// installCmd represents the 'atmos ai skill install' command.
var installCmd = &cobra.Command{
	Use:     "install [source]",
	Short:   "Install bundled or GitHub-hosted AI skills",
	Long:    installLongMarkdown,
	Example: installUsageMarkdown,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.aiSkillInstallCmd")()

		// Bind parsed flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := installParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flags from Viper (supports CLI > ENV > config > defaults).
		force := v.GetBool("force")
		skipConfirm := v.GetBool("yes")
		path := v.GetString("path")

		// Create installer.
		installer, err := marketplace.NewInstaller(version.Version)
		if err != nil {
			return fmt.Errorf("failed to initialize installer: %w", err)
		}

		basePath, err := os.Getwd()
		if err != nil {
			basePath = "."
		}

		// An explicit --path takes full manual control, so skip resolving
		// (and possibly prompting for) scope and clients to auto-distribute to.
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
		}

		// Install skill.
		opts := marketplace.InstallOptions{
			Force:       force,
			SkipConfirm: skipConfirm,
			Path:        path,
			BasePath:    basePath,
			Scope:       scope,
			Clients:     clients,
			AllClients:  v.GetBool("all-clients"),
		}

		// With no <source> given, install every bundled skill instead of just
		// one, mirroring `atmos mcp install` acting on every configured
		// server when no server names are given.
		if len(args) == 0 {
			return installer.InstallAllBundled(&opts)
		}

		ctx := context.Background()
		if err := installer.Install(ctx, args[0], opts); err != nil {
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
		flags.WithStringFlag("path", "", "", "Override the skill install directory (default: ~/.atmos/skills). Relative paths resolve against CWD, e.g. --path .github/skills for VS Code/Copilot auto-discovery."),
		flags.WithEnvVars("path", "ATMOS_AI_SKILL_PATH"),
		flags.WithStringSliceFlag("client", "c", nil, "AI client to distribute the skill to (repeatable): claude-code, vscode, gemini"),
		flags.WithEnvVars("client", "ATMOS_AI_SKILL_CLIENT"),
		flags.WithBoolFlag("all-clients", "", false, "Distribute the skill to all supported AI clients"),
		flags.WithEnvVars("all-clients", "ATMOS_AI_SKILL_ALL_CLIENTS"),
		flags.WithStringFlag(scopeFlag, "", marketplace.ScopeProject, "Distribution scope: project or user"),
		flags.WithEnvVars(scopeFlag, "ATMOS_AI_SKILL_SCOPE"),
		flags.WithValidValues(scopeFlag, marketplace.ScopeProject, marketplace.ScopeUser),
		flags.WithBoolFlag("global", "g", false, "Alias for --scope user"),
		flags.WithEnvVars("global", "ATMOS_AI_SKILL_GLOBAL"),
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
