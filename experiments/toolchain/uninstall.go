package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall [package]",
	Short: "Uninstall a CLI binary",
	Long: `Uninstall a CLI binary that was previously installed.

The package should be specified in the format: owner/repo@version or tool@version
Examples:
  toolchain uninstall terraform@1.9.8
  toolchain uninstall hashicorp/terraform@1.9.8
  toolchain uninstall                    # Uninstall from .tool-versions file`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUninstall,
}

// Refactored runUninstall to accept an optional installer parameter for testability
func runUninstallWithInstaller(cmd *cobra.Command, args []string, installer *Installer) error {
	// If no arguments, uninstall from .tool-versions
	if len(args) == 0 {
		return uninstallFromToolVersions(".tool-versions", installer)
	}

	packageSpec := args[0]
	parts := strings.Split(packageSpec, "@")
	if len(parts) == 1 {
		// Try to look up version in .tool-versions using LookupToolVersion
		tool := parts[0]
		toolVersions, err := LoadToolVersions(".tool-versions")
		if err != nil {
			return fmt.Errorf("invalid package specification: %s. Expected format: owner/repo@version or tool@version, and failed to load .tool-versions: %w", packageSpec, err)
		}
		if installer == nil {
			installer = NewInstaller()
		}
		resolvedKey, version, found := LookupToolVersion(tool, toolVersions, installer.resolver)
		if !found {
			return fmt.Errorf("invalid package specification: %s. Expected format: owner/repo@version or tool@version, and tool not found in .tool-versions", packageSpec)
		}
		packageSpec = resolvedKey + "@" + version
		parts = strings.Split(packageSpec, "@")
	}
	if len(parts) != 2 {
		return fmt.Errorf("invalid package specification: %s. Expected format: owner/repo@version or tool@version", packageSpec)
	}
	repoPath := parts[0]
	version := parts[1]

	if installer == nil {
		installer = NewInstaller()
	}
	owner, repo, err := installer.parseToolSpec(repoPath)
	if err != nil {
		return fmt.Errorf("invalid repository path: %s. Expected format: owner/repo or tool name", repoPath)
	}

	if version == "latest" {
		// Resolve actual version from latest file
		actualVersion, err := installer.readLatestFile(owner, repo)
		if err != nil {
			// If the latest file does not exist, return error (test expects this)
			return fmt.Errorf("package %s/%s@latest is not installed (no latest file found)", owner, repo)
		}
		version = actualVersion
		// Check if the versioned binary exists
		binaryPath := installer.getBinaryPath(owner, repo, version)
		if _, statErr := os.Stat(binaryPath); os.IsNotExist(statErr) {
			// Binary does not exist, but latest file does: delete latest file and return success
			_ = os.Remove(filepath.Join(installer.binDir, owner, repo, "latest"))
			_ = os.Remove(filepath.Join(installer.binDir, owner, repo)) // will only remove if empty
			return nil
		} else {
			// Both binary and latest file exist: uninstall binary, delete latest file, return nil
			err = uninstallSinglePackage(installer, owner, repo, version)
			_ = os.Remove(filepath.Join(installer.binDir, owner, repo, "latest"))
			_ = os.Remove(filepath.Join(installer.binDir, owner, repo)) // will only remove if empty
			return err
		}
	}

	err = uninstallSinglePackage(installer, owner, repo, version)

	// Always attempt to remove the latest file and parent directory after uninstalling @latest
	if args[0] == packageSpec && strings.HasSuffix(packageSpec, "@latest") {
		_ = os.Remove(filepath.Join(installer.binDir, owner, repo, "latest"))
		_ = os.Remove(filepath.Join(installer.binDir, owner, repo)) // will only remove if empty
	}

	return err
}

// Keep the original runUninstall for CLI usage
func runUninstall(cmd *cobra.Command, args []string) error {
	return runUninstallWithInstaller(cmd, args, nil)
}

