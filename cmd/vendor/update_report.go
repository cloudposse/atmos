package vendor

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// updateReportHeadersWithReason are the column headers used when at least one row in the
// current (post --outdated/--archived-filter) result set carries a non-empty Reason
// (StatusSkipped or StatusFailed). CURRENT and LATEST are separate columns (rather than a single
// "CURRENT → LATEST" cell) so 40+ row tables stay scannable instead of cramming two versions into
// one cell. The leading blank-titled column holds only the colored status dot (matching the same
// "dot as its own first column" convention used by `atmos auth list`'s identity table and
// `atmos version list`'s createVersionTable); STATUS itself is a plain-text word.
var updateReportHeadersWithReason = []string{"", "COMPONENT", "STATUS", "CURRENT", "LATEST", "REASON"}

// updateReportHeadersNoReason omits the trailing REASON column entirely. Used when no row in the
// result set has a Reason: an always-empty column wastes horizontal width the CURRENT/LATEST
// split needs, so it's dropped rather than rendered blank.
var updateReportHeadersNoReason = []string{"", "COMPONENT", "STATUS", "CURRENT", "LATEST"}

// updateReportBorderColor matches the thin gray header-separator border used by
// `atmos version list` (cmd/version/formatters.go's createVersionTable).
const updateReportBorderColor = "8"

// updateReportHeaderStyle renders the table header exactly like `atmos version list`'s
// headerStyle: plain bold text with no theme color lookup. This is a deliberate, local
// rendering choice for this table only (not a change to any shared/theme-wide default) so
// vendor update's output reads as calm text with small colored accents, matching the rest of
// the CLI's restrained table style, rather than the previously loud, fully-saturated header.
var updateReportHeaderStyle = lipgloss.NewStyle().Bold(true)

// updateReportDotColumnWidth pins the leading blank-titled dot column's width, matching
// cmd/version/formatters.go's createVersionTable's indicatorColumnWidth: 1-char dot + 1 char
// padding each side. Without this, lipgloss/table's Width()-driven auto-expand distributes the
// extra width needed to fill the terminal disproportionately onto this narrow, content-light
// column, leaving a large gap between the dot and COMPONENT instead of spreading the extra
// width across the wider text columns where it belongs.
const updateReportDotColumnWidth = 3

// renderUpdateReport prints the per-source results as a table, followed by a summary line.
func renderUpdateReport(report *vendoring.UpdateReport, dryRun, outdated, archived bool) {
	headers, rows := buildUpdateReportRows(report.Results, outdated, archived)
	if len(rows) > 0 {
		// createUpdateReportTable already wraps the table with a leading blank line and a
		// trailing blank line (for visual separation), so use ui.Write (not Writeln)
		// here to avoid stacking an extra blank line before the summary.
		ui.Write(createUpdateReportTable(headers, rows))
	}
	renderUpdateSummary(report.UpdatedCount(), dryRun)
}

// createUpdateReportTable renders the vendor update report as a lipgloss table styled to match
// `atmos version list`'s restrained look (see cmd/version/formatters.go's createVersionTable):
// a plain bold header with no theme color, and a thin gray border under the header only. This is
// built directly with lipgloss/table (rather than pkg/list/format.CreateStyledTable, which bakes
// in a saturated theme-colored header used by list commands) since this is a one-off, local
// rendering choice for this specific table.
func createUpdateReportTable(headers []string, rows [][]string) string {
	t := table.New().
		Headers(headers...).
		Rows(rows...).
		BorderHeader(true).                                                                   // Show border under header.
		BorderTop(false).                                                                     // No top border.
		BorderBottom(false).                                                                  // No bottom border.
		BorderLeft(false).                                                                    // No left border.
		BorderRight(false).                                                                   // No right border.
		BorderRow(false).                                                                     // No row separators.
		BorderColumn(false).                                                                  // No column separators.
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(updateReportBorderColor))). // Gray border.
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case col == 0: // Dot column: pin the width so lipgloss/table's expand step doesn't stretch this 1-char column.
				return lipgloss.NewStyle().Padding(0, 1).Width(updateReportDotColumnWidth)
			case row == table.HeaderRow:
				return updateReportHeaderStyle.Padding(0, 1)
			default:
				return lipgloss.NewStyle().Padding(0, 1)
			}
		}).
		Width(templates.GetTerminalWidth())

	lineEnding := u.GetLineEnding()
	return lineEnding + t.String() + lineEnding + lineEnding
}

// updateReportRow holds one result's cell values before the REASON column's inclusion is
// decided, so buildUpdateReportRows can inspect every built row's reason (across the full,
// post-filter result set) before flattening to the final [][]string grid handed to
// createUpdateReportTable. Dot and status are kept separate (rather than combined into one
// cell) so the colored "●" dot renders in its own leading column, matching the convention used
// by `atmos auth list`'s identity table and `atmos version list`'s createVersionTable.
type updateReportRow struct {
	dot       string
	component string
	status    string
	current   string
	latest    string
	reason    string
}

// buildUpdateReportRows converts the report results into table headers+rows, applying the
// --outdated/--archived filtering the previous printf-based renderer used (see rowIncluded for
// the OR-based inclusion semantics). The REASON column is included only when at least one built
// row actually carries a reason (StatusSkipped/StatusFailed); otherwise it's dropped from both
// the headers and every row, since an always-empty column just wastes width the CURRENT/LATEST
// split needs.
func buildUpdateReportRows(results []vendoring.SourceUpdateResult, outdated, archivedOnly bool) ([]string, [][]string) {
	styles := theme.GetCurrentStyles()

	var built []updateReportRow
	hasReason := false
	for i := range results {
		row, ok := buildUpdateReportRow(&results[i], outdated, archivedOnly, styles)
		if !ok {
			continue
		}
		built = append(built, row)
		if row.reason != "" {
			hasReason = true
		}
	}

	headers := updateReportHeadersNoReason
	if hasReason {
		headers = updateReportHeadersWithReason
	}

	rows := make([][]string, 0, len(built))
	for _, row := range built {
		if hasReason {
			rows = append(rows, []string{row.dot, row.component, row.status, row.current, row.latest, row.reason})
		} else {
			rows = append(rows, []string{row.dot, row.component, row.status, row.current, row.latest})
		}
	}
	return headers, rows
}

