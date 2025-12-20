package step

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/pager"
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
	return &OutputModeWriter{
		mode:     mode,
		stepName: stepName,
		viewport: viewport,
	}
}

// Execute runs the command with the configured output mode.
func (w *OutputModeWriter) Execute(cmd *exec.Cmd) (string, string, error) {
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

	// Use MultiWriter to both capture and forward output.
	cmd.Stdout = io.MultiWriter(&stdout, os.Stdout)
	cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

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
	stepLabel := w.stepName
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
	styles := theme.GetCurrentStyles()
	var footer string
	if runErr != nil {
		if styles != nil {
			footer = styles.XMark.String() + " " + w.stepName + " failed"
		} else {
			footer = "✗ " + w.stepName + " failed"
		}
	} else {
		if styles != nil {
			footer = styles.Checkmark.String() + " " + w.stepName + " completed"
		} else {
			footer = "✓ " + w.stepName + " completed"
		}
	}
	_ = ui.Writeln(footer)

	return stdout, stderr, runErr
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

// StreamingOutputWriter handles real-time output streaming with prefix.
type StreamingOutputWriter struct {
	prefix string
	output *bytes.Buffer
	mu     sync.Mutex
	target io.Writer
}

// NewStreamingOutputWriter creates a writer that prefixes each line.
func NewStreamingOutputWriter(prefix string, target io.Writer) *StreamingOutputWriter {
	return &StreamingOutputWriter{
		prefix: prefix,
		output: &bytes.Buffer{},
		target: target,
	}
}

// Write implements io.Writer.
func (w *StreamingOutputWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Store in buffer.
	w.output.Write(p)

	// Write with prefix to target.
	if w.target != nil {
		_, _ = fmt.Fprintf(w.target, "%s %s", w.prefix, string(p))
	}

	return len(p), nil
}

// String returns the captured output.
func (w *StreamingOutputWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.output.String()
}
