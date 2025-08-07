package toolchain

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run [tool[@version]] [flags...]",
	Short: "Run a specific version of a tool",
	Long: `Run a specific version of a tool with arguments.

If no version is specified, the latest version will be used.

Examples:
  toolchain run terraform --version          # Uses latest version
  toolchain run terraform@1.9.8 --version   # Uses specific version
  toolchain run opentofu@1.10.1 init
  toolchain run terraform@1.5.7 plan -var-file=prod.tfvars`,
	Args:               cobra.MinimumNArgs(1),
	RunE:               runTool,
	DisableFlagParsing: true,
}

// ToolRunner defines the interface for running and resolving tools (for real and mock installers)
type ToolRunner interface {
	findBinaryPath(owner, repo, version string) (string, error)
	GetResolver() ToolResolver
	createLatestFile(owner, repo, version string) error
	readLatestFile(owner, repo string) (string, error)
}

func runToolWithInstaller(installer ToolRunner, cmd *cobra.Command, args []string) error {
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

	// Parse tool into owner/repo using installer's tool resolution
	owner, repo, err := installer.GetResolver().Resolve(tool)
	if err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}

	// Get the binary path for this version
	binaryPath, err := installer.findBinaryPath(owner, repo, version)
	if err != nil {
		// Binary path not found, try to install it automatically
		fmt.Printf("🔧 Tool %s@%s is not installed. Installing automatically...\n", tool, version)

		// Use the same installation UI as the install command
		installErr := InstallSingleTool(owner, repo, version, false, true)
		if installErr != nil {
			return fmt.Errorf("failed to auto-install %s@%s: %w. Run 'toolchain install %s/%s@%s' manually",
				tool, version, installErr, owner, repo, version)
		}

		// Get the binary path again after installation
		binaryPath, err = installer.findBinaryPath(owner, repo, version)
		if err != nil {
			return fmt.Errorf("failed to find binary path after installation: %w", err)
		}
	} else {
		// Check if binary exists, and if not, try to install it automatically
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			fmt.Printf("🔧 Tool %s@%s is not installed. Installing automatically...\n", tool, version)

			// Use the same installation UI as the install command
			installErr := InstallSingleTool(owner, repo, version, false, true)
			if installErr != nil {
				return fmt.Errorf("failed to auto-install %s@%s: %w. Run 'toolchain install %s/%s@%s' manually",
					tool, version, installErr, owner, repo, version)
			}

			// Get the binary path again after installation
			binaryPath, err = installer.findBinaryPath(owner, repo, version)
			if err != nil {
				return fmt.Errorf("failed to find binary path after installation: %w", err)
			}
		}
	}

	// Execute the binary with the provided arguments
	cmdExec := exec.Command(binaryPath, remainingArgs...)
	cmdExec.Stdout = cmd.OutOrStdout()
	cmdExec.Stderr = cmd.ErrOrStderr()
	cmdExec.Stdin = os.Stdin
	return cmdExec.Run()
}

func runTool(cmd *cobra.Command, args []string) error {
	installer := NewInstaller()
	return runToolWithInstaller(installer, cmd, args)
}
