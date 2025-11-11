package toolchain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// Progress bar percentages for uninstall stages.
	progressFindingTool = 0.2
	progressRemoving    = 0.6

	// Spinner animation configuration.
	maxSpinnerUpdates = 5

	// Progress bar format string.
	progressBarFormat = "%s %s"
)

// Refactored runUninstall to accept an optional installer parameter for testability.
func runUninstallWithInstaller(cmd *cobra.Command, args []string, installer *Installer) error {
	// If no arguments, uninstall from tool-versions file
	if len(args) == 0 {
		return uninstallFromToolVersions(GetToolVersionsFilePath(), installer)
	}
	toolSpec := args[0]
	tool, version, err := ParseToolVersionArg(toolSpec)
	if err != nil {
		return err
	}
	if tool == "" {
		return fmt.Errorf("%w: %s. Expected format: owner/repo@version or tool@version", ErrInvalidToolSpec, toolSpec)
	}

	if installer == nil {
		installer = NewInstaller()
	}
	owner, repo, err := installer.parseToolSpec(tool)
	if err != nil {
		// For uninstall operations, be more lenient with tool names
		// If the tool resolver fails, assume the tool name is the repo name
		// and use a default owner (the tool name itself)
		owner = tool
		repo = tool
	}

	// If no version specified, uninstall all versions of this tool
	if version == "" {
		return uninstallAllVersionsOfTool(installer, owner, repo)
	}

	if version == "latest" {
		// Resolve actual version from latest file
		actualVersion, err := installer.ReadLatestFile(owner, repo)
		if err != nil {
			// If the latest file does not exist, return error (test expects this)
			latestFilePath := filepath.Join(installer.binDir, owner, repo, "latest")
			return errUtils.Build(errUtils.ErrLatestFileNotFound).
				WithExplanationf("Tool `%s/%s@latest` is not installed", owner, repo).
				WithHint("Install with `atmos toolchain install "+repo+"@latest`").
				WithHint("Or install specific version: `atmos toolchain install "+repo+"@1.5.0`").
				WithContext("tool", fmt.Sprintf("%s/%s", owner, repo)).
				WithContext("latest_file", latestFilePath).
				WithExitCode(2).
				Err()
		}
		version = actualVersion
		// Check if the versioned binary exists
		binaryPath := installer.getBinaryPath(owner, repo, version, "")
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

	return uninstallSingleTool(installer, owner, repo, version, true)
}

// RunUninstall removes tools by spec (owner/repo, tool, or ).
func RunUninstall(toolSpec string) error {
	defer perf.Track(nil, "toolchain.RunUninstall")()

	installer := NewInstaller()
	// If no arguments, uninstall from tool-versions file
	if len(toolSpec) == 0 {
		return uninstallFromToolVersions(GetToolVersionsFilePath(), installer)
	}
	tool, version, err := ParseToolVersionArg(toolSpec)
	if err != nil {
		return err
	}
	if tool == "" {
		return fmt.Errorf("%w: %s. Expected format: owner/repo@version or tool@version", ErrInvalidToolSpec, toolSpec)
	}

	owner, repo, err := installer.parseToolSpec(tool)
	if err != nil {
		// For uninstall operations, be more lenient with tool names
		// If the tool resolver fails, assume the tool name is the repo name
		// and use a default owner (the tool name itself)
		owner = tool
		repo = tool
	}

	// If no version specified, uninstall all versions of this tool
	if version == "" {
		return uninstallAllVersionsOfTool(installer, owner, repo)
	}

	if version == "latest" {
		// Resolve actual version from latest file
		actualVersion, err := installer.ReadLatestFile(owner, repo)
		if err != nil {
			// If the latest file does not exist, return error (test expects this)
			latestFilePath := filepath.Join(installer.binDir, owner, repo, "latest")
			return errUtils.Build(errUtils.ErrLatestFileNotFound).
				WithExplanationf("Tool `%s/%s@latest` is not installed", owner, repo).
				WithHint("Install with `atmos toolchain install "+repo+"@latest`").
				WithHint("Or install specific version: `atmos toolchain install "+repo+"@1.5.0`").
				WithContext("tool", fmt.Sprintf("%s/%s", owner, repo)).
				WithContext("latest_file", latestFilePath).
				WithExitCode(2).
				Err()
		}
		version = actualVersion
		// Check if the versioned binary exists
		binaryPath := installer.getBinaryPath(owner, repo, version, "")
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

	return uninstallSingleTool(installer, owner, repo, version, true)
}

// Keep the original runUninstall for CLI usage.
func runUninstall(cmd *cobra.Command, args []string) error {
	return runUninstallWithInstaller(cmd, args, nil)
}

// Change uninstallSingleTool to accept showProgressBar.
func uninstallSingleTool(installer *Installer, owner, repo, version string, showProgressBar bool) error {
	// Check if the tool is actually installed first
	_, err := installer.FindBinaryPath(owner, repo, version)
	if err != nil {
		// If the binary is not found, treat as success (idempotent delete)
		if errors.Is(err, ErrToolNotFound) || os.IsNotExist(err) {
			if showProgressBar {
				printStatusLine(fmt.Sprintf("%s %s/%s@%s not installed", theme.Styles.Checkmark, owner, repo, version))
			}
			return nil
		}
		return err
	}

	if showProgressBar {
		spinner := bspinner.New()
		progressBar := progress.New(progress.WithDefaultGradient())
		percent := 0.0

		// Show progress for finding tool
		percent = progressFindingTool
		bar := progressBar.ViewAs(percent)
		printProgressBar(fmt.Sprintf(progressBarFormat, spinner.View(), bar))
		spinner, _ = spinner.Update(bspinner.TickMsg{})
		time.Sleep(100 * time.Millisecond)

		// Show progress for removing
		percent = progressRemoving
		bar = progressBar.ViewAs(percent)
		printProgressBar(fmt.Sprintf(progressBarFormat, spinner.View(), bar))
		spinner.Update(bspinner.TickMsg{})
		time.Sleep(100 * time.Millisecond)
	}

	// Perform uninstallation
	err = installer.Uninstall(owner, repo, version)
	if err != nil {
		// If the binary is already gone, treat as success
		if errors.Is(err, ErrToolNotFound) || os.IsNotExist(err) {
			if showProgressBar {
				printStatusLine(fmt.Sprintf("%s %s/%s@%s not installed", theme.Styles.Checkmark, owner, repo, version))
			}
			return nil
		}
		if showProgressBar {
			printStatusLine(fmt.Sprintf("%s %s/%s@%s failed to uninstall: %v", theme.Styles.XMark, owner, repo, version, err))
		}
		return err
	}

	// Show completion
	if showProgressBar {
		percent := 1.0
		progressBar := progress.New(progress.WithDefaultGradient())
		spinner := bspinner.New()
		bar := progressBar.ViewAs(percent)
		printProgressBar(fmt.Sprintf(progressBarFormat, spinner.View(), bar))
		time.Sleep(100 * time.Millisecond)
		// Clear the line before printing the summary
		resetLine()
		printStatusLine(fmt.Sprintf("%s %s/%s@%s uninstalled", theme.Styles.Checkmark, owner, repo, version))
	}

	return nil
}

// Update uninstallFromToolVersions to accept an optional installer.
func uninstallFromToolVersions(toolVersionsPath string, installer *Installer) error {
	if installer == nil {
		installer = NewInstaller()
	}

	// Load tool versions
	toolVersions, err := LoadToolVersions(toolVersionsPath)
	if err != nil {
		return fmt.Errorf("%w: failed to load %s: %v", ErrFileOperation, toolVersionsPath, err)
	}

	// Count total tools for progress tracking
	totalTools := len(toolVersions.Tools)
	if totalTools == 0 {
		_ = ui.Writef("No tools found in %s\n", toolVersionsPath)
		return nil
	}

	spinner := bspinner.New()
	spinner.Spinner = bspinner.Dot
	styles := theme.GetCurrentStyles()
	spinner.Style = styles.Spinner
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
				_ = ui.Warningf("Skipping invalid tool specification: %s", tool)
				continue
			}
			_, err = installer.FindBinaryPath(owner, repo, version)
			if err == nil {
				installedTools = append(installedTools, struct {
					tool    string
					version string
					owner   string
					repo    string
				}{tool, version, owner, repo})
			} else {
				_ = ui.Successf("Skipped %s/%s@%s (not installed)", owner, repo, version)
			}
		}
	}

	if len(installedTools) == 0 {
		_ = ui.Writeln("No tools to uninstall")
		return nil
	}

	installedCount := 0
	failedCount := 0
	alreadyRemovedCount := 0

	for i, tool := range installedTools {
		version := tool.version
		var msg string
		// Check if tool is installed
		_, err := installer.FindBinaryPath(tool.owner, tool.repo, version)
		if err != nil {
			msg = fmt.Sprintf("%s %s/%s@%s not installed", theme.Styles.Checkmark, tool.owner, tool.repo, version)
			alreadyRemovedCount++
		} else {
			err = uninstallSingleTool(installer, tool.owner, tool.repo, version, false)
			if err == nil {
				msg = fmt.Sprintf("%s Uninstalled %s/%s@%s", theme.Styles.Checkmark, tool.owner, tool.repo, version)
				installedCount++
			} else {
				msg = fmt.Sprintf("%s Uninstall failed %s/%s@%s: %v", theme.Styles.XMark, tool.owner, tool.repo, version, err)
				failedCount++
			}
		}
		percent := float64(i+1) / float64(len(installedTools))
		bar := progressBar.ViewAs(percent)
		resetLine()
		ui.Writeln(msg)
		// Show animated progress for a moment
		for j := 0; j < maxSpinnerUpdates; j++ {
			printProgressBar(fmt.Sprintf(progressBarFormat, spinner.View(), bar))
			spinner, _ = spinner.Update(bspinner.TickMsg{})
			time.Sleep(50 * time.Millisecond)
		}
	}
	resetLine()
	ui.Writeln("")

	// Print summary message based on results
	switch {
	case totalTools == 0:
		printStatusLine(fmt.Sprintf("%s no tools to uninstall", theme.Styles.Checkmark))
	case failedCount == 0 && alreadyRemovedCount == 0:
		printStatusLine(fmt.Sprintf("%s uninstalled %d tools", theme.Styles.Checkmark, installedCount))
	case failedCount == 0 && alreadyRemovedCount > 0:
		printStatusLine(fmt.Sprintf("%s uninstalled %d tools, skipped %d", theme.Styles.Checkmark, installedCount, alreadyRemovedCount))
	case failedCount > 0:
		printStatusLine(fmt.Sprintf("%s uninstalled %d tools, %d failed, skipped %d", theme.Styles.XMark, installedCount, failedCount, alreadyRemovedCount))
	default:
		printStatusLine(fmt.Sprintf("%s uninstalled %d tools, %d failed, skipped %d", theme.Styles.Checkmark, installedCount, failedCount, alreadyRemovedCount))
	}

	return nil
}

