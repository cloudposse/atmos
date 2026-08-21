// Package acceptance implements repository-internal acceptance-test orchestration.
package acceptance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	directoryPermissions = 0o755
	defaultTestTimeout   = "40m"
	cgoDisabled          = "CGO_ENABLED=0"

	// Retry bounds for the known-benign Windows race handled by isTransientWindowsUnlinkError.
	transientUnlinkRetries = 2
	transientUnlinkDelay   = 2 * time.Second
)

var (
	errInvalidConfiguration = errors.New("invalid acceptance configuration")
	errCoverageData         = errors.New("invalid coverage data")
	errShardPlan            = errors.New("invalid acceptance shard plan")
	errRequiredArtifact     = errors.New("required acceptance artifact")
)

type commandRunner struct {
	stdout     io.Writer
	stderr     io.Writer
	stdin      io.Reader
	retryDelay time.Duration
}

func newCommandRunner() commandRunner {
	return commandRunner{stdout: os.Stdout, stderr: os.Stderr, stdin: os.Stdin, retryDelay: transientUnlinkDelay}
}

func environment(name string) string {
	value, _ := os.LookupEnv(name)
	return value
}

func goCommandEnvironment(values ...string) []string {
	return append([]string{cgoDisabled}, values...)
}

func writeStatus(format string, args ...any) error {
	writer := io.Writer(os.Stdout)
	if _, err := fmt.Fprintf(writer, format, args...); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	return nil
}

// run executes name/args, retrying on the known-benign Windows race where `go test`
// (or a precompiled *.test.exe it invoked) fails to delete its own temp binary after
// every test case has already reported ok/FAIL -- see isTransientWindowsUnlinkError.
// Retrying is safe here: by the time that error appears, the process under test has
// already exited and its actual pass/fail result was already written to stdout, so a
// retry re-runs already-cached work rather than masking a real failure.
func (r commandRunner) run(ctx context.Context, dir string, env []string, name string, args ...string) error {
	var lastErr error
	for attempt := 0; attempt <= transientUnlinkRetries; attempt++ {
		var stderrCapture bytes.Buffer
		cmd := exec.CommandContext(ctx, name, args...) // #nosec G702 -- CI executes only repository-selected tools and test binaries.
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		cmd.Stdin = r.stdin
		cmd.Stdout = r.stdout
		cmd.Stderr = io.MultiWriter(r.stderr, &stderrCapture)
		err := cmd.Run()
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("run %s: %w", commandString(name, args), err)
		if attempt == transientUnlinkRetries || !isTransientWindowsUnlinkError(stderrCapture.String()) {
			return lastErr
		}
		_, _ = fmt.Fprintf(r.stderr, "::warning::retrying %s after a transient Windows go-test cleanup race (attempt %d/%d): %v\n",
			commandString(name, args), attempt+1, transientUnlinkRetries, err)
		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(r.retryDelay):
		}
	}
	return lastErr
}

// isTransientWindowsUnlinkError reports whether output is the Go toolchain's own
// "go: unlinkat ...: The process cannot access the file because it is being used by
// another process" diagnostic -- a documented Windows race (another process, commonly
// Windows Defender's real-time scanner, briefly holds the temp test binary open right
// as `go test` tries to delete it) that fires only after every test case in the run has
// already reported its real result. It is never emitted for an actual test failure.
func isTransientWindowsUnlinkError(output string) bool {
	return strings.Contains(output, "unlinkat") &&
		strings.Contains(output, "cannot access the file because it is being used by another process")
}

func (r commandRunner) output(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run %s: %w: %s", commandString(name, args), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func commandString(name string, args []string) string {
	return strings.Join(append([]string{name}, args...), " ")
}

func FindRepoRoot(start string) (string, error) {
	root, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr == nil {
			return root, nil
		}
		parent := filepath.Dir(root)
		if parent == root {
			return "", fmt.Errorf("%w: find repository root from %s", errInvalidConfiguration, start)
		}
		root = parent
	}
}
