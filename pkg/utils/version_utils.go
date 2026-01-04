package utils

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/version"
)

// PrintMessageToUpgradeToAtmosLatestRelease prints info on how to upgrade Atmos to the latest version.
func PrintMessageToUpgradeToAtmosLatestRelease(latestVersion string) {
	defer perf.Track(nil, "utils.PrintMessageToUpgradeToAtmosLatestRelease")()

	// Get current theme styles that respect the active color profile.
	styles := theme.GetCurrentStyles()

	// Define content.
	message := lipgloss.NewStyle().
		Render(fmt.Sprintf("Update available! %s » %s",
			styles.VersionNumber.Render(version.Version),
			styles.NewVersion.Render(latestVersion)))

	// Build the install command suggestion.
	installCmd := fmt.Sprintf("atmos version install %s", latestVersion)

	lines := []string{
		lipgloss.NewStyle().Render(fmt.Sprintf("Run: %s", styles.Command.Render(installCmd))),
		lipgloss.NewStyle().Render(fmt.Sprintf("More info: %s", styles.Link.Render("https://atmos.tools/install"))),
	}

	messageLines := append([]string{message}, lines...)
	messageContent := strings.Join(messageLines, "\n")

	// Define box
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.ColorGreen)).
		Padding(0, 1).
		Align(lipgloss.Center)

	// Render the box
	box := style.Render(messageContent)

	// Print the box
	fmt.Println(box)
}
