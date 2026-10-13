package step

import (
	"bytes"
	"io"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CommandResultOption configures command output capture.
type CommandResultOption func(*commandResultOptions)

type commandResultOptions struct {
	captureUnnamed bool
}

// WithUnnamedCommandOutput captures unnamed attempts for output-based retry conditions.
func WithUnnamedCommandOutput() CommandResultOption {
	defer perf.Track(nil, "step.WithUnnamedCommandOutput")()

	return func(options *commandResultOptions) { options.captureUnnamed = true }
}

// ExecuteCommandResult captures named command attempts and optionally failed unnamed attempts.
// Failed output remains available for retry conditions without being stored as a successful step.
func ExecuteCommandResult(name string, run func(stdout, stderr io.Writer) error, options ...CommandResultOption) (*StepResult, error) {
	defer perf.Track(nil, "step.ExecuteCommandResult")()

	config := commandResultOptions{}
	for _, option := range options {
		option(&config)
	}
	if name == "" && !config.captureUnnamed {
		return nil, run(io.Discard, io.Discard)
	}

	var stdout, stderr bytes.Buffer
	runErr := run(&stdout, &stderr)
	if name == "" && runErr == nil {
		return nil, nil
	}

	stdoutValue := iolib.MaskString(stdout.String())
	stderrValue := iolib.MaskString(stderr.String())
	result := NewStepResult(strings.TrimSpace(stdoutValue)).
		WithMetadata("stdout", stdoutValue).
		WithMetadata("stderr", stderrValue).
		WithMetadata(exitCodeMetadata, errUtils.GetExitCode(runErr))
	return result, runErr
}

// StoreCommandResult evaluates declared outputs and stores a captured result.
func StoreCommandResult(vars *Variables, name string, outputs map[string]string, result *StepResult) error {
	defer perf.Track(nil, "step.StoreCommandResult")()

	if name == "" || result == nil {
		return nil
	}
	return vars.SetWithOutputs(name, result, outputs)
}
