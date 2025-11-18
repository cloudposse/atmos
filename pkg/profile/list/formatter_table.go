package list

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/profile"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// Table column widths.
	nameWidth     = 20
	locationWidth = 15
	pathWidth     = 60
	filesWidth    = 10

	// Display constants.
	newline = "\n"
)

// RenderTable renders profiles as a formatted table.
func RenderTable(profiles []profile.ProfileInfo) (string, error) {
	defer perf.Track(nil, "profile.list.RenderTable")()

	var output strings.Builder

	// Create section header style.
	sectionHeaderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.GetPrimaryColor())).
		Bold(true).
		Underline(true)

	// Handle empty result.
	if len(profiles) == 0 {
		styles := theme.GetCurrentStyles()
		output.WriteString(styles.Notice.Render("No profiles configured."))
		output.WriteString(newline)
		return output.String(), nil
	}

	// Render profiles table.
	profileTable, err := createProfilesTable(profiles)
	if err != nil {
		return "", err
	}

	output.WriteString(sectionHeaderStyle.Render("PROFILES"))
	output.WriteString(newline)
	output.WriteString(profileTable.View())
	output.WriteString(newline)

	return output.String(), nil
}

// createProfilesTable creates a table for profiles.
func createProfilesTable(profiles []profile.ProfileInfo) (table.Model, error) {
	defer perf.Track(nil, "profile.list.createProfilesTable")()

	columns := []table.Column{
		{Title: "NAME", Width: nameWidth},
		{Title: "LOCATION", Width: locationWidth},
		{Title: "PATH", Width: pathWidth},
		{Title: "FILES", Width: filesWidth},
	}

	rows := buildProfileRows(profiles)

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(len(rows)),
	)

	applyTableStyles(&t)

	return t, nil
}

// buildProfileRows builds table rows from profiles.
func buildProfileRows(profiles []profile.ProfileInfo) []table.Row {
	rows := make([]table.Row, 0, len(profiles))

	// Sort profiles by name.
	sortedProfiles := make([]profile.ProfileInfo, len(profiles))
	copy(sortedProfiles, profiles)
	sort.Slice(sortedProfiles, func(i, j int) bool {
		return sortedProfiles[i].Name < sortedProfiles[j].Name
	})

	for _, p := range sortedProfiles {
		// Truncate path if too long.
		displayPath := p.Path
		if len(displayPath) > pathWidth {
			displayPath = "..." + displayPath[len(displayPath)-pathWidth+3:]
		}

		// File count.
		fileCount := "0"
		if len(p.Files) > 0 {
			fileCount = string(rune('0' + len(p.Files)))
			if len(p.Files) >= 10 {
				fileCount = "10+"
			}
		}

		rows = append(rows, table.Row{
			p.Name,
			p.LocationType,
			displayPath,
			fileCount,
		})
	}

	return rows
}

// applyTableStyles applies consistent theme styles to a table.
func applyTableStyles(t *table.Model) {
	borderColor := theme.GetBorderColor()

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color(theme.GetPrimaryColor())).
		Background(lipgloss.Color("")).
		Bold(false)

	t.SetStyles(s)
}
