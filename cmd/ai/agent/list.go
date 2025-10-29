package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	ai "github.com/cloudposse/atmos/cmd/ai"

	"github.com/cloudposse/atmos/pkg/ai/agents/marketplace"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// aiAgentListCmd represents the 'atmos ai agent list' command.
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed agents",
	Long: `List all community-contributed agents installed on this system.

Shows agent name, version, source, and installation status.
Agents are stored in ~/.atmos/agents/ and tracked in registry.json.

Examples:
  # List all installed agents
  atmos ai agent list

  # List with detailed output
  atmos ai agent list --detailed`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.aiAgentListCmd")()

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

		// Get installed agents.
		agents := installer.List()

		if len(agents) == 0 {
			fmt.Println("No agents installed.")
			fmt.Println("\nInstall an agent with:")
			fmt.Println("  atmos ai agent install github.com/user/agent-name")
			return nil
		}

		// Print header.
		fmt.Printf("Installed agents (%d):\n\n", len(agents))

		// Print agents.
		for _, agent := range agents {
			if detailed {
				printAgentDetailed(agent)
			} else {
				printAgentSummary(agent)
			}
		}

		fmt.Println("\nUse 'Ctrl+A' in the AI TUI to switch between agents.")

		return nil
	},
}

func init() {
	// Add flags.
	listCmd.Flags().BoolP("detailed", "d", false, "Show detailed information for each agent")

	// Add 'list' subcommand to 'agent' command.
	ai.AgentCmd.AddCommand(listCmd)
}

// printAgentSummary prints a one-line summary of an agent.
func printAgentSummary(agent *marketplace.InstalledAgent) {
	status := "✓"
	if !agent.Enabled {
		status = "✗"
	}

	fmt.Printf("%s %s\n", status, agent.DisplayName)
	fmt.Printf("  %s @ %s\n", agent.Source, agent.Version)
	fmt.Printf("\n")
}

// printAgentDetailed prints detailed information about an agent.
func printAgentDetailed(agent *marketplace.InstalledAgent) {
	// Header.
	status := "Enabled"
	if !agent.Enabled {
		status = "Disabled"
	}

	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("%s (%s)\n", agent.DisplayName, status)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Details.
	fmt.Printf("Name:         %s\n", agent.Name)
	fmt.Printf("Source:       %s\n", agent.Source)
	fmt.Printf("Version:      %s\n", agent.Version)
	fmt.Printf("Installed:    %s\n", formatTime(agent.InstalledAt))
	fmt.Printf("Last Updated: %s\n", formatTime(agent.UpdatedAt))
	fmt.Printf("Location:     %s\n", agent.Path)

	if agent.IsBuiltIn {
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
