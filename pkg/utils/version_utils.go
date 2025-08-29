package utils

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/version"
)

// PrintMessageToUpgradeToAtmosLatestRelease prints info on how to upgrade Atmos to the latest version
func PrintMessageToUpgradeToAtmosLatestRelease(latestVersion string) {
	// Get current theme styles
	styles := theme.GetCurrentStyles()
	if styles == nil {
		// Fallback to basic output if styles aren't available
		fmt.Printf("Update available! %s » %s\n", version.Version, latestVersion)
		return
	}
	
	// Define content using dynamic theme styles
	message := lipgloss.NewStyle().
		Render(fmt.Sprintf("Update available! %s » %s",
			styles.VersionNumber.Render(version.Version),
			styles.NewVersion.Render(latestVersion)))

	links := []string{
		lipgloss.NewStyle().Render(fmt.Sprintf("Atmos Releases: %s", styles.Link.Render("https://github.com/cloudposse/atmos/releases"))),
		lipgloss.NewStyle().Render(fmt.Sprintf("Install Atmos: %s", styles.Link.Render("https://atmos.tools/install"))),
	}

	messageLines := append([]string{message}, links...)
	messageContent := strings.Join(messageLines, "\n")

	// Define box using theme-aware border color
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.GetSuccessColor())).
		Padding(0, 1).
		Align(lipgloss.Center)

	// Render the box
	box := style.Render(messageContent)

	// Print the box
	fmt.Println(box)
}
