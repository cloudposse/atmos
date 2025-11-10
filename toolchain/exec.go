package toolchain

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// execFunc is a function variable for executing external commands.
// This allows for testing by replacing with a mock implementation.
var execFunc = func(binaryPath string, args []string, env []string) error {
	cmd := exec.Command(binaryPath, args[1:]...) // args[0] is the binary itself
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	// Exit with the same code as the child process.
	os.Exit(cmd.ProcessState.ExitCode())
	return nil
}

// ToolRunner defines the interface for running and resolving tools (for real and mock installers).
type ToolRunner interface {
	FindBinaryPath(owner, repo, version string) (string, error)
	GetResolver() ToolResolver
	CreateLatestFile(owner, repo, version string) error
	ReadLatestFile(owner, repo string) (string, error)
}

// RunExecCommand contains business logic for executing tools.
// It does not depend on cobra.Command, only raw args.
func RunExecCommand(installer ToolRunner, args []string) error {
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

	// Replace the current process with the tool binary
	return execFunc(binaryPath, append([]string{binaryPath}, remainingArgs...), os.Environ())
}

// ensureToolInstalled checks if the binary exists, otherwise installs it.
// The installer parameter is injected for better testability.
func ensureToolInstalled(installer ToolRunner, tool string) (string, error) {
	binaryPath, err := findBinaryPath(tool)
	if err == nil && binaryPath != "" {
		if _, statErr := os.Stat(binaryPath); !os.IsNotExist(statErr) {
			return binaryPath, nil
		}
	}

	_ = ui.Toastf("🔧", "Tool %s is not installed. Installing automatically...", tool)
	if installErr := RunInstall(tool, false, true); installErr != nil {
		return "", fmt.Errorf("failed to auto-install %s: %w",
			tool, installErr)
	}

	return findBinaryPath(tool)
}
