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
)

const (
	directoryPermissions = 0o755
	defaultTestTimeout   = "40m"
	cgoDisabled          = "CGO_ENABLED=0"
	// Links Go's native FIPS 140-3 validated crypto module so acceptance-test
	// binaries default to FIPS-enforcing (GODEBUG=fips140=on) at runtime,
	// matching release builds. See docs/prd/fips-140-mode.md.
	fips140Latest = "GOFIPS140=latest"
)

var (
	errInvalidConfiguration = errors.New("invalid acceptance configuration")
	errCoverageData         = errors.New("invalid coverage data")
	errShardPlan            = errors.New("invalid acceptance shard plan")
	errRequiredArtifact     = errors.New("required acceptance artifact")
)

type commandRunner struct {
	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
}

func newCommandRunner() commandRunner {
	return commandRunner{stdout: os.Stdout, stderr: os.Stderr, stdin: os.Stdin}
}

func environment(name string) string {
	value, _ := os.LookupEnv(name)
	return value
}

func goCommandEnvironment(values ...string) []string {
	return append([]string{cgoDisabled, fips140Latest}, values...)
}

func writeStatus(format string, args ...any) error {
	writer := io.Writer(os.Stdout)
	if _, err := fmt.Fprintf(writer, format, args...); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	return nil
}

func (r commandRunner) run(ctx context.Context, dir string, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G702 -- CI executes only repository-selected tools and test binaries.
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = r.stdin
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", commandString(name, args), err)
	}
	return nil
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
