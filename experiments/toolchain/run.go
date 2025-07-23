package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run [tool@version] [flags...]",
	Short: "Run a specific version of a tool",
	Long: `Run a specific version of a tool with arguments.

Examples:
  toolchain run terraform@1.9.8 --version
  toolchain run opentofu@1.10.1 init
  toolchain run terraform@1.5.7 plan -var-file=prod.tfvars`,
	Args:               cobra.MinimumNArgs(1),
	RunE:               runTool,
	DisableFlagParsing: true,
}

func runTool(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no arguments provided. Expected format: tool@version")
	}

	toolSpec := args[0]
	remainingArgs := args[1:]

	// Parse tool@version specification
	parts := strings.Split(toolSpec, "@")
	if len(parts) != 2 {
		return fmt.Errorf("invalid tool specification: %s. Expected format: tool@version", toolSpec)
	}
	tool := parts[0]
	version := parts[1]

	// Parse tool into owner/repo using installer's tool resolution
	installer := NewInstaller()
	owner, repo, err := installer.resolveToolName(tool)
	if err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}

	// Get the binary path for this version
	binaryPath, err := installer.findBinaryPath(owner, repo, version)
	if err != nil {
		// Binary path not found, try to install it automatically
		fmt.Printf("🔧 Tool %s@%s is not installed. Installing automatically...\n", tool, version)

		// Try to install the tool
		_, installErr := installer.Install(owner, repo, version)
		if installErr != nil {
			return fmt.Errorf("failed to auto-install %s@%s: %w. Run 'toolchain install %s/%s@%s' manually",
				tool, version, installErr, owner, repo, version)
		}

		// Get the binary path again after installation
		binaryPath, err = installer.findBinaryPath(owner, repo, version)
		if err != nil {
			return fmt.Errorf("failed to find binary path after installation: %w", err)
		}

		fmt.Fprintf(os.Stderr, "%s Successfully installed %s@%s\n", checkMark.Render(), tool, version)
	} else {
		// Check if binary exists, and if not, try to install it automatically
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			fmt.Printf("🔧 Tool %s@%s is not installed. Installing automatically...\n", tool, version)

			// Try to install the tool
			_, installErr := installer.Install(owner, repo, version)
			if installErr != nil {
				return fmt.Errorf("failed to auto-install %s@%s: %w. Run 'toolchain install %s/%s@%s' manually",
					tool, version, installErr, owner, repo, version)
			}

			// Get the binary path again after installation
			binaryPath, err = installer.findBinaryPath(owner, repo, version)
			if err != nil {
				return fmt.Errorf("failed to find binary path after installation: %w", err)
			}

			fmt.Fprintf(os.Stderr, "%s Successfully installed %s@%s\n", checkMark.Render(), tool, version)
		}
	}

	// Execute the binary with remaining arguments
	fmt.Printf("🚀 Running %s@%s: %s %s\n", tool, version, binaryPath, strings.Join(remainingArgs, " "))

	execCmd := exec.Command(binaryPath, remainingArgs...)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Stdin = os.Stdin

	return execCmd.Run()
}
