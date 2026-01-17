package step

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// OutputModeWriter wraps command execution with the specified output mode.
type OutputModeWriter struct {
	mode     OutputMode
	stepName string
	viewport *schema.ViewportConfig
}

// NewOutputModeWriter creates a new OutputModeWriter.
func NewOutputModeWriter(mode OutputMode, stepName string, viewport *schema.ViewportConfig) *OutputModeWriter {
	defer perf.Track(nil, "step.NewOutputModeWriter")()

	return &OutputModeWriter{
		mode:     mode,
		stepName: stepName,
		viewport: viewport,
	}
}

// Execute runs the command with the configured output mode.
func (w *OutputModeWriter) Execute(cmd *exec.Cmd) (string, string, error) {
	defer perf.Track(nil, "step.OutputModeWriter.Execute")()

	switch w.mode {
	case OutputModeViewport:
		return w.executeViewport(cmd)
	case OutputModeRaw:
		return w.executeRaw(cmd)
	case OutputModeLog:
		return w.executeLog(cmd)
	case OutputModeNone:
		return w.executeNone(cmd)
	default:
		// Default to log mode.
		return w.executeLog(cmd)
	}
}

// executeViewport captures output and displays in pager.
func (w *OutputModeWriter) executeViewport(cmd *exec.Cmd) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return stdout.String(), stderr.String(), err
	}

	// Check if terminal is available for viewport.
	term := terminal.New()
	if !term.IsTTY(terminal.Stdout) {
		// Fall back to log mode.
		return w.fallbackToLog(stdout.String(), stderr.String(), nil)
	}

	// Display in pager.
	content := stdout.String()
	if stderr.Len() > 0 {
		content += "\n--- stderr ---\n" + stderr.String()
	}

	p := pager.NewWithAtmosConfig(true)
	if pagerErr := p.Run(w.stepName, content); pagerErr != nil {
		// Pager failed, fall back to raw output.
		if writeErr := data.Write(content); writeErr != nil {
			return stdout.String(), stderr.String(), writeErr
		}
	}

	return stdout.String(), stderr.String(), nil
}

// executeRaw passes output directly to stdout/stderr.
func (w *OutputModeWriter) executeRaw(cmd *exec.Cmd) (string, string, error) {
	// Create writers that capture and forward output.
	var stdout, stderr bytes.Buffer

	// Get I/O context for stream access.
	ioCtx := iolib.GetContext()

	// Use MultiWriter to both capture and forward output.
	// Data() returns stdout for pipeable output, UI() returns stderr for human messages.
	cmd.Stdout = io.MultiWriter(&stdout, ioCtx.Data())
	cmd.Stderr = io.MultiWriter(&stderr, ioCtx.UI())

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// executeLog collects output and prints with step boundaries.
func (w *OutputModeWriter) executeLog(cmd *exec.Cmd) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Print step header.
	styles := theme.GetCurrentStyles()
	var stepLabel string
	if styles != nil {
		stepLabel = styles.Label.Render("[" + w.stepName + "]")
	} else {
		stepLabel = "[" + w.stepName + "]"
	}
	_ = ui.Writeln(stepLabel)

	err := cmd.Run()

	return w.fallbackToLog(stdout.String(), stderr.String(), err)
}

// fallbackToLog writes captured output with boundaries.
func (w *OutputModeWriter) fallbackToLog(stdout, stderr string, runErr error) (string, string, error) {
	// Print captured output.
	if stdout != "" {
		_ = data.Write(stdout)
	}
	if stderr != "" {
		_ = ui.Write(stderr)
	}

	// Print step footer with status.
	footer := w.formatStepFooter(runErr)
	_ = ui.Writeln(footer)

	return stdout, stderr, runErr
}

// formatStepFooter creates the footer string based on step status.
func (w *OutputModeWriter) formatStepFooter(runErr error) string {
	styles := theme.GetCurrentStyles()
	if runErr != nil {
		return w.formatFailedFooter(styles)
	}
	return w.formatSuccessFooter(styles)
}

// formatFailedFooter creates the footer for a failed step.
func (w *OutputModeWriter) formatFailedFooter(styles *theme.StyleSet) string {
	if styles != nil {
		return styles.XMark.String() + " " + w.stepName + " failed"
	}
	return "✗ " + w.stepName + " failed"
}

// formatSuccessFooter creates the footer for a successful step.
func (w *OutputModeWriter) formatSuccessFooter(styles *theme.StyleSet) string {
	if styles != nil {
		return styles.Checkmark.String() + " " + w.stepName + " completed"
	}
	return "✓ " + w.stepName + " completed"
}

// executeNone runs command silently.
func (w *OutputModeWriter) executeNone(cmd *exec.Cmd) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// GetOutputMode returns the effective output mode for a step.
// Checks step-level, workflow-level, and defaults.
func GetOutputMode(step *schema.WorkflowStep, workflow *schema.WorkflowDefinition) OutputMode {
	defer perf.Track(nil, "step.GetOutputMode")()

	// Step-level override.
	if step.Output != "" {
		return OutputMode(step.Output)
	}

	// Workflow-level default.
	if workflow != nil && workflow.Output != "" {
		return OutputMode(workflow.Output)
	}

	// Default to log mode.
	return OutputModeLog
}

