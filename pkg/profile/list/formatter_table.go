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
	statusWidth   = 1
	nameWidth     = 20
	locationWidth = 15
	pathWidth     = 60
	filesWidth    = 10

	// Display constants.
	newline = "\n"

	// Active-profile marker placed unstyled in the table cell.
	// We deliberately put a plain `●` (visual width 1) into the cell so
	// bubbles' `runewidth.Truncate(value, width, "…")` doesn't mangle it —
	// runewidth counts ANSI escape bytes as visual width, so a pre-styled
	// `\x1b[…m●\x1b[0m` (visual width 1, runewidth-reported width ~8) gets
	// truncated to "…". The colour is applied as a post-render replacement
	// in `RenderTable` after bubbles is done with the row.
	activeMarker = "●"
)

// styledActiveDot returns the active-profile dot wrapped in green. Resolved at
// call time so lipgloss's colour profile is fully initialised (a package-level
// var would freeze NoColor at init(), before TTY detection runs).
// Matches the colour used by `atmos auth list`.
func styledActiveDot() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(activeMarker)
}

// RenderTable renders profiles as a formatted table. Profiles in activeProfiles
// are marked with a green dot in the leading status column.
func RenderTable(profiles []profile.ProfileInfo, activeProfiles map[string]bool) (string, error) {
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
	profileTable, err := createProfilesTable(profiles, activeProfiles)
	if err != nil {
		return "", err
	}

	// Apply colour to the active-profile dot AFTER bubbles renders. See
	// the `activeMarker` const comment for why we can't pre-style the cell.
	view := profileTable.View()
	if len(activeProfiles) > 0 {
		view = strings.ReplaceAll(view, activeMarker, styledActiveDot())
	}

	output.WriteString(sectionHeaderStyle.Render("PROFILES"))
	output.WriteString(newline)
	output.WriteString(view)
	output.WriteString(newline)

	return output.String(), nil
}

// createProfilesTable creates a table for profiles.
func createProfilesTable(profiles []profile.ProfileInfo, activeProfiles map[string]bool) (table.Model, error) {
	defer perf.Track(nil, "profile.list.createProfilesTable")()

	columns := []table.Column{
		{Title: "", Width: statusWidth}, // Active-profile indicator column.
		{Title: "NAME", Width: nameWidth},
		{Title: "LOCATION", Width: locationWidth},
		{Title: "PATH", Width: pathWidth},
		{Title: "FILES", Width: filesWidth},
	}

	rows := buildProfileRows(profiles, activeProfiles)

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(len(rows)+1), // +1 for header row.
	)

	applyTableStyles(&t)

	return t, nil
}

// buildProfileRows builds table rows from profiles.
func buildProfileRows(profiles []profile.ProfileInfo, activeProfiles map[string]bool) []table.Row {
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

		// Active indicator: plain `●` for active profiles, single space otherwise.
		// The dot is colourised post-render — see `activeMarker` const.
		indicator := " "
		if activeProfiles[p.Name] {
			indicator = activeMarker
		}

		rows = append(rows, table.Row{
			indicator,
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
	// Bubbles applies `s.Selected` to the entire joined row of the cursor
	// (row 0) even with `WithFocused(false)`. Defaults add bold+colour (the
	// unwanted highlight); aliasing to `s.Cell` would re-apply Padding(0,1)
	// on top of the per-cell padding, shifting the cursor row 1 column right
	// of every other row. Use an empty style so all rows render identically.
	s.Selected = lipgloss.NewStyle()

	t.SetStyles(s)
}
