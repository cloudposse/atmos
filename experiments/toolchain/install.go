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

var installCmd = &cobra.Command{
	Use:   "install [package]",
	Short: "Install a CLI binary from the registry",
	Long: `Install a CLI binary using metadata from the registry.

The package should be specified in the format: owner/repo@version
Examples:
  toolchain install suzuki-shunsuke/github-comment@v3.5.0
  toolchain install hashicorp/terraform@v1.5.0
  toolchain install                    # Install from .tool-versions file`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInstall,
}

func runInstall(cmd *cobra.Command, args []string) error {
	// If no arguments, install from .tool-versions
	if len(args) == 0 {
		return installFromToolVersions(".tool-versions")
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

	// Handle "latest" keyword
	isLatest := false
	if version == "latest" {
		registry := NewAquaRegistry()
		latestVersion, err := registry.GetLatestVersion(owner, repo)
		if err != nil {
			return fmt.Errorf("failed to get latest version for %s/%s: %w", owner, repo, err)
		}
		fmt.Printf("📦 Using latest version: %s\n", latestVersion)
		version = latestVersion
		isLatest = true
	}

	return installSinglePackage(owner, repo, version, isLatest)
}

func installSinglePackage(owner, repo, version string, isLatest bool) error {
	// Suppress logger immediately to prevent interference with progress bar
	restoreLogger := SuppressLogger()
	defer restoreLogger()

	spinner := bspinner.New()
	progressBar := progress.New(progress.WithDefaultGradient())
	percent := 0.0
	phase := "Installing"

	// Start spinner
	spinner.Tick()
	fmt.Fprint(os.Stderr, "\n")

	// Create installer and perform real installation
	installer := NewInstaller()

	// Show progress for registry lookup
	percent = 0.1
	phase = "Looking up package"
	bar := progressBar.ViewAs(percent)
	fmt.Fprintf(os.Stderr, "\r%s %s %s %3.0f%%", spinner.View(), phase, bar, percent*100)
	os.Stderr.Sync()
	spinner.Tick()
	time.Sleep(200 * time.Millisecond)

	// Show progress for downloading
	percent = 0.3
	phase = "Downloading"
	bar = progressBar.ViewAs(percent)
	fmt.Fprintf(os.Stderr, "\r%s %s %s %3.0f%%", spinner.View(), phase, bar, percent*100)
	os.Stderr.Sync()
	spinner.Tick()
	time.Sleep(200 * time.Millisecond)

	// Perform installation
	binaryPath, err := installer.Install(owner, repo, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\r%s Failed to install %s/%s@%s: %v%s\n", xMark.Render(), owner, repo, version, err, strings.Repeat(" ", 50))
		return err
	}

	// Create latest file if this is a latest installation
	if isLatest {
		if err := installer.createLatestFile(owner, repo, version); err != nil {
			fmt.Fprintf(os.Stderr, "\r%s Failed to create latest file for %s/%s: %v%s\n", xMark.Render(), owner, repo, err, strings.Repeat(" ", 50))
			// Don't fail the installation, just warn
		}
	}

	// Show progress for extracting
	percent = 0.7
	phase = "Extracting"
	bar = progressBar.ViewAs(percent)
	fmt.Fprintf(os.Stderr, "\r%s %s %s %3.0f%%", spinner.View(), phase, bar, percent*100)
	os.Stderr.Sync()
	spinner.Tick()
	time.Sleep(200 * time.Millisecond)

	// Show completion
	percent = 1.0
	phase = "Complete"
	bar = progressBar.ViewAs(percent)
	fmt.Fprintf(os.Stderr, "\r%s %s %s %3.0f%%", spinner.View(), phase, bar, percent*100)
	os.Stderr.Sync()
	time.Sleep(500 * time.Millisecond)

	fmt.Fprintf(os.Stderr, "\r%s Installed %s/%s@%s to %s%s\n", checkMark.Render(), owner, repo, version, binaryPath, strings.Repeat(" ", 50))
	return nil
}

func installFromToolVersions(toolVersionsPath string) error {
	// Suppress logger immediately to prevent interference with progress bar
	restoreLogger := SuppressLogger()
	defer restoreLogger()

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

	spinner := bspinner.New()
	progressBar := progress.New(progress.WithDefaultGradient())

	// Start spinner
	spinner.Tick()
	fmt.Fprint(os.Stderr, "\n")

	installedCount := 0
	failedCount := 0

	for tool, version := range toolVersions.Tools {
		// Parse tool specification (owner/repo@version or just repo@version)
		owner, repo, err := installer.parseToolSpec(tool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\r⚠️  Skipping invalid tool specification: %s%s\n", tool, strings.Repeat(" ", 20))
			continue
		}

		// Handle "latest" keyword
		if version == "latest" {
			registry := NewAquaRegistry()
			latestVersion, err := registry.GetLatestVersion(owner, repo)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\r%s Failed to get latest version for %s/%s: %v%s\n", xMark.Render(), owner, repo, err, strings.Repeat(" ", 50))
				failedCount++
				continue
			}
			fmt.Fprintf(os.Stderr, "\r📦 Using latest version for %s/%s: %s%s\n", owner, repo, latestVersion, strings.Repeat(" ", 20))
			version = latestVersion
		}

		// Show progress for current package
		percent := float64(installedCount) / float64(totalPackages)
		phase := fmt.Sprintf("Installing %s/%s@%s", owner, repo, version)
		bar := progressBar.ViewAs(percent)
		fmt.Fprintf(os.Stderr, "\r%s %s %s %3.0f%%", spinner.View(), phase, bar, percent*100)
		os.Stderr.Sync()
		spinner.Tick()
		time.Sleep(100 * time.Millisecond)

		// Install the package
		_, err = installer.Install(owner, repo, version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\r%s Failed to install %s/%s@%s: %v%s\n", xMark.Render(), owner, repo, version, err, strings.Repeat(" ", 50))
			failedCount++
		} else {
			// Create latest file if the original version was "latest"
			if toolVersions.Tools[tool] == "latest" {
				if err := installer.createLatestFile(owner, repo, version); err != nil {
					fmt.Fprintf(os.Stderr, "\r%s Failed to create latest file for %s/%s: %v%s\n", xMark.Render(), owner, repo, err, strings.Repeat(" ", 50))
					// Don't fail the installation, just warn
				}
			}
			installedCount++
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
		fmt.Fprintf(os.Stderr, "\r%s Successfully installed %d packages%s\n", checkMark.Render(), installedCount, strings.Repeat(" ", 50))
	} else {
		fmt.Fprintf(os.Stderr, "\r⚠️  Installed %d packages, %d failed%s\n", installedCount, failedCount, strings.Repeat(" ", 50))
	}

	return nil
}
