package toolchain

import (
	"fmt"
	"os"
	"syscall"
)

var execFunc = syscall.Exec

// RunExecCommand contains business logic for executing tools.
// It does not depend on cobra.Command, only raw args.
func RunExecCommand(installer ToolRunner, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no arguments provided. Expected format: tool@version")
	}

	toolSpec := args[0]
	tool, version, err := ParseToolVersionArg(toolSpec)
	if err != nil {
		return err
	}
	if tool == "" {
		return fmt.Errorf("invalid tool specification: missing tool name")
	}
	remainingArgs := args[1:]

	owner, repo, err := installer.GetResolver().Resolve(tool)
	if err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}

	binaryPath, err := ensureToolInstalled(installer, owner, repo, tool, version)
	if err != nil {
		return err
	}

	// Replace the current process with the tool binary
	return execFunc(binaryPath, append([]string{binaryPath}, remainingArgs...), os.Environ())
}

// ensureToolInstalled checks if the binary exists, otherwise installs it.
func ensureToolInstalled(installer ToolRunner, owner, repo, tool, version string) (string, error) {
	binaryPath, err := installer.FindBinaryPath(owner, repo, version)
	if err == nil && binaryPath != "" {
		if _, statErr := os.Stat(binaryPath); !os.IsNotExist(statErr) {
			return binaryPath, nil
		}
	}

	fmt.Printf("🔧 Tool %s@%s is not installed. Installing automatically...\n", tool, version)
	if installErr := InstallSingleTool(owner, repo, version, false, true); installErr != nil {
		return "", fmt.Errorf("failed to auto-install %s@%s: %w. Run 'toolchain install %s/%s@%s' manually",
			tool, version, installErr, owner, repo, version)
	}

	return installer.FindBinaryPath(owner, repo, version)
}
