package shell

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	envpkg "github.com/cloudposse/atmos/pkg/env"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteShellSpec configures shell execution.
type ExecuteShellSpec struct {
	// Context, when set, is threaded into the interpreter so cancellation
	// (e.g. Ctrl-C on the top-level Cobra invocation) stops an in-flight
	// shell step instead of letting it run to completion. Defaults to
	// context.Background() when nil.
	Context context.Context
	Command string
	Name    string
	Dir     string
	EnvVars []string
	DryRun  bool
	Stdout  io.Writer
	Stderr  io.Writer
}

// ExecuteShellMasked runs spec with stdout and stderr masked by split-safe streaming maskers, so a
// secret split across two pipe reads is still masked. The maskers are flushed once the script has
// exited. The optional captures receive the output as written by the script; the caller owns them.
// The Stdout and Stderr fields of spec are overwritten.
func ExecuteShellMasked(spec *ExecuteShellSpec, stdout, stderr, stdoutCapture, stderrCapture io.Writer) error {
	defer perf.Track(nil, "shell.ExecuteShellMasked")()

	masked := ioLayer.NewMaskedStreams(os.Stdin, stdout, stderr)
	spec.Stdout = withCapture(masked.Stdout, stdoutCapture)
	spec.Stderr = withCapture(masked.Stderr, stderrCapture)
	return masked.Finish(ExecuteShellWithWriters(spec))
}

// withCapture tees w into capture when capture is set.
func withCapture(w, capture io.Writer) io.Writer {
	if capture == nil {
		return w
	}
	return io.MultiWriter(w, capture)
}

// ExecuteShellWithWriters runs a shell script with explicit stdout/stderr writers.
func ExecuteShellWithWriters(spec *ExecuteShellSpec) error {
	defer perf.Track(nil, "shell.ExecuteShellWithWriters")()

	newShellLevel, err := u.GetNextShellLevel()
	if err != nil {
		return err
	}

	// Always start with the current process environment to ensure PATH and other
	// system variables are available. Custom env vars passed in will override
	// any existing values with the same key via UpdateEnvVar semantics.
	// This matches the behavior before commit 9fd7d156a where the environment
	// was merged rather than replaced.
	mergedEnv := os.Environ()
	for _, envVar := range spec.EnvVars {
		mergedEnv = envpkg.UpdateEnvVar(mergedEnv, parseEnvVarKey(envVar), parseEnvVarValue(envVar))
	}

	mergedEnv = append(mergedEnv, fmt.Sprintf("ATMOS_SHLVL=%d", newShellLevel))

	log.Debug("Executing", "command", spec.Command)

	if spec.DryRun {
		return nil
	}

	return u.ShellRunnerWithWriters(&u.ShellRunnerSpec{
		Context: spec.Context,
		Command: spec.Command,
		Name:    spec.Name,
		Dir:     spec.Dir,
		Env:     mergedEnv,
		Stdout:  spec.Stdout,
		Stderr:  spec.Stderr,
	})
}

// parseEnvVarKey extracts the key from an environment variable string (KEY=value).
func parseEnvVarKey(envVar string) string {
	if idx := strings.IndexByte(envVar, '='); idx >= 0 {
		return envVar[:idx]
	}
	return envVar
}

// parseEnvVarValue extracts the value from an environment variable string (KEY=value).
func parseEnvVarValue(envVar string) string {
	if idx := strings.IndexByte(envVar, '='); idx >= 0 {
		return envVar[idx+1:]
	}
	return ""
}
