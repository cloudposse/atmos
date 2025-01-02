package utils

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/cloudposse/atmos/pkg/version"
)

// PrintMessageToUpgradeToAtmosLatestRelease prints info on how to upgrade Atmos to the latest version
func PrintMessageToUpgradeToAtmosLatestRelease(latestVersion string) {
	// Define colors
	c1 := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	c2 := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	c3 := lipgloss.NewStyle().Foreground(lipgloss.Color("14"))

	// Define content
	message := lipgloss.NewStyle().
		Render(fmt.Sprintf("Update available! %s » %s",
			c1.Render(version.Version),
			c2.Render(latestVersion)))

	links := []string{lipgloss.NewStyle().Render(fmt.Sprintf("Atmos Releases: %s", c3.Render("https://github.com/cloudposse/atmos/releases"))),
		lipgloss.NewStyle().Render(fmt.Sprintf("Install Atmos: %s", c3.Render("https://atmos.tools/install"))),
	}

	messageLines := append([]string{message}, links...)
	messageContent := strings.Join(messageLines, "\n")

	// Define box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("10")).
		Padding(0, 1).
		Align(lipgloss.Center)

	// Render the box
	box := boxStyle.Render(messageContent)

	// Print the box
	fmt.Println(box)
}
