package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	atmosui "github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	defaultTermWidth = 120

	// Terraform action constants.
	actionCreate = "create"
	actionRead   = "read"
	actionUpdate = "update"
	actionDelete = "delete"

	// Format string constants.
	fmtDurationSuffix = " (%.1fs)"
)

// getCompletedResources returns resources in completed or error state.
func (m *Model) getCompletedResources() []*ResourceOperation {
	var completed []*ResourceOperation
	for _, res := range m.tracker.GetResources() {
		if res.State == ResourceStateComplete || res.State == ResourceStateError {
			completed = append(completed, res)
		}
	}
	return completed
}

// progressView renders the in-progress state.
func (m *Model) progressView() string {
	var b strings.Builder

	b.WriteString(m.progressHeaderLine())
	b.WriteString("\n\n")

	// Render only completed/errored resources (in-progress ones are shown on the header line).
	resources := m.tracker.GetResources()
	for _, res := range resources {
		if res.State == ResourceStateComplete || res.State == ResourceStateError {
			b.WriteString(m.renderResource(res))
			b.WriteString(newlineStr)
		}
	}

	return b.String()
}

// progressHeaderLine builds the spinner + command + activity + right-aligned progress info line.
func (m *Model) progressHeaderLine() string {
	// Styles.
	stackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan)).Bold(true)
	componentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))

	// Build spinner + command + stack/component.
	spin := m.spinner.View() + " "
	commandInfo := fmt.Sprintf(
		"%s %s/%s",
		m.command,
		stackStyle.Render(m.stack),
		componentStyle.Render(m.component),
	)

	// Add current activity (e.g., "Reading data.http.weather").
	activityInfo := ""
	if currentOp := m.tracker.GetCurrentActivity(); currentOp != nil {
		activityVerb := m.formatActivityVerb(currentOp)
		opElapsed := m.clock.Since(currentOp.StartTime).Seconds()
		activityInfo = mutedStyle.Render(fmt.Sprintf(" %s %s (%.1fs)", activityVerb, currentOp.Address, opElapsed))
	}

	progressInfo := m.progressCountInfo()

	// Calculate available width and build inline layout.
	// Layout: spinner + commandInfo + activityInfo + gap + progressInfo.
	width := m.width
	if width == 0 {
		width = defaultTermWidth // Default width if not set.
	}

	leftPart := spin + commandInfo + activityInfo
	leftWidth := lipgloss.Width(leftPart)
	rightWidth := lipgloss.Width(progressInfo)

	// Calculate gap to right-align the progress bar.
	gap := ""
	cellsRemaining := width - leftWidth - rightWidth
	if cellsRemaining > 0 {
		gap = strings.Repeat(" ", cellsRemaining)
	}

	return leftPart + gap + progressInfo
}

// progressCountInfo returns the "<bar> completed/total" text when the total is known,
// or plain elapsed seconds otherwise.
func (m *Model) progressCountInfo() string {
	total := m.tracker.GetTotalCount()
	completed := m.tracker.GetCompletedCount()

	if total > 0 {
		percent := float64(completed) / float64(total)
		progressBar := m.progress.ViewAs(percent)
		return fmt.Sprintf("%s %d/%d", progressBar, completed, total)
	}

	elapsed := m.clock.Since(m.startTime).Seconds()
	return fmt.Sprintf("%.1fs", elapsed)
}

// formatActivityVerb returns a short verb describing the current activity.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m Model) formatActivityVerb(op *ResourceOperation) string {
	switch op.State {
	case ResourceStateRefreshing:
		return "Reading"
	case ResourceStateInProgress:
		switch op.Action {
		case actionCreate:
			return "Creating"
		case actionUpdate:
			return "Updating"
		case actionDelete:
			return "Destroying"
		case actionRead:
			return "Reading"
		default:
			return "Processing"
		}
	default:
		return "Processing"
	}
}

// renderResource renders a single resource line.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m Model) renderResource(res *ResourceOperation) string {
	var icon string
	var actionVerb string
	var style lipgloss.Style

	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorYellow))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))

	switch res.State {
	case ResourceStatePending:
		icon = "○"
		actionVerb = m.formatActionPending(res.Action)
		style = mutedStyle
	case ResourceStateRefreshing:
		icon = m.spinner.View()
		actionVerb = "Refreshing"
		style = warningStyle
	case ResourceStateInProgress:
		icon = m.spinner.View()
		actionVerb = m.formatActionInProgress(res.Action)
		style = warningStyle
	case ResourceStateComplete:
		icon = "✓"
		actionVerb = m.formatActionComplete(res.Action)
		style = successStyle
	case ResourceStateError:
		icon = "✗"
		actionVerb = "Failed"
		style = errorStyle
	}

	// Build timing info.
	var timingStr string
	switch res.State {
	case ResourceStateInProgress, ResourceStateRefreshing:
		elapsed := m.clock.Since(res.StartTime).Seconds()
		timingStr = fmt.Sprintf(fmtDurationSuffix, elapsed)
	case ResourceStateComplete, ResourceStateError:
		if res.ElapsedSecs > 0 {
			timingStr = fmt.Sprintf(fmtDurationSuffix, float64(res.ElapsedSecs))
		} else if !res.EndTime.IsZero() {
			timingStr = fmt.Sprintf(fmtDurationSuffix, res.EndTime.Sub(res.StartTime).Seconds())
		}
	}

	return fmt.Sprintf(
		"  %s %s %s%s",
		style.Render(icon),
		style.Render(actionVerb),
		res.Address,
		mutedStyle.Render(timingStr),
	)
}

