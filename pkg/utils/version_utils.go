package utils

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/version"
)

// PrintMessageToUpgradeToAtmosLatestRelease prints info on how to upgrade Atmos to the latest version
func PrintMessageToUpgradeToAtmosLatestRelease(latestVersion string) {
	defer perf.Track(nil, "utils.PrintMessageToUpgradeToAtmosLatestRelease")()

	// Define content
	message := lipgloss.NewStyle().
		Render(fmt.Sprintf("Update available! %s » %s",
			theme.Styles.VersionNumber.Render(version.Version),
			theme.Styles.NewVersion.Render(latestVersion)))

	links := []string{
		lipgloss.NewStyle().Render(fmt.Sprintf("Atmos Releases: %s", theme.Styles.Link.Render("https://github.com/cloudposse/atmos/releases"))),
		lipgloss.NewStyle().Render(fmt.Sprintf("Install Atmos: %s", theme.Styles.Link.Render("https://atmos.tools/install"))),
	}

	messageLines := append([]string{message}, links...)
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
