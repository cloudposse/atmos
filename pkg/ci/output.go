package ci

import (
	"fmt"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// NoopOutputWriter is an OutputWriter that does nothing.
// Used when not running in CI or when CI outputs are disabled.
type NoopOutputWriter struct{}

// WriteOutput implements OutputWriter.
func (w *NoopOutputWriter) WriteOutput(_, _ string) error {
	defer perf.Track(nil, "ci.NoopOutputWriter.WriteOutput")()

	return nil
}

// WriteSummary implements OutputWriter.
func (w *NoopOutputWriter) WriteSummary(_ string) error {
	defer perf.Track(nil, "ci.NoopOutputWriter.WriteSummary")()

	return nil
}

// FileOutputWriter writes outputs to a file (like $GITHUB_OUTPUT).
type FileOutputWriter struct {
	outputPath  string
	summaryPath string
}

// NewFileOutputWriter creates a new FileOutputWriter.
func NewFileOutputWriter(outputPath, summaryPath string) *FileOutputWriter {
	defer perf.Track(nil, "ci.NewFileOutputWriter")()

	return &FileOutputWriter{
		outputPath:  outputPath,
		summaryPath: summaryPath,
	}
}

// WriteOutput writes a key-value pair to the output file.
// Format: key=value (single line) or key<<EOF\nvalue\nEOF (multiline).
func (w *FileOutputWriter) WriteOutput(key, value string) error {
	defer perf.Track(nil, "ci.FileOutputWriter.WriteOutput")()

	if w.outputPath == "" {
		return nil
	}

	f, err := os.OpenFile(w.outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open output file: %w", err)
	}
	defer f.Close()

	// Use heredoc format for multiline values.
	if strings.Contains(value, "\n") {
		delimiter := "EOF"
		// Ensure delimiter doesn't appear in value.
		for strings.Contains(value, delimiter) {
			delimiter += "_"
		}
		_, err = fmt.Fprintf(f, "%s<<%s\n%s\n%s\n", key, delimiter, value, delimiter)
	} else {
		_, err = fmt.Fprintf(f, "%s=%s\n", key, value)
	}

	return err
}

// WriteSummary appends content to the job summary file.
func (w *FileOutputWriter) WriteSummary(content string) error {
	defer perf.Track(nil, "ci.FileOutputWriter.WriteSummary")()

	if w.summaryPath == "" {
		return nil
	}

	f, err := os.OpenFile(w.summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open summary file: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(content)
	return err
}

// OutputHelpers provides helper methods for common CI output patterns.
type OutputHelpers struct {
	Writer OutputWriter
}

// NewOutputHelpers creates a new OutputHelpers.
func NewOutputHelpers(writer OutputWriter) *OutputHelpers {
	defer perf.Track(nil, "ci.NewOutputHelpers")()

	return &OutputHelpers{Writer: writer}
}

// WritePlanOutputs writes standard plan output variables.
func (h *OutputHelpers) WritePlanOutputs(opts PlanOutputOptions) error {
	defer perf.Track(nil, "ci.OutputHelpers.WritePlanOutputs")()

	if err := h.Writer.WriteOutput("has_changes", fmt.Sprintf("%t", opts.HasChanges)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("has_additions", fmt.Sprintf("%t", opts.HasAdditions)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("has_additions_count", fmt.Sprintf("%d", opts.AdditionsCount)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("has_changes_count", fmt.Sprintf("%d", opts.ChangesCount)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("has_destructions", fmt.Sprintf("%t", opts.HasDestructions)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("has_destructions_count", fmt.Sprintf("%d", opts.DestructionsCount)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("plan_exit_code", fmt.Sprintf("%d", opts.ExitCode)); err != nil {
		return err
	}
	if opts.ArtifactKey != "" {
		if err := h.Writer.WriteOutput("artifact_key", opts.ArtifactKey); err != nil {
			return err
		}
	}
	return nil
}

// WriteApplyOutputs writes standard apply output variables.
func (h *OutputHelpers) WriteApplyOutputs(opts ApplyOutputOptions) error {
	defer perf.Track(nil, "ci.OutputHelpers.WriteApplyOutputs")()

	if err := h.Writer.WriteOutput("apply_exit_code", fmt.Sprintf("%d", opts.ExitCode)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("success", fmt.Sprintf("%t", opts.Success)); err != nil {
		return err
	}
	// Write terraform outputs with output_ prefix.
	for key, value := range opts.Outputs {
		if err := h.Writer.WriteOutput("output_"+key, value); err != nil {
			return err
		}
	}
	return nil
}

// PlanOutputOptions contains options for writing plan outputs.
type PlanOutputOptions struct {
	HasChanges        bool
	HasAdditions      bool
	AdditionsCount    int
	ChangesCount      int
	HasDestructions   bool
	DestructionsCount int
	ExitCode          int
	ArtifactKey       string
}

// ApplyOutputOptions contains options for writing apply outputs.
type ApplyOutputOptions struct {
	ExitCode int
	Success  bool
	Outputs  map[string]string
}
