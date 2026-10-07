package provider

import (
	"fmt"
	"os"
	"strconv"

	errUtils "github.com/cloudposse/atmos/errors"
	ghactions "github.com/cloudposse/atmos/pkg/github/actions"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// outputFilePermissions is the file permission mode for CI output files.
	outputFilePermissions = 0o644
)

// NoopOutputWriter is an OutputWriter that does nothing.
// Used when not running in CI or when CI outputs are disabled.
type NoopOutputWriter struct{}

// WriteOutput implements OutputWriter.
func (w *NoopOutputWriter) WriteOutput(_, _ string) error {
	defer perf.Track(nil, "provider.NoopOutputWriter.WriteOutput")()

	return nil
}

// WriteSummary implements OutputWriter.
func (w *NoopOutputWriter) WriteSummary(_ string) error {
	defer perf.Track(nil, "provider.NoopOutputWriter.WriteSummary")()

	return nil
}

// FileOutputWriter writes outputs to a file (like $GITHUB_OUTPUT).
type FileOutputWriter struct {
	OutputPath  string
	SummaryPath string
}

// NewFileOutputWriter creates a new FileOutputWriter.
func NewFileOutputWriter(outputPath, summaryPath string) *FileOutputWriter {
	defer perf.Track(nil, "provider.NewFileOutputWriter")()

	return &FileOutputWriter{
		OutputPath:  outputPath,
		SummaryPath: summaryPath,
	}
}

// WriteOutput writes a key-value pair to the output file.
// Format: key=value (single line) or key<<ATMOS_EOF_key\nvalue\nATMOS_EOF_key (multiline).
func (w *FileOutputWriter) WriteOutput(key, value string) error {
	defer perf.Track(nil, "provider.FileOutputWriter.WriteOutput")()

	if w.OutputPath == "" {
		return nil
	}

	return AppendFile(w.OutputPath, ghactions.FormatValue(key, value), errUtils.ErrCIOutputWriteFailed)
}

// WriteSummary appends content to the job summary file.
func (w *FileOutputWriter) WriteSummary(content string) error {
	defer perf.Track(nil, "provider.FileOutputWriter.WriteSummary")()

	if w.SummaryPath == "" {
		return nil
	}

	return AppendFile(w.SummaryPath, MaskPublishedContent(content), errUtils.ErrCISummaryWriteFailed)
}

// AppendFile appends content to the file at path, creating it when needed. Open and
// write failures are wrapped in sentinel so callers get the right CI error kind.
func AppendFile(path, content string, sentinel error) error {
	defer perf.Track(nil, "provider.AppendFile")()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, outputFilePermissions)
	if err != nil {
		return fmt.Errorf("%w: failed to open %s: %w", sentinel, path, err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("%w: failed to write %s: %w", sentinel, path, err)
	}
	return nil
}

// OutputHelpers provides helper methods for common CI output patterns.
type OutputHelpers struct {
	Writer OutputWriter
}

// NewOutputHelpers creates a new OutputHelpers.
func NewOutputHelpers(writer OutputWriter) *OutputHelpers {
	defer perf.Track(nil, "provider.NewOutputHelpers")()

	return &OutputHelpers{Writer: writer}
}

// WritePlanOutputs writes standard plan output variables.
func (h *OutputHelpers) WritePlanOutputs(opts PlanOutputOptions) error {
	defer perf.Track(nil, "provider.OutputHelpers.WritePlanOutputs")()

	if err := h.Writer.WriteOutput("has_changes", strconv.FormatBool(opts.HasChanges)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("has_additions", strconv.FormatBool(opts.HasAdditions)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("has_additions_count", strconv.Itoa(opts.AdditionsCount)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("has_changes_count", strconv.Itoa(opts.ChangesCount)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("has_destructions", strconv.FormatBool(opts.HasDestructions)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("has_destructions_count", strconv.Itoa(opts.DestructionsCount)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("plan_exit_code", strconv.Itoa(opts.ExitCode)); err != nil {
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
	defer perf.Track(nil, "provider.OutputHelpers.WriteApplyOutputs")()

	if err := h.Writer.WriteOutput("apply_exit_code", strconv.Itoa(opts.ExitCode)); err != nil {
		return err
	}
	if err := h.Writer.WriteOutput("success", strconv.FormatBool(opts.Success)); err != nil {
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
