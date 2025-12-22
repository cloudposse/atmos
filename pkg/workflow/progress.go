package workflow

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/workflow/step"
)

const (
	progressBarWidth = 30
	defaultWidth     = 80
)

// ProgressRenderer handles progress bar rendering for workflow execution.
// It displays a right-aligned progress bar showing step completion status.
// Each step gets its own progress line showing current position in the workflow.
type ProgressRenderer struct {
	progress progress.Model
	total    int
	current  int
	stepName string
	width    int
	enabled  bool
	styles   *theme.StyleSet
	term     terminal.Terminal
}

// NewProgressRenderer creates a new progress renderer for workflow execution.
// Returns nil if progress display is disabled or terminal doesn't support TTY.
func NewProgressRenderer(workflow *schema.WorkflowDefinition, totalSteps int) *ProgressRenderer {
	// Check if progress is enabled at workflow level.
	showCfg := step.GetShowConfig(nil, workflow)
	if !step.ShowProgress(showCfg) {
		return nil
	}

	// Check for TTY support.
	term := terminal.New()
	if !term.IsTTY(terminal.Stderr) {
		return nil
	}

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(progressBarWidth),
		progress.WithoutPercentage(),
	)

	return &ProgressRenderer{
		progress: p,
		total:    totalSteps,
		enabled:  true,
		styles:   theme.GetCurrentStyles(),
		term:     term,
	}
}

// Update updates the progress to the current step.
func (r *ProgressRenderer) Update(current int, stepName string) {
	if r == nil || !r.enabled {
		return
	}
	r.current = current
	r.stepName = stepName
}

// Render renders the progress bar with just the step name on the left.
// Format: stepName ... [████░░░] n/m
// Deprecated: Use RenderWithLabel for combined step label + progress.
func (r *ProgressRenderer) Render() {
	if r == nil || !r.enabled {
		return
	}

	output := r.formatProgressLine("")
	_ = ui.Writeln(output)
}

// RenderWithLabel renders the progress bar with the step label on the left.
// Format: [1/5] stepname ... [████░░░] n/m
// Renders WITHOUT newline for in-place display. Call ui.ClearLine() before
// any step output, then RenderPermanent() after step completes.
func (r *ProgressRenderer) RenderWithLabel(stepLabel string) {
	if r == nil || !r.enabled {
		return
	}

	output := r.formatProgressLine(stepLabel)
	_ = ui.Write(output) // No newline - cursor stays on this line.
}

// RenderPermanent renders the progress bar WITH newline as a permanent record.
// Call this after step execution completes to preserve the progress line.
func (r *ProgressRenderer) RenderPermanent(stepLabel string) {
	if r == nil || !r.enabled {
		return
	}

	output := r.formatProgressLine(stepLabel)
	_ = ui.Writeln(output) // With newline - becomes permanent record.
}

// formatProgressLine creates the progress line with right-aligned progress bar.
// If stepLabel is provided, it's used as the left part. Otherwise, uses step name.
func (r *ProgressRenderer) formatProgressLine(stepLabel string) string {
	// Get terminal width.
	width := r.term.Width(terminal.Stderr)
	if width == 0 {
		width = defaultWidth
	}
	r.width = width

	// Format step count: " n/m".
	n := r.total
	w := lipgloss.Width(fmt.Sprintf("%d", n))
	stepCount := fmt.Sprintf(" %*d/%*d", w, r.current, w, n)

	// Get progress bar view.
	percent := 0.0
	if r.total > 0 {
		percent = float64(r.current) / float64(r.total)
	}
	prog := r.progress.ViewAs(percent)

	// Use provided label or fall back to step name.
	leftPart := stepLabel
	if leftPart == "" {
		if r.styles != nil {
			leftPart = r.styles.PackageName.Render(r.stepName)
		} else {
			leftPart = r.stepName
		}
	}

	// Calculate remaining space for gap.
	rightPart := prog + stepCount
	leftWidth := lipgloss.Width(leftPart)
	rightWidth := lipgloss.Width(rightPart)
	cellsAvail := r.width - leftWidth - rightWidth

	if cellsAvail < 0 {
		cellsAvail = 0
	}
	gap := strings.Repeat(" ", cellsAvail)

	return leftPart + gap + rightPart
}

// Done marks the progress as complete.
// This can be used to clean up or render a final state.
func (r *ProgressRenderer) Done() {
	if r == nil || !r.enabled {
		return
	}
	r.enabled = false
}

// IsEnabled returns whether progress rendering is enabled.
func (r *ProgressRenderer) IsEnabled() bool {
	return r != nil && r.enabled
}
