// Package automation defines the Go API shared by automation language adapters.
package automation

import (
	"context"
	"io"
)

// StepLibrary dispatches registered Atmos steps. Each invocation and parallel
// branch owns a fork, so step outputs and environment changes remain local.
type StepLibrary interface {
	Names() []string
	Fork() StepLibrary
	Validate(*StepCall) error
	Run(context.Context, *StepCall) (*StepResult, error)
}

// StepCall contains native Go step configuration.
// Writers and process settings belong to the calling script thread.
type StepCall struct {
	Type             string
	Configuration    map[string]any
	WorkingDirectory string
	ProcessEnv       []string
	Stdout, Stderr   io.Writer
	Parallel         bool
}

// StepResult exposes the same value, selections, metadata, and named outputs
// returned by the step registry. Engines convert these into immutable values.
type StepResult struct {
	Value    string            `json:"value"`
	Values   []string          `json:"values"`
	Metadata map[string]any    `json:"metadata"`
	Outputs  map[string]string `json:"outputs"`
	Skipped  bool              `json:"skipped"`
	Error    string            `json:"error"`
}
