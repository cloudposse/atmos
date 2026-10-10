package hooks

import (
	"context"
	"io"
	"os"
	"slices"

	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RunOption configures a Git hook invocation.
type RunOption func(*runOptions)

type runOptions struct {
	ctx            context.Context
	config         *schema.AtmosConfiguration
	stdout, stderr io.Writer
}

// WithContext supplies cancellation and deadlines for step execution.
func WithContext(ctx context.Context) RunOption {
	defer perf.Track(nil, "hooks.WithContext")()

	return func(o *runOptions) { o.ctx = ctx }
}

// WithAtmosConfig supplies project configuration to step handlers.
func WithAtmosConfig(config *schema.AtmosConfiguration) RunOption {
	defer perf.Track(nil, "hooks.WithAtmosConfig")()

	return func(o *runOptions) { o.config = config }
}

// WithOutputWriters routes step output to the host streams.
func WithOutputWriters(stdout, stderr io.Writer) RunOption {
	defer perf.Track(nil, "hooks.WithOutputWriters")()

	return func(o *runOptions) { o.stdout, o.stderr = stdout, stderr }
}

func runSteps(entry schema.GitHookEntry, args []string, opts []RunOption) error {
	defer perf.Track(nil, "hooks.runSteps")()
	options := runOptions{ctx: context.Background(), stdout: os.Stdout, stderr: os.Stderr}
	for _, opt := range opts {
		opt(&options)
	}
	vars := step.NewVariables()
	vars.SetAtmosConfig(options.config)
	vars.ScriptArgs = slices.Clone(args)
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	return step.NewAutomationLibrary(vars, nil).RunSteps(options.ctx, entry.Steps, &automation.StepCall{
		WorkingDirectory: dir, Stdout: options.stdout, Stderr: options.stderr,
	})
}
