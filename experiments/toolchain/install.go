package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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

var reinstallFlag bool

func init() {
	installCmd.Flags().Bool("default", false, "Set installed version as default (front of .tool-versions)")
	installCmd.Flags().BoolVar(&reinstallFlag, "reinstall", false, "Reinstall even if already installed")
}

func runInstall(cmd *cobra.Command, args []string) error {
	// If no arguments, install from .tool-versions
	if len(args) == 0 {
		return installFromToolVersions(".tool-versions")
	}

	setDefault, _ := cmd.Flags().GetBool("default")

	packageSpec := args[0]
	parts := strings.Split(packageSpec, "@")
	if len(parts) == 1 {
		// Try to look up version in .tool-versions or fallback to alias/latest
		tool := parts[0]
		toolVersions, err := LoadToolVersions(".tool-versions")
		if err != nil {
			return fmt.Errorf("invalid package specification: %s. Expected format: owner/repo@version or tool@version, and failed to load .tool-versions: %w", packageSpec, err)
		}
		installer := NewInstaller()
		resolvedKey, version, found, usedLatest := LookupToolVersionOrLatest(tool, toolVersions, installer.resolver)
		if !found && !usedLatest {
			return fmt.Errorf("invalid package specification: %s. Expected format: owner/repo@version or tool@version, and tool not found in .tool-versions or as an alias", packageSpec)
		}
		packageSpec = resolvedKey + "@" + version
		parts = strings.Split(packageSpec, "@")
	}
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

	err = InstallSinglePackage(owner, repo, version, isLatest, true)
	if err != nil {
		return err
	}

	// Update .tool-versions: add version, set as default if requested
	toolVersions, err := LoadToolVersions(".tool-versions")
	if err != nil {
		toolVersions = &ToolVersions{Tools: make(map[string][]string)}
	}
	AddVersionToTool(toolVersions, repoPath, version, setDefault)
	if err := SaveToolVersions(".tool-versions", toolVersions); err != nil {
		return fmt.Errorf("failed to update .tool-versions: %w", err)
	}

	return nil
}

func InstallSinglePackage(owner, repo, version string, isLatest bool, showProgressBar bool) error {
	restoreLogger := SuppressLogger()
	defer restoreLogger()
	installer := NewInstaller()
	if showProgressBar {
		spinner := bspinner.New()
		progressBar := progress.New(progress.WithDefaultGradient())
		phases := []struct {
			phase   string
			percent float64
		}{
			{"Looking up package", 0.1},
			{"Downloading", 0.3},
			{"Extracting", 0.7},
			{"Complete", 1.0},
		}
		fmt.Fprint(os.Stderr, "\n")
		// Phase 1: Looking up package
		bar := progressBar.ViewAs(phases[0].percent)
		printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
		spinner.Tick()
		time.Sleep(100 * time.Millisecond)
		// Phase 2: Downloading
		bar = progressBar.ViewAs(phases[1].percent)
		printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
		spinner.Tick()
		time.Sleep(100 * time.Millisecond)
	}
	// Perform installation
	binaryPath, err := installer.Install(owner, repo, version)
	if err != nil {
		if showProgressBar {
			fmt.Fprintf(os.Stderr, "\r%s Failed to install %s/%s@%s: %v\n", xMark.Render(), owner, repo, version, err)
		}
		return err
	}
	if isLatest {
		if err := installer.createLatestFile(owner, repo, version); err != nil {
			if showProgressBar {
				fmt.Fprintf(os.Stderr, "\r%s Failed to create latest file for %s/%s: %v\n", xMark.Render(), owner, repo, err)
			}
		}
	}
	if showProgressBar {
		// Phase 3: Extracting
		progressBar := progress.New(progress.WithDefaultGradient())
		spinner := bspinner.New()
		bar := progressBar.ViewAs(0.7)
		printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s Extracting %s %3.0f%%", spinner.View(), bar, 70.0))
		spinner.Tick()
		time.Sleep(100 * time.Millisecond)
		// Phase 4: Complete
		bar = progressBar.ViewAs(1.0)
		printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s Complete %s %3.0f%%", spinner.View(), bar, 100.0))
		time.Sleep(100 * time.Millisecond)
		if term.IsTerminal(int(os.Stderr.Fd())) {
			fmt.Fprint(os.Stderr, "\r\033[K")
		}
		fmt.Fprintf(os.Stderr, "%s Installed %s/%s@%s to %s\n", checkMark.Render(), owner, repo, version, binaryPath)
	}
	if err := AddToolToVersions(".tool-versions", repo, version); err == nil && showProgressBar {
		fmt.Fprintf(os.Stderr, "%s Registered %s %s in .tool-versions\n", checkMark.Render(), repo, version)
	}
	return nil
}

