package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall [tool]",
	Short: "Uninstall a CLI binary from the registry",
	Long: `Uninstall a CLI binary using metadata from the registry.

The tool should be specified in the format: owner/repo@version or tool@version.
If no tool is specified, uninstalls all tools from the .tool-versions file.

Examples:
  toolchain uninstall terraform@1.9.8
  toolchain uninstall hashicorp/terraform@1.11.4
  toolchain uninstall                    # Uninstall all tools from .tool-versions`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUninstall,
}

// Refactored runUninstall to accept an optional installer parameter for testability
func runUninstallWithInstaller(cmd *cobra.Command, args []string, installer *Installer) error {
	// If no arguments, uninstall from tool-versions file
	if len(args) == 0 {
		return uninstallFromToolVersions(".tool-versions", installer)
	}
	toolSpec := args[0]
	tool, version, err := ParseToolVersionArg(toolSpec)
	if err != nil {
		return err
	}
	if tool == "" || version == "" {
		return fmt.Errorf("invalid tool specification: %s. Expected format: owner/repo@version or tool@version", toolSpec)
	}

	if installer == nil {
		installer = NewInstaller()
	}
	owner, repo, err := installer.parseToolSpec(tool)
	if err != nil {
		return fmt.Errorf("invalid repository path: %s. Expected format: owner/repo or tool name", tool)
	}

	if version == "latest" {
		// Resolve actual version from latest file
		actualVersion, err := installer.readLatestFile(owner, repo)
		if err != nil {
			// If the latest file does not exist, return error (test expects this)
			return fmt.Errorf("tool %s/%s@latest is not installed (no latest file found)", owner, repo)
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
			err = uninstallSingleTool(installer, owner, repo, version, true)
			_ = os.Remove(filepath.Join(installer.binDir, owner, repo, "latest"))
			_ = os.Remove(filepath.Join(installer.binDir, owner, repo)) // will only remove if empty
			return err
		}
	}

	err = uninstallSingleTool(installer, owner, repo, version, true)

	// Always attempt to remove the latest file and parent directory after uninstalling @latest
	if args[0] == toolSpec && strings.HasSuffix(toolSpec, "@latest") {
		_ = os.Remove(filepath.Join(installer.binDir, owner, repo, "latest"))
		_ = os.Remove(filepath.Join(installer.binDir, owner, repo)) // will only remove if empty
	}

	return err
}

// Keep the original runUninstall for CLI usage
func runUninstall(cmd *cobra.Command, args []string) error {
	return runUninstallWithInstaller(cmd, args, nil)
}

// Change uninstallSingleTool to accept showProgressBar
func uninstallSingleTool(installer *Installer, owner, repo, version string, showProgressBar bool) error {
	// Check if the tool is actually installed first
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

		// Show progress for finding tool
		percent = 0.2
		bar := progressBar.ViewAs(percent)
		printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
		spinner, _ = spinner.Update(bspinner.TickMsg{})
		time.Sleep(100 * time.Millisecond)

		// Show progress for removing
		percent = 0.6
		bar = progressBar.ViewAs(percent)
		printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
		spinner, _ = spinner.Update(bspinner.TickMsg{})
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

	// Count total tools for progress tracking
	totalTools := len(toolVersions.Tools)
	if totalTools == 0 {
		fmt.Fprintf(os.Stderr, "No tools found in %s\n", toolVersionsPath)
		return nil
	}

	// Suppress logger immediately to prevent interference with progress bar
	restoreLogger := SuppressLogger()
	defer restoreLogger()

	spinner := bspinner.New()
	spinner.Spinner = bspinner.Dot
	spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	progressBar := progress.New(progress.WithDefaultGradient())

	// Collect installed tools
	var installedTools []struct {
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
				installedTools = append(installedTools, struct {
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

	if len(installedTools) == 0 {
		fmt.Fprintf(os.Stderr, "No tools to uninstall\n")
		return nil
	}

	installedCount := 0
	failedCount := 0
	alreadyRemovedCount := 0

	for i, tool := range installedTools {
		version := tool.version
		var msg string
		// Check if tool is installed
		_, err := installer.findBinaryPath(tool.owner, tool.repo, version)
		if err != nil {
			msg = fmt.Sprintf("%s %s/%s@%s not installed", checkMark.Render(), tool.owner, tool.repo, version)
			alreadyRemovedCount++
		} else {
			err = uninstallSingleTool(installer, tool.owner, tool.repo, version, false)
			if err == nil {
				msg = fmt.Sprintf("%s Uninstalled %s/%s@%s", checkMark.Render(), tool.owner, tool.repo, version)
				installedCount++
			} else {
				msg = fmt.Sprintf("%s Uninstall failed %s/%s@%s: %v", xMark.Render(), tool.owner, tool.repo, version, err)
				failedCount++
			}
		}
		percent := float64(i+1) / float64(len(installedTools))
		bar := progressBar.ViewAs(percent)
		resetLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
		fmt.Fprintln(os.Stderr, msg)
		// Show animated progress for a moment
		for j := 0; j < 5; j++ {
			printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
			spinner, _ = spinner.Update(bspinner.TickMsg{})
			time.Sleep(50 * time.Millisecond)
		}
	}
	resetLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
	fmt.Fprintln(os.Stderr)
	if totalTools == 0 {
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