// GetViewportConfig returns the effective viewport config for a step.
func GetViewportConfig(step *schema.WorkflowStep, workflow *schema.WorkflowDefinition) *schema.ViewportConfig {
	defer perf.Track(nil, "step.GetViewportConfig")()

	// Step-level override.
	if step.Viewport != nil {
		return step.Viewport
	}

	// Workflow-level default.
	if workflow != nil && workflow.Viewport != nil {
		return workflow.Viewport
	}

	return nil
}

// FormatStepLabel formats the step label with optional count prefix.
// If show.count is enabled, returns "[1/3] stepname" with stepname in muted style.
// Otherwise returns just "stepname" in muted style.
func FormatStepLabel(step *schema.WorkflowStep, workflow *schema.WorkflowDefinition, stepIndex, totalSteps int) string {
	defer perf.Track(nil, "step.FormatStepLabel")()

	showCfg := GetShowConfig(step, workflow)
	styles := theme.GetCurrentStyles()

	// Format step name in muted (dark gray) style.
	stepName := step.Name
	if styles != nil {
		stepName = styles.Muted.Render(step.Name)
	}

	if ShowCount(showCfg) && totalSteps > 0 {
		countPrefix := fmt.Sprintf("[%d/%d]", stepIndex+1, totalSteps)
		if styles != nil {
			countPrefix = styles.Label.Render(countPrefix)
		}
		return countPrefix + " " + stepName
	}
	return stepName
}

// RenderCommand renders the command before execution if show.command is enabled.
// Displays the command with a $ prefix for shell-like appearance.
func RenderCommand(step *schema.WorkflowStep, workflow *schema.WorkflowDefinition, command string) {
	defer perf.Track(nil, "step.RenderCommand")()

	showCfg := GetShowConfig(step, workflow)
	if !ShowCommand(showCfg) || command == "" {
		return
	}

	styles := theme.GetCurrentStyles()
	var cmdDisplay string
	if styles != nil {
		cmdDisplay = styles.Muted.Render("$ " + command)
	} else {
		cmdDisplay = "$ " + command
	}
	_ = ui.Writeln(cmdDisplay)
}

// StreamingOutputWriter handles real-time output streaming with prefix per line.
type StreamingOutputWriter struct {
	prefix     string
	output     *bytes.Buffer
	lineBuffer *bytes.Buffer // Buffer for incomplete lines.
	mu         sync.Mutex
	target     io.Writer
}

// NewStreamingOutputWriter creates a writer that prefixes each line.
func NewStreamingOutputWriter(prefix string, target io.Writer) *StreamingOutputWriter {
	defer perf.Track(nil, "step.NewStreamingOutputWriter")()

	return &StreamingOutputWriter{
		prefix:     prefix,
		output:     &bytes.Buffer{},
		lineBuffer: &bytes.Buffer{},
		target:     target,
	}
}

// Write implements io.Writer with line-buffering so prefix is applied per line.
func (w *StreamingOutputWriter) Write(p []byte) (n int, err error) {
	defer perf.Track(nil, "step.StreamingOutputWriter.Write")()

	w.mu.Lock()
	defer w.mu.Unlock()

	// Store in buffer.
	w.output.Write(p)

	// Process line by line for target output.
	if w.target == nil {
		return len(p), nil
	}

	w.processLines(string(p))

	return len(p), nil
}

// processLines handles line-buffered output with prefix. Must be called with lock held.
func (w *StreamingOutputWriter) processLines(input string) {
	lines := strings.Split(input, "\n")

	for i, line := range lines {
		isLastPart := (i == len(lines)-1)
		hasNewline := (i < len(lines)-1)

		if hasNewline {
			w.writeCompleteLine(line)
		} else if !isLastPart || line != "" {
			// Incomplete line or non-empty last part - buffer it.
			w.lineBuffer.WriteString(line)
		}
	}
}

// writeCompleteLine writes a complete line with prefix, flushing buffer first if needed. Must be called with lock held.
func (w *StreamingOutputWriter) writeCompleteLine(line string) {
	if w.lineBuffer.Len() > 0 {
		_, _ = fmt.Fprintf(w.target, "%s %s%s\n", w.prefix, w.lineBuffer.String(), line)
		w.lineBuffer.Reset()
	} else {
		_, _ = fmt.Fprintf(w.target, "%s %s\n", w.prefix, line)
	}
}

// Flush writes any buffered content to the target.
func (w *StreamingOutputWriter) Flush() {
	defer perf.Track(nil, "step.StreamingOutputWriter.Flush")()

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.target != nil && w.lineBuffer.Len() > 0 {
		_, _ = fmt.Fprintf(w.target, "%s %s", w.prefix, w.lineBuffer.String())
		w.lineBuffer.Reset()
	}
}

// String returns the captured output, flushing any buffered content first.
func (w *StreamingOutputWriter) String() string {
	defer perf.Track(nil, "step.StreamingOutputWriter.String")()

	// Flush buffered content before returning.
	w.Flush()

	w.mu.Lock()
	defer w.mu.Unlock()
	return w.output.String()
}