func installFromToolVersions(toolVersionsPath string) error {
	restoreLogger := SuppressLogger()
	defer restoreLogger()

	installer := NewInstaller()

	toolVersions, err := LoadToolVersions(toolVersionsPath)
	if err != nil {
		return fmt.Errorf("failed to load .tool-versions: %w", err)
	}

	totalPackages := len(toolVersions.Tools)
	if totalPackages == 0 {
		fmt.Fprintf(os.Stderr, "No packages found in %s\n", toolVersionsPath)
		return nil
	}

	spinner := bspinner.New()
	progressBar := progress.New(progress.WithDefaultGradient())

	installedCount := 0
	failedCount := 0
	alreadyInstalledCount := 0

	// In installFromToolVersions, add a history slice to track actions
	var history []string

	toolList := make([]struct {
		tool    string
		version string
		owner   string
		repo    string
	}, 0, totalPackages)
	for tool, versions := range toolVersions.Tools {
		owner, repo, err := installer.parseToolSpec(tool)
		if err != nil {
			continue
		}
		for _, version := range versions {
			toolList = append(toolList, struct {
				tool    string
				version string
				owner   string
				repo    string
			}{tool, version, owner, repo})
		}
	}

	for i, pkg := range toolList {
		version := pkg.version
		if version == "latest" {
			registry := NewAquaRegistry()
			latestVersion, err := registry.GetLatestVersion(pkg.owner, pkg.repo)
			if err != nil {
				msg := fmt.Sprintf("%s %s/%s@%s Failed to get latest version: %v", xMark.Render(), pkg.owner, pkg.repo, version, err)
				resetLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
				fmt.Fprintln(os.Stderr, msg)
				percent := float64(i+1) / float64(totalPackages)
				bar := progressBar.ViewAs(percent)
				printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
				time.Sleep(100 * time.Millisecond)
				history = append(history, msg)
				failedCount++
				continue
			}
			version = latestVersion
		}

		_, err := installer.findBinaryPath(pkg.owner, pkg.repo, version)
		if err == nil && !reinstallFlag {
			msg := fmt.Sprintf("%s Skipped %s/%s@%s (already installed)", checkMark.Render(), pkg.owner, pkg.repo, version)
			percent := float64(i+1) / float64(totalPackages)
			bar := progressBar.ViewAs(percent)
			resetLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
			fmt.Fprintln(os.Stderr, msg)
			printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
			time.Sleep(100 * time.Millisecond)
			history = append(history, msg)
			alreadyInstalledCount++
			continue
		}

		err = InstallSinglePackage(pkg.owner, pkg.repo, version, pkg.version == "latest", false)
		var msg string
		if err == nil {
			msg = fmt.Sprintf("%s Installed %s/%s@%s", checkMark.Render(), pkg.owner, pkg.repo, version)
			installedCount++
		} else {
			msg = fmt.Sprintf("%s Failed to install %s/%s@%s: %v", xMark.Render(), pkg.owner, pkg.repo, version, err)
			failedCount++
		}
		percent := float64(i+1) / float64(totalPackages)
		bar := progressBar.ViewAs(percent)
		resetLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
		fmt.Fprintln(os.Stderr, msg)
		printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
		time.Sleep(100 * time.Millisecond)
		history = append(history, msg)
	}
	// At the end, clear the progress bar line before printing the summary
	resetLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
	fmt.Fprintln(os.Stderr)
	if totalPackages == 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s No tools to install", checkMark.Render()))
	} else if failedCount == 0 && alreadyInstalledCount == 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s Installed %d tools", checkMark.Render(), installedCount))
	} else if failedCount == 0 && alreadyInstalledCount > 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s Installed %d tools, %d already installed", checkMark.Render(), installedCount, alreadyInstalledCount))
	} else if failedCount > 0 && alreadyInstalledCount == 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s Installed %d tools, %d failed", xMark.Render(), installedCount, failedCount))
	} else if failedCount > 0 && alreadyInstalledCount > 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s Installed %d tools, %d failed, %d already installed", xMark.Render(), installedCount, failedCount, alreadyInstalledCount))
	} else {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s Installed %d tools, %d failed, %d already installed", checkMark.Render(), installedCount, failedCount, alreadyInstalledCount))
	}

	return nil
}