// rowIncluded decides whether a result should appear in the rendered table, given the
// --outdated and --archived flags. The two flags combine with OR semantics (not AND): the user
// asked for "what's updated or archived" in one view, so setting both flags shows the union
// (StatusUpdated rows AND archived rows), not their intersection. Neither flag set means show
// everything (today's default, unchanged); only --outdated set keeps the pre-existing
// StatusUpdated-only behavior; only --archived set keeps every row whose upstream is archived
// regardless of Status (an archived-but-up-to-date or archived-but-skipped component still
// shows, since "the upstream is archived" is orthogonal to Status).
func rowIncluded(status vendoring.UpdateStatus, archived, outdatedOnly, archivedOnly bool) bool {
	if !outdatedOnly && !archivedOnly {
		return true
	}
	if outdatedOnly && status == vendoring.StatusUpdated {
		return true
	}
	if archivedOnly && archived {
		return true
	}
	return false
}

// buildUpdateReportRow renders a single result as a table row. It returns ok=false when the
// row should be omitted (filtered out by --outdated/--archived; see rowIncluded).
//
// Only the status dot is colored (a small accent, matching the same "●" colored-dot convention
// used by `atmos version list`/pkg/auth/list's status indicators and pkg/toolchain/list.go's
// gray installed-version dot); the status word itself ("Updated", "Up to date", etc.) is left in
// the default terminal foreground so the table reads as mostly plain text rather than a wall of
// saturated color.
func buildUpdateReportRow(r *vendoring.SourceUpdateResult, outdated, archivedOnly bool, styles *theme.StyleSet) (updateReportRow, bool) {
	if !rowIncluded(r.Status, r.Archived, outdated, archivedOnly) {
		return updateReportRow{}, false
	}

	switch r.Status {
	case vendoring.StatusUpdated:
		dot, word := statusCell(styles, &styles.Success, "Updated", r.Archived)
		return updateReportRow{
			dot:       dot,
			component: r.Component,
			status:    word,
			current:   formatVersionForDisplay(r.CurrentVersion),
			latest:    formatVersionForDisplay(r.LatestVersion),
		}, true
	case vendoring.StatusUpToDate:
		// The current version IS the latest allowed version; show it in both columns rather
		// than leaving LATEST blank, so the row itself confirms "this is already current".
		current := formatVersionForDisplay(r.CurrentVersion)
		dot, word := statusCell(styles, &styles.Info, "Up to date", r.Archived)
		return updateReportRow{
			dot:       dot,
			component: r.Component,
			status:    word,
			current:   current,
			latest:    current,
		}, true
	case vendoring.StatusSkipped:
		dot, word := statusCell(styles, &styles.Warning, "Skipped", r.Archived)
		return updateReportRow{
			dot:       dot,
			component: r.Component,
			status:    word,
			current:   formatVersionForDisplay(r.CurrentVersion),
			reason:    r.Reason,
		}, true
	case vendoring.StatusFailed:
		dot, word := statusCell(styles, &styles.Error, "Failed", r.Archived)
		return updateReportRow{
			dot:       dot,
			component: r.Component,
			status:    word,
			current:   formatVersionForDisplay(r.CurrentVersion),
			reason:    r.Reason,
		}, true
	default:
		return updateReportRow{}, false
	}
}

// statusCell renders a colored "●" dot (theme.IconActive) and a plain-text status word as two
// separate values, matching the convention already used by pkg/auth/list's status indicators and
// pkg/toolchain/list.go's gray installed-version dot: the dot goes in its own leading table
// column (rather than prefixing the STATUS cell), while STATUS itself stays plain text.
//
// An archived upstream repo (archived=true) overrides the dot to styles.Muted (gray) regardless
// of the underlying status, since "the upstream is archived" is an orthogonal fact from whether
// the component was updated/up-to-date/skipped/failed — a component can be both up to date AND
// archived upstream. "(archived)" is appended to the status word so it stays visible even when
// color is stripped (non-TTY output, --force-color=false, etc.).
func statusCell(styles *theme.StyleSet, style *lipgloss.Style, word string, archived bool) (dot, statusWord string) {
	if archived {
		style = &styles.Muted
		word += " (archived)"
	}
	return style.Render(theme.IconActive), word
}

// shortSHALength is the length a full git commit SHA is truncated to for table display,
// matching the conventional short-SHA length used by `git rev-parse --short`'s default.
const shortSHALength = 7

// formatVersionForDisplay shortens a version string for display in the update report table
// when it looks like a full git commit SHA (e.g. a component pinned to a commit rather than
// a tag). This is purely a display-time truncation: it never touches
// SourceUpdateResult.CurrentVersion/LatestVersion or anything written back to a manifest —
// only the table cell string built in buildUpdateReportRow is shortened.
func formatVersionForDisplay(v string) string {
	if len(v) <= shortSHALength {
		return v
	}
	if ci.IsCommitSHA(v) {
		return v[:shortSHALength]
	}
	return v
}

func renderUpdateSummary(n int, dryRun bool) {
	switch {
	case n == 0:
		ui.Info("No updates available.")
	case dryRun:
		ui.Successf("Found %d update(s) available.", n)
	default:
		ui.Successf("Updated %d component(s).", n)
	}
}