// Change uninstallSinglePackage to accept installer
func uninstallSinglePackage(installer *Installer, owner, repo, version string) error {
	// Check if the package is actually installed first
	_, err := installer.findBinaryPath(owner, repo, version)
	if err != nil {
		// If the binary is not found, treat as success (idempotent delete)
		if strings.Contains(err.Error(), "not installed") || os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Suppress logger immediately to prevent interference with progress bar
	restoreLogger := SuppressLogger()
	defer restoreLogger()

	spinner := bspinner.New()
	progressBar := progress.New(progress.WithDefaultGradient())
	percent := 0.0

	// Start spinner
	spinner.Tick()
	fmt.Fprint(os.Stderr, "\n")

	// Show progress for finding package
	percent = 0.2
	bar := progressBar.ViewAs(percent)
	printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
	spinner.Tick()
	time.Sleep(200 * time.Millisecond)

	// Show progress for removing
	percent = 0.6
	bar = progressBar.ViewAs(percent)
	printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
	spinner.Tick()
	time.Sleep(200 * time.Millisecond)

	// Perform uninstallation
	err = installer.Uninstall(owner, repo, version)
	if err != nil {
		// If the binary is already gone, treat as success
		if strings.Contains(err.Error(), "not installed") || os.IsNotExist(err) {
			return nil
		}
		fmt.Fprintf(os.Stderr, "\r%s Failed to uninstall %s/%s@%s: %v\n", xMark.Render(), owner, repo, version, err)
		return err
	}

	fmt.Fprintf(os.Stderr, "%s Uninstalled %s/%s@%s\n", checkMark.Render(), owner, repo, version)

	// Show completion
	percent = 1.0
	bar = progressBar.ViewAs(percent)
	printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
	time.Sleep(500 * time.Millisecond)

	return nil
}

// Update uninstallFromToolVersions to accept an optional installer
func uninstallFromToolVersions(toolVersionsPath string, installer *Installer) error {
	if installer == nil {
		installer = NewInstaller()
	}

	// Load tool versions
	toolVersions, err := LoadToolVersions(toolVersionsPath)
	if err != nil {
		return fmt.Errorf("failed to load .tool-versions: %w", err)
	}

	// Count total packages for progress tracking
	totalPackages := len(toolVersions.Tools)
	if totalPackages == 0 {
		fmt.Fprintf(os.Stderr, "No packages found in %s\n", toolVersionsPath)
		return nil
	}

	// First, check which packages are actually installed
	var installedPackages []struct {
		tool    string
		version string
		owner   string
		repo    string
	}

	for tool, versions := range toolVersions.Tools {
		for _, version := range versions {
			// Parse tool specification (owner/repo@version or just repo@version)
			owner, repo, err := installer.parseToolSpec(tool)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  Skipping invalid tool specification: %s\n", tool)
				continue
			}

			// Check if the package is actually installed
			_, err = installer.findBinaryPath(owner, repo, version)
			if err == nil {
				// Package is installed, add to list for uninstallation
				installedPackages = append(installedPackages, struct {
					tool    string
					version string
					owner   string
					repo    string
				}{tool, version, owner, repo})
			} else {
				fmt.Fprintf(os.Stderr, "ℹ️  Package %s/%s@%s is not installed, skipping\n", owner, repo, version)
			}
		}
	}

	if len(installedPackages) == 0 {
		fmt.Fprintf(os.Stderr, "No installed packages found to uninstall\n")
		return nil
	}

	// Suppress logger immediately to prevent interference with progress bar
	restoreLogger := SuppressLogger()
	defer restoreLogger()

	spinner := bspinner.New()
	progressBar := progress.New(progress.WithDefaultGradient())

	// Start spinner
	spinner.Tick()
	fmt.Fprint(os.Stderr, "\n")

	uninstalledCount := 0
	failedCount := 0

	for _, pkg := range installedPackages {
		// Show progress for current package
		percent := float64(uninstalledCount) / float64(len(installedPackages))
		bar := progressBar.ViewAs(percent)
		printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
		spinner.Tick()
		time.Sleep(100 * time.Millisecond)

		// Uninstall the package
		err = uninstallSinglePackage(installer, pkg.owner, pkg.repo, pkg.version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\r%s Failed to uninstall %s/%s@%s: %v\n", xMark.Render(), pkg.owner, pkg.repo, pkg.version, err)
			failedCount++
		} else {
			uninstalledCount++
		}
	}

	// Show final completion
	percent := 1.0
	bar := progressBar.ViewAs(percent)
	printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
	time.Sleep(500 * time.Millisecond)

	// Show summary
	if failedCount == 0 {
		fmt.Fprintf(os.Stderr, "%s Successfully uninstalled %d packages\n", checkMark.Render(), uninstalledCount)
	} else {
		fmt.Fprintf(os.Stderr, "\r⚠️  Uninstalled %d packages, %d failed\n", uninstalledCount, failedCount)
	}

	return nil
}
