package toolchain

import (
	"fmt"
	"os"
	"os/exec"
)

// execCommand is a variable to allow mocking exec.Command
var execCommand = exec.Command

// ToolRunner defines the interface for running and resolving tools (for real and mock installers)
type ToolRunner interface {
	FindBinaryPath(owner, repo, version string) (string, error)
	GetResolver() ToolResolver
	CreateLatestFile(owner, repo, version string) error
	ReadLatestFile(owner, repo string) (string, error)
}

func RunToolWithInstaller(installer ToolRunner, tool, version string, remainingArgs []string) error {
	// Parse tool into owner/repo using installer's tool resolution
	owner, repo, err := installer.GetResolver().Resolve(tool)
	if err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}

	// Get the binary path for this version
	binaryPath, err := installer.FindBinaryPath(owner, repo, version)
	if err != nil {
		// Binary path not found, try to install it automatically
		fmt.Fprintf(os.Stdout, "🔧 Tool %s@%s is not installed. Installing automatically...\n", tool, version)

		// Use the same installation UI as the install command
		installErr := InstallSingleTool(owner, repo, version, false, true)
		if installErr != nil {
			return fmt.Errorf("failed to auto-install %s@%s: %w. Run 'toolchain install %s/%s@%s' manually",
				tool, version, installErr, owner, repo, version)
		}

		// Get the binary path again after installation
		binaryPath, err = installer.FindBinaryPath(owner, repo, version)
		if err != nil {
			return fmt.Errorf("failed to find binary path after installation: %w", err)
		}
	} else {
		// Check if binary exists, and if not, try to install it automatically
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stdout, "🔧 Tool %s@%s is not installed. Installing automatically...\n", tool, version)

			// Use the same installation UI as the install command
			installErr := InstallSingleTool(owner, repo, version, false, true)
			if installErr != nil {
				return fmt.Errorf("failed to auto-install %s@%s: %w. Run 'toolchain install %s/%s@%s' manually",
					tool, version, installErr, owner, repo, version)
			}

			// Get the binary path again after installation
			binaryPath, err = installer.FindBinaryPath(owner, repo, version)
			if err != nil {
				return fmt.Errorf("failed to find binary path after installation: %w", err)
			}
		}
	}

	// Execute the binary with the provided arguments
	cmdExec := execCommand(binaryPath, remainingArgs...)
	return cmdExec.Run()
}
