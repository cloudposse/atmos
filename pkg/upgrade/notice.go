// Package upgrade renders installation-aware Atmos update notices.
package upgrade

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/installer"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// PrintNotice displays upgrade guidance on the UI channel (stderr).
func PrintNotice(currentVersion, latestVersion string, hint installer.Hint) {
	defer perf.Track(nil, "upgrade.PrintNotice")()

	ui.Writeln(renderNotice(currentVersion, latestVersion, hint))
}

func renderNotice(currentVersion, latestVersion string, hint installer.Hint) string {
	// Get current theme styles that respect the active color profile.
	styles := theme.GetCurrentStyles()
	lines := []string{fmt.Sprintf("Update available! %s » %s",
		styles.VersionNumber.Render(currentVersion), styles.NewVersion.Render(latestVersion))}
	if hint.Command != "" {
		if hint.Condition != "" {
			lines = append(lines, hint.Condition+":")
		}
		lines = append(lines, "Run: "+styles.Command.Render(hint.Command))
	}
	if hint.Message != "" {
		lines = append(lines, hint.Message)
	}
	if hint.URL != "" {
		lines = append(lines, "More info: "+styles.Link.Render(hint.URL))
	}
	// Retain the rounded update box and theme's success color.
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.GetCurrentColorScheme().Success)).
		Padding(0, 1).
		Align(lipgloss.Center).
		Render(strings.Join(lines, "\n"))
}
