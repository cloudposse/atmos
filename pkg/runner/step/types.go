package step

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// StepCategory groups step types for documentation and validation.
type StepCategory string

const (
	// CategoryInteractive requires user input (TTY required).
	CategoryInteractive StepCategory = "interactive"
	// CategoryOutput displays formatted output.
	CategoryOutput StepCategory = "output"
	// CategoryUI displays status messages.
	CategoryUI StepCategory = "ui"
	// CategoryCommand executes commands.
	CategoryCommand StepCategory = "command"
)

// OutputMode controls how command output is displayed.
type OutputMode string

const (
	// OutputModeViewport displays output in an interactive TUI pager.
	OutputModeViewport OutputMode = "viewport"
	// OutputModeRaw passes output directly to stdout/stderr.
	OutputModeRaw OutputMode = "raw"
	// OutputModeLog groups output with step boundaries.
	OutputModeLog OutputMode = "log"
	// OutputModeNone suppresses all output.
	OutputModeNone OutputMode = "none"
)

// StepResult captures the output of a step execution.
type StepResult struct {
	// Value is the primary output value (for variable capture).
	Value string
	// Values holds multiple values for multiselect operations.
	Values []string
	// Metadata contains additional data from the step execution.
	Metadata map[string]any
	// Skipped indicates if the step was skipped.
	Skipped bool
	// Error captures any error message from the step.
	Error string
}

// NewStepResult creates a new StepResult with the given value.
func NewStepResult(value string) *StepResult {
	defer perf.Track(nil, "step.NewStepResult")()

	return &StepResult{
		Value:    value,
		Metadata: make(map[string]any),
	}
}

// WithValues adds multiple values to the result.
func (r *StepResult) WithValues(values []string) *StepResult {
	defer perf.Track(nil, "step.StepResult.WithValues")()

	r.Values = values
	return r
}

// WithMetadata adds metadata to the result.
func (r *StepResult) WithMetadata(key string, value any) *StepResult {
	defer perf.Track(nil, "step.StepResult.WithMetadata")()

	if r.Metadata == nil {
		r.Metadata = make(map[string]any)
	}
	r.Metadata[key] = value
	return r
}

// WithSkipped marks the result as skipped.
func (r *StepResult) WithSkipped() *StepResult {
	defer perf.Track(nil, "step.StepResult.WithSkipped")()

	r.Skipped = true
	return r
}

// WithError adds an error message to the result.
func (r *StepResult) WithError(err string) *StepResult {
	defer perf.Track(nil, "step.StepResult.WithError")()

	r.Error = err
	return r
}
