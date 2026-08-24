package toolchain

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// execFunc is a function variable for executing external commands.
// This allows for testing by replacing with a mock implementation.
// Returns exec.ExitError which preserves the exit code for the caller to handle.
var execFunc = func(binaryPath string, args []string, env []string) error {
	// #nosec G204 -- binaryPath is validated through tool resolution and binary lookup
	cmd := exec.Command(binaryPath, args[1:]...) // args[0] is the binary itself
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run returns exec.ExitError on non-zero exit, which preserves the exit code.
	// The caller is responsible for extracting the exit code using errors.GetExitCode().
	return cmd.Run()
}

// ToolRunner defines the interface for running and resolving tools (for real and mock installers).
type ToolRunner interface {
	FindBinaryPath(owner, repo, version string, binaryName ...string) (string, error)
	GetResolver() ToolResolver
	CreateLatestFile(owner, repo, version string) error
	ReadLatestFile(owner, repo string) (string, error)
}

// RunExecCommand contains business logic for executing tools.
// It does not depend on cobra.Command, only raw args.
func RunExecCommand(installer ToolRunner, args []string) error {
	return RunExecCommandWithOptions(installer, args, false)
}

// RunExecCommandWithOptions contains business logic for executing tools, with an optional
// dry-run mode. It does not depend on cobra.Command, only raw args.
//
// When dryRun is true, the tool is still resolved and auto-installed if necessary (so the
// dry-run output reflects a real, executable binary path), but execFunc is never invoked —
// the resolved binary path and arguments are printed instead.
func RunExecCommandWithOptions(installer ToolRunner, args []string, dryRun bool) error {
	defer perf.Track(nil, "toolchain.Exec")()

	if len(args) == 0 {
		return fmt.Errorf("%w: no arguments provided. Expected format: tool@version", ErrInvalidToolSpec)
	}

	toolSpec := args[0]
	remainingArgs := args[1:]
	tool, _, err := ParseToolVersionArg(toolSpec)
	if err != nil {
		return err
	}
	if tool == "" {
		return fmt.Errorf("%w: missing tool name", ErrInvalidToolSpec)
	}

	_, _, err = installer.GetResolver().Resolve(tool)
	if err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}

	binaryPath, err := ensureToolInstalled(installer, toolSpec)
	if err != nil {
		return err
	}

	execArgs := append([]string{binaryPath}, remainingArgs...)

	if dryRun {
		ui.Writeln(fmt.Sprintf("Would execute: %s", strings.Join(execArgs, " ")))
		return nil
	}

	// Replace the current process with the tool binary.
	return execFunc(binaryPath, execArgs, os.Environ())
}

// ensureToolInstalled checks if the binary exists, otherwise installs it.
// The installer parameter is injected for better testability.
func ensureToolInstalled(_ ToolRunner, tool string) (string, error) {
	binaryPath, err := findBinaryPath(tool)
	if err == nil && binaryPath != "" {
		if _, statErr := os.Stat(binaryPath); !os.IsNotExist(statErr) {
			return binaryPath, nil
		}
	}

	ui.Toastf("🔧", "Tool %s is not installed. Installing automatically...", tool)
	// Show hint and progress bar for manual exec installs (user requested specific tool execution).
	if installErr := RunInstall(tool, false, true, true, true); installErr != nil {
		return "", fmt.Errorf("failed to auto-install %s: %w",
			tool, installErr)
	}

	return findBinaryPath(tool)
}
