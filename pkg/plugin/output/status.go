package output

import (
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

func isInteractiveTable(outputFormat string) bool {
	return outputFormat == string(format.FormatTable) && terminal.New().IsTTY(terminal.Stdout)
}

// statusIndicator uses the same dot and semantic theme styles as auth and list.
// Status words remain alongside the indicator so color is never the only signal.
func statusIndicator(state string) string {
	if terminal.New().ColorProfile() == terminal.ColorNone {
		return theme.IconActive
	}
	styles := theme.GetCurrentStyles()
	style := styles.Muted
	switch state {
	case "installed":
		style = styles.Success
	case "missing":
		style = styles.Warning
	case "path":
		style = styles.Info
	case "broken":
		style = styles.Error
	case "corrupt":
		style = styles.Error
	}
	return style.Render(theme.IconActive)
}

// reportHints tells the user how to resolve statuses that need attention. Hints go to
// the UI channel so machine-readable data on stdout stays clean.
func reportHints(statuses []manage.Status) {
	for index := range statuses {
		if statuses[index].Hint != "" {
			ui.Warning(statuses[index].Hint)
		}
	}
}
