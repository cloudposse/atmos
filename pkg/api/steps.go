// Package api defines the Go API shared by automation language adapters.
package api

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

// GroupScheduler is implemented by a StepLibrary that positions parallel groups. A scheduler that
// implements fan-out calls ScheduleGroup once per group, before it starts the first branch, and forks
// the branches from the returned library. Each group gets a position of its own, so the work of two
// consecutive groups is never interleaved.
type GroupScheduler interface {
	StepLibrary

	// ScheduleGroup reserves the position of one parallel group that starts now.
	ScheduleGroup() ScheduledForker
}

// ScheduledForker is implemented by a StepLibrary that can fork a branch of a parallel group at a
// known scheduled position. A scheduler that implements fan-out calls ForkAt instead of Fork, with
// the zero-based index of the branch in the group, so work recorded by the branches (such as artifact
// reports) is ordered by where each branch was scheduled rather than by when it finished. The library
// of a group comes from [GroupScheduler.ScheduleGroup].
type ScheduledForker interface {
	StepLibrary

	// ForkAt isolates a branch like Fork and records the branch's scheduled index.
	ForkAt(index int) StepLibrary
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
