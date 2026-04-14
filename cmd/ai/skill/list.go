package skill

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	ai "github.com/cloudposse/atmos/cmd/ai"

	"github.com/cloudposse/atmos/pkg/ai/skills/embedded"
	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// listParser handles flag parsing with Viper precedence for the list command.
var listParser *flags.StandardParser

//go:embed markdown/atmos_ai_skill_list.md
var listLongMarkdown string

//go:embed markdown/atmos_ai_skill_list_usage.md
var listUsageMarkdown string

// listCmd represents the 'atmos ai skill list' command.
var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List installed skills",
	Long:    listLongMarkdown,
	Example: listUsageMarkdown,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.aiSkillListCmd")()

		// Bind parsed flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := listParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flags from Viper (supports CLI > ENV > config > defaults).
		detailed := v.GetBool("detailed")

		// Create installer (which manages registry).
		installer, err := marketplace.NewInstaller(version.Version)
		if err != nil {
			return fmt.Errorf("failed to initialize installer: %w", err)
		}

		// Get installed skills.
		installed := installer.List()

		// Get embedded (built-in) skills shipped with the Atmos binary.
		embeddedNames, _ := embedded.ListNames()

		// Hide any embedded skill that is also marketplace-installed — the
		// marketplace version overrides it, so listing both would be confusing.
		installedNames := make(map[string]struct{}, len(installed))
		for _, s := range installed {
			installedNames[s.Name] = struct{}{}
		}
		var builtIn []string
		for _, name := range embeddedNames {
			if _, overridden := installedNames[name]; overridden {
				continue
			}
			builtIn = append(builtIn, name)
		}

		if len(installed) == 0 && len(builtIn) == 0 {
			fmt.Println("No skills installed.")
			fmt.Println("\nInstall a skill with:")
			fmt.Println("  atmos ai skill install github.com/cloudposse/atmos//agent-skills/skills/atmos-terraform")
			fmt.Println("")
			fmt.Println("Browse all available skills:")
			fmt.Println("  https://github.com/cloudposse/atmos/tree/main/agent-skills/skills")
			return nil
		}

		// Print built-in (embedded) skills first — they're always available.
		if len(builtIn) > 0 {
			fmt.Printf("Built-in skills (%d):\n\n", len(builtIn))
			for _, name := range builtIn {
				printEmbeddedSummary(name, detailed)
			}
		}

		// Print marketplace-installed skills.
		if len(installed) > 0 {
			fmt.Printf("Installed skills (%d):\n\n", len(installed))
			for _, skill := range installed {
				if detailed {
					printSkillDetailed(skill)
				} else {
					printSkillSummary(skill)
				}
			}
		}

		fmt.Println("Use 'Ctrl+A' in the AI TUI to switch between skills.")

		return nil
	},
}

// printEmbeddedSummary prints a one-line summary of a built-in (embedded) skill.
// Uses embedded.Load for the description — keeps the list command source-of-truth.
func printEmbeddedSummary(name string, detailed bool) {
	skill, err := embedded.Load(name)
	if err != nil {
		// Extremely unlikely (embed is static), but degrade gracefully.
		fmt.Printf("✓ %s (built-in)\n\n", name)
		return
	}
	if detailed {
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("%s (Built-in)\n", skill.DisplayName)
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("Name:         %s\n", skill.Name)
		fmt.Printf("Description:  %s\n", skill.Description)
		fmt.Printf("Type:         Built-in (embedded in Atmos binary)\n\n")
		return
	}
	fmt.Printf("✓ %s\n  %s\n\n", skill.DisplayName, skill.Description)
}

func init() {
	// Create parser with list-specific flags using functional options.
	listParser = flags.NewStandardParser(
		flags.WithBoolFlag("detailed", "d", false, "Show detailed information for each skill"),
		flags.WithEnvVars("detailed", "ATMOS_AI_SKILL_DETAILED"),
	)

	// Register flags on the command.
	listParser.RegisterFlags(listCmd)

	// Bind flags to Viper for environment variable support.
	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add 'list' subcommand to 'skill' command.
	ai.SkillCmd.AddCommand(listCmd)
}

// printSkillSummary prints a one-line summary of a skill.
func printSkillSummary(skill *marketplace.InstalledSkill) {
	status := "✓"
	if !skill.Enabled {
		status = "✗"
	}

	fmt.Printf("%s %s\n", status, skill.DisplayName)
	fmt.Printf("  %s @ %s\n", skill.Source, skill.Version)
	fmt.Printf("\n")
}

// printSkillDetailed prints detailed information about a skill.
func printSkillDetailed(skill *marketplace.InstalledSkill) {
	// Header.
	status := "Enabled"
	if !skill.Enabled {
		status = "Disabled"
	}

	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("%s (%s)\n", skill.DisplayName, status)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Details.
	fmt.Printf("Name:         %s\n", skill.Name)
	fmt.Printf("Source:       %s\n", skill.Source)
	fmt.Printf("Version:      %s\n", skill.Version)
	fmt.Printf("Installed:    %s\n", formatTime(skill.InstalledAt))
	fmt.Printf("Last Updated: %s\n", formatTime(skill.UpdatedAt))
	fmt.Printf("Location:     %s\n", skill.Path)

	if skill.IsBuiltIn {
		fmt.Printf("Type:         Built-in\n")
	} else {
		fmt.Printf("Type:         Community\n")
	}

	fmt.Printf("\n")
}

// formatTime formats a time in a human-readable way.
func formatTime(t time.Time) string {
	const (
		hoursPerDay = 24
		daysPerWeek = 7
	)

	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < hoursPerDay*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < daysPerWeek*hoursPerDay*time.Hour:
		days := int(diff.Hours() / hoursPerDay)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		// More than a week ago, show date.
		return strings.TrimSpace(t.Format("Jan 2, 2006"))
	}
}
