package toolchain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
)

// listAliases handles the business logic for retrieving and formatting aliases.
func ListAliases() error {
	defer perf.Track(nil, "toolchain.ListAliases")()

	configFilePath := GetToolsConfigFilePath()
	// Load local configuration
	lcm := NewLocalConfigManager()
	if err := lcm.Load(configFilePath); err != nil {
		return fmt.Errorf("failed to load local config: %w", err)
	}

	aliasMap := lcm.GetAliases()
	if aliasMap == nil || len(aliasMap) == 0 {
		fmt.Println("No aliases configured.")
		return nil
	}

	// Get aliases and sort them for consistent output
	aliases := make([]string, 0, len(aliasMap))
	for alias := range aliasMap {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	fmt.Print(formatAliasesAsTable(aliases, aliasMap))
	// Format as table
	return nil
}

// formatAliasesAsTable formats aliases as a table using lipgloss.
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
