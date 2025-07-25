package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec [tool[@version]] [flags...]",
	Short: "Exec a specific version of a tool (replaces current process)",
	Long: `Exec a specific version of a tool with arguments, replacing the current process.

If no version is specified, the latest version will be used.

Examples:
  toolchain exec terraform --version          # Uses latest version
  toolchain exec terraform@1.9.8 --version   # Uses specific version
  toolchain exec opentofu@1.10.1 init
  toolchain exec terraform@1.5.7 plan -var-file=prod.tfvars`,
	Args:               cobra.MinimumNArgs(1),
	RunE:               execTool,
	DisableFlagParsing: true,
}

var execFunc = syscall.Exec

func execTool(cmd *cobra.Command, args []string) error {
	installer := NewInstaller()
	return execToolWithInstaller(installer, cmd, args)
}

func execToolWithInstaller(installer ToolRunner, cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no arguments provided. Expected format: tool@version")
	}

	toolSpec := args[0]
	remainingArgs := args[1:]

	// Parse tool@version specification
	parts := strings.Split(toolSpec, "@")
	var tool, version string

	if len(parts) == 1 {
		tool = parts[0]
		version = "latest"
		usedLatest := true
		if toolVersions, err := LoadToolVersions(".tool-versions"); err == nil {
			_, configuredVersion, found := LookupToolVersion(tool, toolVersions, installer.GetResolver())
			if found {
				version = configuredVersion
				usedLatest = false
			}
		}
		if version == "latest" {
			owner, repo, err := installer.GetResolver().Resolve(tool)
			if err == nil {
				if latestVersion, err := installer.readLatestFile(owner, repo); err == nil {
					version = latestVersion
				}
			}
		}
		defer func() {
			if usedLatest {
				_ = AddToolToVersions(".tool-versions", tool, "latest")
				owner, repo, err := installer.GetResolver().Resolve(tool)
				if err == nil {
					_ = installer.createLatestFile(owner, repo, version)
				}
			}
		}()
	} else if len(parts) == 2 {
		tool = parts[0]
		version = parts[1]
	} else {
		return fmt.Errorf("invalid tool specification: %s. Expected format: tool or tool@version", toolSpec)
	}

	owner, repo, err := installer.GetResolver().Resolve(tool)
	if err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}

	binaryPath, err := installer.findBinaryPath(owner, repo, version)
	if err != nil {
		fmt.Printf("🔧 Tool %s@%s is not installed. Installing automatically...\n", tool, version)
		installErr := InstallSingleTool(owner, repo, version, false, true)
		if installErr != nil {
			return fmt.Errorf("failed to auto-install %s@%s: %w. Run 'toolchain install %s/%s@%s' manually",
				tool, version, installErr, owner, repo, version)
		}
		binaryPath, err = installer.findBinaryPath(owner, repo, version)
		if err != nil {
			return fmt.Errorf("failed to find binary path after installation: %w", err)
		}
	} else {
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			fmt.Printf("🔧 Tool %s@%s is not installed. Installing automatically...\n", tool, version)
			installErr := InstallSingleTool(owner, repo, version, false, true)
			if installErr != nil {
				return fmt.Errorf("failed to auto-install %s@%s: %w. Run 'toolchain install %s/%s@%s' manually",
					tool, version, installErr, owner, repo, version)
			}
			binaryPath, err = installer.findBinaryPath(owner, repo, version)
			if err != nil {
				return fmt.Errorf("failed to find binary path after installation: %w", err)
			}
		}
	}

	// Replace the current process with the tool binary
	return execFunc(binaryPath, append([]string{binaryPath}, remainingArgs...), os.Environ())
}
