package step

import (
	"bytes"
	"io"
	"strings"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ExecuteAndStoreCommandResult captures and stores one successful command attempt.
func ExecuteAndStoreCommandResult(vars *Variables, name string, outputs map[string]string, run func(stdout, stderr io.Writer) error) error {
	defer perf.Track(nil, "step.ExecuteAndStoreCommandResult")()

	if name == "" {
		return run(io.Discard, io.Discard)
	}

	var stdout, stderr bytes.Buffer
	if err := run(&stdout, &stderr); err != nil {
		return err
	}

	stdoutValue := iolib.MaskString(stdout.String())
	stderrValue := iolib.MaskString(stderr.String())
	result := NewStepResult(strings.TrimSpace(stdoutValue)).
		WithMetadata("stdout", stdoutValue).
		WithMetadata("stderr", stderrValue).
		WithMetadata(exitCodeMetadata, 0)

	return vars.SetWithOutputs(name, result, outputs)
}
