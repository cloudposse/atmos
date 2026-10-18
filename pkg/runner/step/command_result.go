package step

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

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
	if name == "" {
		if !config.captureUnnamed {
			return nil, run(io.Discard, io.Discard)
		}
		return executeSpooledCommand(run, os.CreateTemp)
	}

	var stdout, stderr bytes.Buffer
	runErr := run(&stdout, &stderr)
	return capturedCommandResult(stdout.String(), stderr.String(), runErr), runErr
}

// capturedCommandResult masks both streams before exposing them to expressions or retry conditions.
func capturedCommandResult(stdout, stderr string, runErr error) *StepResult {
	defer perf.Track(nil, "step.capturedCommandResult")()

	stdoutValue := iolib.MaskString(stdout)
	stderrValue := iolib.MaskString(stderr)
	return NewStepResult(strings.TrimSpace(stdoutValue)).
		WithMetadata("stdout", stdoutValue).
		WithMetadata("stderr", stderrValue).
		WithMetadata(exitCodeMetadata, errUtils.GetExitCode(runErr))
}

// commandOutputSpool remembers write failures even when the command ignores writer errors.
type commandOutputSpool struct {
	file *os.File
	mu   sync.Mutex
	err  error
}

func (s *commandOutputSpool) Write(p []byte) (int, error) {
	defer perf.Track(nil, "step.commandOutputSpool.Write")()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return 0, s.err
	}
	n, err := s.file.Write(p)
	s.err = err
	return n, err
}

// executeSpooledCommand keeps successful unnamed output off the heap. Failed output is read in full
// because retry conditions match complete strings. CreateTemp produces owner-only files.
func executeSpooledCommand(run func(stdout, stderr io.Writer) error, createTemp func(string, string) (*os.File, error)) (_ *StepResult, resultErr error) {
	defer perf.Track(nil, "step.executeSpooledCommand")()

	stdout, err := createTemp("", "atmos-retry-stdout-*")
	if err != nil {
		return nil, fmt.Errorf("create command stdout capture: %w", err)
	}
	defer cleanupCommandSpool(stdout, &resultErr)
	stderr, err := createTemp("", "atmos-retry-stderr-*")
	if err != nil {
		return nil, fmt.Errorf("create command stderr capture: %w", err)
	}
	defer cleanupCommandSpool(stderr, &resultErr)

	out, errOut := &commandOutputSpool{file: stdout}, &commandOutputSpool{file: stderr}
	runErr := run(out, errOut)
	if writeErr := errors.Join(out.err, errOut.err); writeErr != nil {
		return nil, errors.Join(runErr, fmt.Errorf("%w: %w", errUtils.ErrWriteOutput, writeErr))
	}
	if runErr == nil {
		return nil, nil
	}
	stdoutBytes, stdoutErr := readCommandSpool(stdout)
	stderrBytes, stderrErr := readCommandSpool(stderr)
	if readErr := errors.Join(stdoutErr, stderrErr); readErr != nil {
		return nil, errors.Join(runErr, fmt.Errorf("read failed command output: %w", readErr))
	}
	return capturedCommandResult(string(stdoutBytes), string(stderrBytes), runErr), runErr
}

// readCommandSpool rewinds an attempt's stream only after the command fails.
func readCommandSpool(file *os.File) ([]byte, error) {
	defer perf.Track(nil, "step.readCommandSpool")()

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(file)
}

// cleanupCommandSpool closes before removal for Windows and preserves the original command error.
func cleanupCommandSpool(file *os.File, resultErr *error) {
	defer perf.Track(nil, "step.cleanupCommandSpool")()

	if err := errors.Join(file.Close(), os.Remove(file.Name())); err != nil {
		*resultErr = errors.Join(*resultErr, fmt.Errorf("clean up command output: %w", err))
	}
}

// StoreCommandResult evaluates declared outputs and stores a captured result.
func StoreCommandResult(vars *Variables, name string, outputs map[string]string, result *StepResult) error {
	defer perf.Track(nil, "step.StoreCommandResult")()

	if name == "" || result == nil {
		return nil
	}
	return vars.SetWithOutputs(name, result, outputs)
}
