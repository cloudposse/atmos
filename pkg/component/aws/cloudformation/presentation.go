package cloudformation

import (
	"fmt"
	"strings"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/charmbracelet/lipgloss"

	listformat "github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// diffSummaryText labels the change preview separately from deployment outputs.
func diffSummaryText(stackName string, result *changeSetResult) string {
	if result.NoOp {
		return fmt.Sprintf("%s: no changes (changeset would be a no-op)", stackName)
	}
	rows := make([][]string, 0, len(result.Changes))
	for _, change := range result.Changes {
		if rc := change.ResourceChange; rc != nil {
			replacement := string(rc.Replacement)
			if replacement == "" {
				replacement = "—"
			}
			rows = append(rows, []string{string(rc.Action), stringValue(rc.LogicalResourceId), stringValue(rc.ResourceType), replacement})
		}
	}
	noun := "resources"
	if len(rows) == 1 {
		noun = "resource"
	}
	title := fmt.Sprintf("Changes for %s: %d %s", stackName, len(rows), noun)
	if len(rows) == 0 {
		return title
	}
	return title + strings.TrimRight(listformat.CreateStyledTable([]string{"Action", "Resource", "Type", "Replacement"}, rows), "\r\n") + "\n"
}

// dispatchStackEvents remembers the root status actually displayed, including a
// late terminal event, so the spinner does not repeat it as a second summary.
func dispatchStackEvents(sp *spinner.Spinner, events []cfntypes.StackEvent, stackName string, status cfntypes.StackStatus) cfntypes.StackStatus {
	for i := range events {
		dispatchStackEvent(sp, &events[i])
		if isStackTerminalEvent(&events[i], stackName) {
			status = cfntypes.StackStatus(events[i].ResourceStatus)
		}
	}
	return status
}

// formatLiveStackEvent follows Terraform's progress vocabulary. The change
// preview carries resource types; progress focuses on the action and resource.
// Unusual states and failure reasons retain the exact AWS status for diagnosis.
func formatLiveStackEvent(event *cfntypes.StackEvent) string {
	status := string(event.ResourceStatus)
	labels := map[string]string{
		"CREATE_IN_PROGRESS": "Creating", "CREATE_COMPLETE": "Created",
		"UPDATE_IN_PROGRESS": "Updating", "UPDATE_COMPLETE": "Updated",
		"DELETE_IN_PROGRESS": "Deleting", "DELETE_COMPLETE": "Deleted",
	}
	action, ok := labels[status]
	if !ok {
		action = status
	}
	line := fmt.Sprintf("%-9s %s", action, stringValue(event.LogicalResourceId))
	if reason := stringValue(event.ResourceStatusReason); reason != "" {
		line += " — " + reason
	}
	return line
}

// formatLiveStackEventResult reserves semantic color for the outcome, keeping
// identifiers plain like Terraform's completed resource rows.
func formatLiveStackEventResult(event *cfntypes.StackEvent, success bool) string {
	scheme := theme.GetCurrentColorScheme()
	color, icon := scheme.Success, "✓"
	if !success {
		color, icon = scheme.Error, "✗"
	}
	line := formatLiveStackEvent(event)
	action, detail, _ := strings.Cut(line, " ")
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	return "  " + style.Render(icon) + " " + style.Render(action) + " " + detail
}
