package skill

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	ai "github.com/cloudposse/atmos/cmd/ai"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// listCmd represents the 'atmos ai skill list' command.
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed skills",
	Long: `List all community-contributed skills installed on this system.

Shows skill name, version, source, and installation status.
Skills are stored in ~/.atmos/skills/ and tracked in registry.json.

Examples:
  # List all installed skills
  atmos ai skill list

  # List with detailed output
  atmos ai skill list --detailed`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.aiSkillListCmd")()

		// Get flags.
		detailed, err := cmd.Flags().GetBool("detailed")
		if err != nil {
			return fmt.Errorf("failed to get --detailed flag: %w", err)
		}

		// Create installer (which manages registry).
		installer, err := marketplace.NewInstaller(version.Version)
		if err != nil {
			return fmt.Errorf("failed to initialize installer: %w", err)
		}

		// Get installed skills.
		skills := installer.List()

		if len(skills) == 0 {
			fmt.Println("No skills installed.")
			fmt.Println("\nInstall a skill with:")
			fmt.Println("  atmos ai skill install github.com/user/skill-name")
			return nil
		}

		// Print header.
		fmt.Printf("Installed skills (%d):\n\n", len(skills))

		// Print skills.
		for _, skill := range skills {
			if detailed {
				printSkillDetailed(skill)
			} else {
				printSkillSummary(skill)
			}
		}

		fmt.Println("\nUse 'Ctrl+A' in the AI TUI to switch between skills.")

		return nil
	},
}

func init() {
	// Add flags.
	listCmd.Flags().BoolP("detailed", "d", false, "Show detailed information for each skill")

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
