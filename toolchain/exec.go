package toolchain

import (
	"fmt"
	"os"
	"syscall"
)

var execFunc = syscall.Exec

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

	binaryPath, err := ensureToolInstalled(toolSpec)
	if err != nil {
		return err
	}

	// Replace the current process with the tool binary
	return execFunc(binaryPath, append([]string{binaryPath}, remainingArgs...), os.Environ())
}

// ensureToolInstalled checks if the binary exists, otherwise installs it.
func ensureToolInstalled(tool string) (string, error) {
	binaryPath, err := findBinaryPath(tool)
	if err == nil && binaryPath != "" {
		if _, statErr := os.Stat(binaryPath); !os.IsNotExist(statErr) {
			return binaryPath, nil
		}
	}

	fmt.Printf("🔧 Tool %s is not installed. Installing automatically...\n", tool)
	if installErr := RunInstall(tool, false, true); installErr != nil {
		return "", fmt.Errorf("failed to auto-install %s: %w",
			tool, installErr)
	}

	return findBinaryPath(tool)
}
