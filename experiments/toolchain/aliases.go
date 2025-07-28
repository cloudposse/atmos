package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var aliasesCmd = &cobra.Command{
	Use:   "aliases",
	Short: "List configured tool aliases",
	Long: `List all configured tool aliases from the local tools.yaml configuration.

Aliases allow you to use short tool names (like 'terraform') instead of
full owner/repo paths (like 'hashicorp/terraform') in commands.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load local configuration
		lcm := NewLocalConfigManager()
		if err := lcm.Load(GetToolsConfigFilePath()); err != nil {
			return fmt.Errorf("failed to load local config: %w", err)
		}

		if lcm.config == nil || len(lcm.config.Aliases) == 0 {
			cmd.Println("No aliases configured.")
			return nil
		}

		// Get aliases and sort them for consistent output
		aliases := make([]string, 0, len(lcm.config.Aliases))
		for alias := range lcm.config.Aliases {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)

		// Format as table
		table := formatAliasesAsTable(aliases, lcm.config.Aliases)
		cmd.Print(table)

		return nil
	},
}

// formatAliasesAsTable formats aliases as a table using lipgloss
func formatAliasesAsTable(aliases []string, aliasMap map[string]string) string {
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).Width(20)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	var rows []string

	// Header
	rows = append(rows, labelStyle.Render("Alias:")+valueStyle.Render("Owner/Repo:"))

	// Aliases
	for _, alias := range aliases {
		ownerRepo := aliasMap[alias]
		rows = append(rows, labelStyle.Render(alias+":")+valueStyle.Render(ownerRepo))
	}

	return strings.Join(rows, "\n")
}
