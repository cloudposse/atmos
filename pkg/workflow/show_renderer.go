package workflow

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/workflow/step"
)

// ShowRenderer handles rendering of show features for workflows.
type ShowRenderer struct {
	styles     *theme.StyleSet
	headerDone bool // Track if header was already rendered.
}

// NewShowRenderer creates a new ShowRenderer.
func NewShowRenderer() *ShowRenderer {
	return &ShowRenderer{
		styles: theme.GetCurrentStyles(),
	}
}

// RenderHeaderIfNeeded renders the workflow header and flags if configured and not already rendered.
// This should be called before the first step executes.
func (r *ShowRenderer) RenderHeaderIfNeeded(
	workflow *schema.WorkflowDefinition,
	workflowName string,
	flags map[string]string,
) {
	if r.headerDone {
		return
	}

	showCfg := step.GetShowConfig(nil, workflow)

	// Render header if enabled (as markdown).
	if step.ShowHeader(showCfg) && workflow.Description != "" {
		header := r.formatHeader(workflowName, workflow.Description)
		_ = ui.Markdown(header)
	}

	// Render flags if enabled.
	if step.ShowFlags(showCfg) && len(flags) > 0 {
		flagsOutput := r.formatFlags(flags)
		_ = ui.Writeln(flagsOutput)
	}

	// Add a blank line after header/flags for visual separation.
	if (step.ShowHeader(showCfg) && workflow.Description != "") || (step.ShowFlags(showCfg) && len(flags) > 0) {
		_ = ui.Writeln("")
	}

	r.headerDone = true
}

// formatHeader creates a markdown header with workflow name and description.
func (r *ShowRenderer) formatHeader(name, description string) string {
	return fmt.Sprintf("# %s\n%s", name, description)
}

// formatFlags formats flag values for display.
// Flags are sorted alphabetically for consistent output.
func (r *ShowRenderer) formatFlags(flags map[string]string) string {
	if len(flags) == 0 {
		return ""
	}

	// Sort keys for consistent output.
	keys := make([]string, 0, len(flags))
	for k := range flags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, key := range keys {
		value := flags[key]
		if r.styles != nil {
			parts = append(parts, fmt.Sprintf("%s: %s",
				r.styles.Label.Render(key),
				r.styles.Body.Render(value)))
		} else {
			parts = append(parts, fmt.Sprintf("%s: %s", key, value))
		}
	}

	return strings.Join(parts, "  ")
}
