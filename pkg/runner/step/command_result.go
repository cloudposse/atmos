package step

import (
	"bytes"
	"io"
	"strings"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ExecuteCommandResult captures one successful named command attempt.
func ExecuteCommandResult(name string, run func(stdout, stderr io.Writer) error) (*StepResult, error) {
	defer perf.Track(nil, "step.ExecuteCommandResult")()

	if name == "" {
		return nil, run(io.Discard, io.Discard)
	}

	var stdout, stderr bytes.Buffer
	if err := run(&stdout, &stderr); err != nil {
		return nil, err
	}

	stdoutValue := iolib.MaskString(stdout.String())
	stderrValue := iolib.MaskString(stderr.String())
	result := NewStepResult(strings.TrimSpace(stdoutValue)).
		WithMetadata("stdout", stdoutValue).
		WithMetadata("stderr", stderrValue).
		WithMetadata(exitCodeMetadata, 0)
	return result, nil
}

// StoreCommandResult evaluates declared outputs and stores a captured result.
func StoreCommandResult(vars *Variables, name string, outputs map[string]string, result *StepResult) error {
	defer perf.Track(nil, "step.StoreCommandResult")()

	if name == "" || result == nil {
		return nil
	}
	return vars.SetWithOutputs(name, result, outputs)
}