// formatActionPending formats the pending action verb.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m Model) formatActionPending(action string) string {
	switch action {
	case actionCreate:
		return "Create"
	case actionRead:
		return "Read"
	case actionUpdate:
		return "Update"
	case actionDelete:
		return "Destroy"
	case "no-op":
		return "No change"
	default:
		return action
	}
}

// formatActionInProgress formats the in-progress action verb.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m Model) formatActionInProgress(action string) string {
	switch action {
	case actionCreate:
		return "Creating"
	case actionRead:
		return "Reading"
	case actionUpdate:
		return "Updating"
	case actionDelete:
		return "Destroying"
	case "no-op":
		return "No change"
	default:
		return action
	}
}

// formatActionComplete formats the completed action verb.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m Model) formatActionComplete(action string) string {
	switch action {
	case actionCreate:
		return "Created"
	case actionRead:
		return "Read"
	case actionUpdate:
		return "Updated"
	case actionDelete:
		return "Destroyed"
	case "no-op":
		return "No change"
	default:
		return action
	}
}

// finalView renders the completion state.
func (m *Model) finalView() string {
	var b strings.Builder
	elapsed := m.clock.Since(m.startTime).Seconds()

	command, summary := m.finalCommandAndSummary()

	// Condensed summary.
	// Note: Diagnostic details (errors/warnings from terraform) are shown via LogDiagnostics() after the TUI completes.
	// But failed resources (apply errors with address info) are shown inline below.
	switch {
	case m.cancelled:
		b.WriteString(atmosui.FormatWarningf(
			"%s `%s/%s` cancelled",
			command,
			m.stack,
			m.component,
		))
		b.WriteString(newlineStr)
	case m.tracker.HasErrors():
		m.renderErrorSummary(&b, command, elapsed)
	default:
		m.renderSuccessSummary(&b, command, summary, elapsed)
	}

	return b.String()
}

// finalCommandAndSummary determines the display command name (showing "Destroy" instead
// of "Apply" when an apply only contains deletions) along with the change summary.
func (m *Model) finalCommandAndSummary() (string, *ChangeSummaryMessage) {
	command := capitalizeCommand(m.command)
	summary := m.tracker.GetChangeSummary()
	if m.command == "apply" && summary != nil &&
		summary.Changes.Remove > 0 && summary.Changes.Add == 0 && summary.Changes.Change == 0 {
		command = "Destroy"
	}
	return command, summary
}

// renderErrorSummary writes the failure summary line and the list of failed resources.
func (m *Model) renderErrorSummary(b *strings.Builder, command string, elapsed float64) {
	errorCount := m.tracker.GetErrorCount()
	b.WriteString(atmosui.FormatErrorf(
		"%s `%s/%s` failed: %d error(s) (%.1fs)",
		command,
		m.stack,
		m.component,
		errorCount,
		elapsed,
	))
	b.WriteString(newlineStr)

	// Show failed resources (different from diagnostics - these have resource addresses).
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))
	for _, res := range m.tracker.GetResources() {
		if res.State == ResourceStateError && res.Error != "" {
			fmt.Fprintf(
				b, "  %s %s: %s\n",
				errorStyle.Render("✗"),
				res.Address,
				res.Error,
			)
		}
	}
}

// renderSuccessSummary writes the completion summary line, noting when there were no changes.
func (m *Model) renderSuccessSummary(b *strings.Builder, command string, summary *ChangeSummaryMessage, elapsed float64) {
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))
	noChanges := summary != nil && summary.Changes.Add == 0 && summary.Changes.Change == 0 && summary.Changes.Remove == 0

	if noChanges {
		// No changes - include in markdown for bold rendering.
		b.WriteString(atmosui.FormatSuccessf(
			"%s `%s/%s` completed (*no changes*)",
			command,
			m.stack,
			m.component,
		))
	} else {
		b.WriteString(atmosui.FormatSuccessf(
			"%s `%s/%s` completed",
			command,
			m.stack,
			m.component,
		))
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf(fmtDurationSuffix, elapsed)))
	b.WriteString(newlineStr)
}

// capitalizeCommand returns the command with the first letter capitalized.
func capitalizeCommand(cmd string) string {
	if len(cmd) == 0 {
		return cmd
	}
	return strings.ToUpper(cmd[:1]) + cmd[1:]
}
