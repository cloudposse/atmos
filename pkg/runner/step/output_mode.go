package step

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// OutputModeWriter wraps command execution with the specified output mode.
type OutputModeWriter struct {
	mode     OutputMode
	stepName string
	viewport *schema.ViewportConfig
	show     *schema.ShowConfig
	writers  OutputWriters
}

// NewOutputModeWriter creates a new OutputModeWriter.
func NewOutputModeWriter(mode OutputMode, stepName string, viewport *schema.ViewportConfig, show ...*schema.ShowConfig) *OutputModeWriter {
	defer perf.Track(nil, "step.NewOutputModeWriter")()

	var showCfg *schema.ShowConfig
	if len(show) > 0 {
		showCfg = show[0]
	}
	return &OutputModeWriter{
		mode:     mode,
		stepName: stepName,
		viewport: viewport,
		show:     showCfg,
	}
}

// Execute runs the command with the configured output mode.
func (w *OutputModeWriter) Execute(cmd *exec.Cmd) (string, string, error) {
	defer perf.Track(nil, "step.OutputModeWriter.Execute")()

	if w.writers.Stdout != nil || w.writers.Stderr != nil {
		return w.executeScoped(func(stdout, stderr io.Writer) error { cmd.Stdout, cmd.Stderr = stdout, stderr; return cmd.Run() })
	}
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

// ExecuteWithIO runs a command-like operation with the configured output mode.
// The runner receives stdout and stderr writers to attach to the operation.
func (w *OutputModeWriter) ExecuteWithIO(runner func(stdout, stderr io.Writer) error) (string, string, error) {
	defer perf.Track(nil, "step.OutputModeWriter.ExecuteWithIO")()

	if w.writers.Stdout != nil || w.writers.Stderr != nil {
		return w.executeScoped(runner)
	}
	switch w.mode {
	case OutputModeViewport:
		return w.executeViewportWithIO(runner)
	case OutputModeRaw:
		return w.executeRawWithIO(runner)
	case OutputModeLog:
		return w.executeLogWithIO(runner)
	case OutputModeNone:
		return w.executeNoneWithIO(runner)
	default:
		return w.executeLogWithIO(runner)
	}
}

// executeViewport runs a subprocess with the shared live output window.
func (w *OutputModeWriter) executeViewport(cmd *exec.Cmd) (string, string, error) {
	return w.executeViewportWithIO(func(stdout, stderr io.Writer) error {
		cmd.Stdout, cmd.Stderr = stdout, stderr
		return cmd.Run()
	})
}

// executeRaw passes output directly to stdout/stderr.
func (w *OutputModeWriter) executeRaw(cmd *exec.Cmd) (string, string, error) {
	return w.executeRawWithIO(func(stdout, stderr io.Writer) error {
		cmd.Stdout, cmd.Stderr = stdout, stderr
		return cmd.Run()
	})
}

// executeRawWithIO forwards output to the data and UI streams while capturing it.
// Captured output stays raw; only the displayed streams are masked. Each displayed stream goes
// through a streaming masker so a secret split across two reads is still masked.
func (w *OutputModeWriter) executeRawWithIO(runner func(stdout, stderr io.Writer) error) (string, string, error) {
	var stdout, stderr bytes.Buffer
	ioCtx := iolib.GetContext()

	// Data() returns stdout for pipeable output, UI() returns stderr for human messages.
	dataOut := iolib.NewStreamingMaskWriter(ioCtx.Data())
	uiOut := iolib.NewStreamingMaskWriter(ioCtx.UI())

	w.writeStepHeader()
	err := runner(io.MultiWriter(&stdout, dataOut), io.MultiWriter(&stderr, uiOut))

	// Release any held tail before the footer so output stays in order.
	flushDisplay(dataOut, uiOut)
	w.writeStepFooter(err)
	return stdout.String(), stderr.String(), err
}

// flushDisplay flushes streaming maskers. Display errors are not actionable at this point,
// matching how forwarded output errors are handled elsewhere in this file.
func flushDisplay(writers ...*iolib.StreamingMaskWriter) {
	for _, sw := range writers {
		_ = sw.Flush()
	}
}

// executeLog streams output with step boundaries.
// Complete lines are forwarded as they are produced; only a trailing partial line is held back until the command exits.
func (w *OutputModeWriter) executeLog(cmd *exec.Cmd) (string, string, error) {
	return w.executeLogWithIO(func(stdout, stderr io.Writer) error {
		cmd.Stdout, cmd.Stderr = stdout, stderr
		return cmd.Run()
	})
}

func (w *OutputModeWriter) executeLogWithIO(runner func(stdout, stderr io.Writer) error) (string, string, error) {
	// One mutex is shared by both streams so forwarded chunks never tear, even when
	// stdout and stderr are written concurrently from different goroutines.
	var forwardMu sync.Mutex

	// Forwarded batches go through one streaming masker per stream so a multiline secret that
	// spans several batches is masked as a whole, exactly like the non-streaming path.
	dataMask := iolib.NewStreamingMaskWriter(sinkWriter(func(s string) { _ = data.Write(s) }))
	uiMask := iolib.NewStreamingMaskWriter(sinkWriter(ui.Write))
	stdout := newLineForwarder(&forwardMu, func(s string) { _, _ = dataMask.Write([]byte(s)) })
	stderr := newLineForwarder(&forwardMu, func(s string) { _, _ = uiMask.Write([]byte(s)) })

	w.writeStepHeader()
	err := runner(stdout, stderr)

	// Flush trailing partial lines (no newline) before the footer.
	stdout.Flush()
	stderr.Flush()
	flushDisplay(dataMask, uiMask)
	w.writeStepFooter(err)

	return stdout.String(), stderr.String(), err
}

func (w *OutputModeWriter) writeStepHeader() {
	if !ShowLabels(w.show) {
		return
	}
	styles := theme.GetCurrentStyles()
	var stepLabel string
	if styles != nil {
		stepLabel = styles.Label.Render("[" + w.stepName + "]")
	} else {
		stepLabel = "[" + w.stepName + "]"
	}
	ui.Writeln(stepLabel)
}

// fallbackToLog writes captured output with boundaries.
func (w *OutputModeWriter) fallbackToLog(stdout, stderr string, runErr error) (string, string, error) {
	// Print captured output.
	if stdout != "" {
		_ = data.Write(stdout)
	}
	if stderr != "" {
		ui.Write(stderr)
	}

	w.writeStepFooter(runErr)

	return stdout, stderr, runErr
}

func (w *OutputModeWriter) writeStepFooter(runErr error) {
	if !ShowLabels(w.show) {
		return
	}
	footer := w.formatStepFooter(runErr)
	ui.Writeln(footer)
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

func (w *OutputModeWriter) executeNoneWithIO(runner func(stdout, stderr io.Writer) error) (string, string, error) {
	var stdout, stderr bytes.Buffer

	err := runner(&stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

// NewCommandOutputWriter honors step/workflow output settings while retaining
// the unadorned streaming default of legacy command execution.
func NewCommandOutputWriter(step *schema.WorkflowStep, workflow *schema.WorkflowDefinition) *OutputModeWriter {
	defer perf.Track(nil, "step.NewCommandOutputWriter")()
	mode := GetOutputMode(step, workflow)
	if step.Output == "" && (workflow == nil || workflow.Output == "") && !envPagerEnabled() {
		mode = OutputModeRaw
	}
	show := GetShowConfig(step, workflow)
	if show.Labels == nil {
		show.Labels = BoolPtr(false)
	}
	return NewOutputModeWriter(mode, step.Name, GetViewportConfig(step, workflow), show)
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

	if envPagerEnabled() {
		return OutputModeViewport
	}

	// Default to log mode.
	return OutputModeLog
}

func envPagerEnabled() bool {
	if os.Getenv("NO_PAGER") != "" { //nolint:forbidigo // NO_PAGER is a standard CLI env var.
		return false
	}

	pagerValue := os.Getenv("ATMOS_PAGER") //nolint:forbidigo // Used here before command-level Viper binding is guaranteed.
	if pagerValue == "" {
		return false
	}

	return (&schema.Terminal{Pager: pagerValue}).IsPagerEnabled()
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
	ui.Writeln(cmdDisplay)
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

// sinkWriter adapts a string sink to io.Writer.
type sinkWriter func(string)

// Write implements io.Writer.
func (f sinkWriter) Write(p []byte) (int, error) {
	defer perf.Track(nil, "step.sinkWriter.Write")()

	f(string(p))
	return len(p), nil
}

// lineForwarder captures everything written to it and forwards complete lines to a sink immediately.
// A trailing partial line is held back until Flush is called. The forward mutex may be shared between
// several forwarders so that writes to different sinks are serialized and never interleave mid-chunk.
type lineForwarder struct {
	forwardMu *sync.Mutex
	forward   func(string)
	captured  bytes.Buffer
	partial   []byte
}

// newLineForwarder creates a lineForwarder that calls forward (under forwardMu) for each batch of complete lines.
func newLineForwarder(forwardMu *sync.Mutex, forward func(string)) *lineForwarder {
	return &lineForwarder{forwardMu: forwardMu, forward: forward}
}

// Write implements io.Writer. It captures p and forwards any newly completed lines.
func (f *lineForwarder) Write(p []byte) (int, error) {
	defer perf.Track(nil, "step.lineForwarder.Write")()

	f.forwardMu.Lock()
	defer f.forwardMu.Unlock()

	f.captured.Write(p)
	f.partial = append(f.partial, p...)

	idx := bytes.LastIndexByte(f.partial, '\n')
	if idx < 0 {
		return len(p), nil
	}
	f.forward(string(f.partial[:idx+1]))
	f.partial = append(f.partial[:0], f.partial[idx+1:]...)
	return len(p), nil
}

// Flush forwards any buffered partial line.
func (f *lineForwarder) Flush() {
	defer perf.Track(nil, "step.lineForwarder.Flush")()

	f.forwardMu.Lock()
	defer f.forwardMu.Unlock()

	if len(f.partial) == 0 {
		return
	}
	f.forward(string(f.partial))
	f.partial = nil
}

// String returns everything written so far, byte-for-byte.
func (f *lineForwarder) String() string {
	defer perf.Track(nil, "step.lineForwarder.String")()

	f.forwardMu.Lock()
	defer f.forwardMu.Unlock()

	return f.captured.String()
}