// uninstallAllVersionsOfTool uninstalls all versions of a specific tool.
// GetVersionsToUninstall reads the tool directory and returns version directories to uninstall.
func getVersionsToUninstall(toolDir string) ([]string, error) {
	entries, err := os.ReadDir(toolDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read tool directory %s: %w", toolDir, err)
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "latest" {
			versions = append(versions, entry.Name())
		}
	}
	return versions, nil
}

func uninstallAllVersionsOfTool(installer *Installer, owner, repo string) error {
	toolDir := filepath.Join(installer.binDir, owner, repo)

	// Check if the tool directory exists.
	if _, err := os.Stat(toolDir); os.IsNotExist(err) {
		return ui.Successf("Tool %s/%s is not installed", owner, repo)
	}

	versionsToUninstall, err := getVersionsToUninstall(toolDir)
	if err != nil {
		return err
	}

	if len(versionsToUninstall) == 0 {
		return ui.Successf("No versions of %s/%s are installed", owner, repo)
	}

	// Only show the "Uninstalling all versions" message if there's more than 1 version.
	if len(versionsToUninstall) > 1 {
		_ = ui.Writef("Uninstalling all versions of %s/%s (%d versions)\n", owner, repo, len(versionsToUninstall))
	}

	// Uninstall each version.
	for _, version := range versionsToUninstall {
		if err := uninstallSingleTool(installer, owner, repo, version, false); err != nil {
			_ = ui.Errorf("Failed to uninstall %s/%s@%s: %v", owner, repo, version, err)
		} else {
			_ = ui.Successf("Uninstalled %s/%s@%s", owner, repo, version)
		}
	}

	// Remove the latest file if it exists.
	latestFile := filepath.Join(toolDir, "latest")
	if _, err := os.Stat(latestFile); err == nil {
		_ = os.Remove(latestFile)
	}

	// Try to remove the tool directory (will only succeed if empty).
	_ = os.Remove(toolDir)

	// Only show summary if there are multiple versions.
	if len(versionsToUninstall) > 1 {
		return ui.Successf("Uninstalled all versions of %s/%s", owner, repo)
	}
	return nil
}
