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
			err = uninstallSinglePackage(installer, owner, repo, version, true)
			_ = os.Remove(filepath.Join(installer.binDir, owner, repo, "latest"))
			_ = os.Remove(filepath.Join(installer.binDir, owner, repo)) // will only remove if empty
			return err
		}
	}

	err = uninstallSinglePackage(installer, owner, repo, version, true)

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

// Change uninstallSinglePackage to accept showProgressBar
func uninstallSinglePackage(installer *Installer, owner, repo, version string, showProgressBar bool) error {
	// Check if the package is actually installed first
	_, err := installer.findBinaryPath(owner, repo, version)
	if err != nil {
		// If the binary is not found, treat as success (idempotent delete)
		if strings.Contains(err.Error(), "not installed") || os.IsNotExist(err) {
			if showProgressBar {
				printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s/%s@%s not installed", checkMark.Render(), owner, repo, version))
			}
			return nil
		}
		return err
	}

	// Suppress logger immediately to prevent interference with progress bar
	restoreLogger := SuppressLogger()
	defer restoreLogger()

	if showProgressBar {
		spinner := bspinner.New()
		progressBar := progress.New(progress.WithDefaultGradient())
		percent := 0.0

		// Start spinner
		spinner.Tick()

		// Show progress for finding package
		percent = 0.2
		bar := progressBar.ViewAs(percent)
		printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
		spinner.Tick()
		time.Sleep(100 * time.Millisecond)

		// Show progress for removing
		percent = 0.6
		bar = progressBar.ViewAs(percent)
		printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
		spinner.Tick()
		time.Sleep(100 * time.Millisecond)
	}

	// Perform uninstallation
	err = installer.Uninstall(owner, repo, version)
	if err != nil {
		// If the binary is already gone, treat as success
		if strings.Contains(err.Error(), "not installed") || os.IsNotExist(err) {
			if showProgressBar {
				printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s/%s@%s not installed", checkMark.Render(), owner, repo, version))
			}
			return nil
		}
		if showProgressBar {
			printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s/%s@%s failed to uninstall: %v", xMark.Render(), owner, repo, version, err))
		}
		return err
	}

	// Show completion
	if showProgressBar {
		percent := 1.0
		progressBar := progress.New(progress.WithDefaultGradient())
		spinner := bspinner.New()
		bar := progressBar.ViewAs(percent)
		printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
		time.Sleep(100 * time.Millisecond)
		// Clear the line before printing the summary
		if term.IsTerminal(int(os.Stderr.Fd())) {
			fmt.Fprint(os.Stderr, "\r\033[K")
		}
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s/%s@%s uninstalled", checkMark.Render(), owner, repo, version))
	}

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

	// Suppress logger immediately to prevent interference with progress bar
	restoreLogger := SuppressLogger()
	defer restoreLogger()

	spinner := bspinner.New()
	progressBar := progress.New(progress.WithDefaultGradient())

	// Start spinner
	spinner.Tick()

	// Collect installed packages
	var installedPackages []struct {
		tool    string
		version string
		owner   string
		repo    string
	}
	for tool, versions := range toolVersions.Tools {
		for _, version := range versions {
			owner, repo, err := installer.parseToolSpec(tool)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  Skipping invalid tool specification: %s\n", tool)
				continue
			}
			_, err = installer.findBinaryPath(owner, repo, version)
			if err == nil {
				installedPackages = append(installedPackages, struct {
					tool    string
					version string
					owner   string
					repo    string
				}{tool, version, owner, repo})
			} else {
				fmt.Fprintf(os.Stderr, "%s Skipped %s/%s@%s (not installed)\n", checkMark.Render(), owner, repo, version)
			}
		}
	}

	if len(installedPackages) == 0 {
		fmt.Fprintf(os.Stderr, "No tools to uninstall\n")
		return nil
	}

	installedCount := 0
	failedCount := 0
	alreadyRemovedCount := 0

	for i, pkg := range installedPackages {
		version := pkg.version
		var msg string
		// Check if tool is installed
		_, err := installer.findBinaryPath(pkg.owner, pkg.repo, version)
		if err != nil {
			msg = fmt.Sprintf("%s %s/%s@%s not installed", checkMark.Render(), pkg.owner, pkg.repo, version)
			alreadyRemovedCount++
		} else {
			err = uninstallSinglePackage(installer, pkg.owner, pkg.repo, version, false)
			if err == nil {
				msg = fmt.Sprintf("%s Uninstalled %s/%s@%s", checkMark.Render(), pkg.owner, pkg.repo, version)
				installedCount++
			} else {
				msg = fmt.Sprintf("%s Uninstall failed %s/%s@%s: %v", xMark.Render(), pkg.owner, pkg.repo, version, err)
				failedCount++
			}
		}
		resetLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
		fmt.Fprintln(os.Stderr, msg)
		percent := float64(i+1) / float64(totalPackages)
		bar := progressBar.ViewAs(percent)
		printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
		time.Sleep(100 * time.Millisecond)
	}
	resetLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
	fmt.Fprintln(os.Stderr)
	if totalPackages == 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s no tools to uninstall", checkMark.Render()))
	} else if failedCount == 0 && alreadyRemovedCount == 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s uninstalled %d tools", checkMark.Render(), installedCount))
	} else if failedCount == 0 && alreadyRemovedCount > 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s uninstalled %d tools, skipped %d", checkMark.Render(), installedCount, alreadyRemovedCount))
	} else if failedCount > 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s uninstalled %d tools, %d failed, skipped %d", xMark.Render(), installedCount, failedCount, alreadyRemovedCount))
	} else {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s uninstalled %d tools, %d failed, skipped %d", checkMark.Render(), installedCount, failedCount, alreadyRemovedCount))
	}

	return nil
}
