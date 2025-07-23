package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	"github.com/spf13/cobra"
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

func runUninstall(cmd *cobra.Command, args []string) error {
	// If no arguments, uninstall from .tool-versions
	if len(args) == 0 {
		return uninstallFromToolVersions(".tool-versions")
	}

	packageSpec := args[0]
	parts := strings.Split(packageSpec, "@")
	if len(parts) != 2 {
		return fmt.Errorf("invalid package specification: %s. Expected format: owner/repo@version or tool@version", packageSpec)
	}
	repoPath := parts[0]
	version := parts[1]

	// Use the enhanced parseToolSpec to handle both owner/repo and tool name formats
	installer := NewInstaller()
	owner, repo, err := installer.parseToolSpec(repoPath)
	if err != nil {
		return fmt.Errorf("invalid repository path: %s. Expected format: owner/repo or tool name", repoPath)
	}

	return uninstallSinglePackage(owner, repo, version)
}

func uninstallSinglePackage(owner, repo, version string) error {
	// Create installer
	installer := NewInstaller()

	// Check if the package is actually installed first
	_, err := installer.findBinaryPath(owner, repo, version)
	if err != nil {
		// Package not installed, return error immediately
		return fmt.Errorf("package %s/%s@%s is not installed", owner, repo, version)
	}

	// Suppress logger immediately to prevent interference with progress bar
	restoreLogger := SuppressLogger()
	defer restoreLogger()

	spinner := bspinner.New()
	progressBar := progress.New(progress.WithDefaultGradient())
	percent := 0.0
	phase := "Uninstalling"

	// Start spinner
	spinner.Tick()
	fmt.Fprint(os.Stderr, "\n")

	// Show progress for finding package
	percent = 0.2
	phase = "Finding package"
	bar := progressBar.ViewAs(percent)
	fmt.Fprintf(os.Stderr, "\r%s %s %s %3.0f%%", spinner.View(), phase, bar, percent*100)
	os.Stderr.Sync()
	spinner.Tick()
	time.Sleep(200 * time.Millisecond)

	// Show progress for removing
	percent = 0.6
	phase = "Removing files"
	bar = progressBar.ViewAs(percent)
	fmt.Fprintf(os.Stderr, "\r%s %s %s %3.0f%%", spinner.View(), phase, bar, percent*100)
	os.Stderr.Sync()
	spinner.Tick()
	time.Sleep(200 * time.Millisecond)

	// Perform uninstallation
	err = installer.Uninstall(owner, repo, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\r%s Failed to uninstall %s/%s@%s: %v%s\n", xMark.Render(), owner, repo, version, err, strings.Repeat(" ", 50))
		return err
	}

	fmt.Fprintf(os.Stderr, "\r%s Uninstalled %s/%s@%s%s\n", checkMark.Render(), owner, repo, version, strings.Repeat(" ", 50))

	// Show completion
	percent = 1.0
	phase = "Complete"
	bar = progressBar.ViewAs(percent)
	fmt.Fprintf(os.Stderr, "\r%s %s %s %3.0f%%", spinner.View(), phase, bar, percent*100)
	os.Stderr.Sync()
	time.Sleep(500 * time.Millisecond)

	return nil
}

func uninstallFromToolVersions(toolVersionsPath string) error {
	installer := NewInstaller()

	// Load tool versions
	toolVersions, err := installer.loadToolVersions(toolVersionsPath)
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

	for tool, version := range toolVersions.Tools {
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
		phase := fmt.Sprintf("Uninstalling %s/%s@%s", pkg.owner, pkg.repo, pkg.version)
		bar := progressBar.ViewAs(percent)
		fmt.Fprintf(os.Stderr, "\r%s %s %s %3.0f%%", spinner.View(), phase, bar, percent*100)
		os.Stderr.Sync()
		spinner.Tick()
		time.Sleep(100 * time.Millisecond)

		// Uninstall the package
		err = installer.Uninstall(pkg.owner, pkg.repo, pkg.version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\r%s Failed to uninstall %s/%s@%s: %v%s\n", xMark.Render(), pkg.owner, pkg.repo, pkg.version, err, strings.Repeat(" ", 50))
			failedCount++
		} else {
			uninstalledCount++
		}
	}

	// Show final completion
	percent := 1.0
	phase := "Complete"
	bar := progressBar.ViewAs(percent)
	fmt.Fprintf(os.Stderr, "\r%s %s %s %3.0f%%", spinner.View(), phase, bar, percent*100)
	os.Stderr.Sync()
	time.Sleep(500 * time.Millisecond)

	// Show summary
	if failedCount == 0 {
		fmt.Fprintf(os.Stderr, "\r%s Successfully uninstalled %d packages%s\n", checkMark.Render(), uninstalledCount, strings.Repeat(" ", 50))
	} else {
		fmt.Fprintf(os.Stderr, "\r⚠️  Uninstalled %d packages, %d failed%s\n", uninstalledCount, failedCount, strings.Repeat(" ", 50))
	}

	return nil
}
