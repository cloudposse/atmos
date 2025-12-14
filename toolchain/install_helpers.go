package toolchain

import (
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudposse/atmos/pkg/ui"
)

// spinnerControl manages the lifecycle of a Bubble Tea spinner.
type spinnerControl struct {
	program        *tea.Program
	showingSpinner bool
}

// installResult holds information about a successful tool installation.
type installResult struct {
	owner       string
	repo        string
	version     string
	binaryPath  string
	isLatest    bool
	showMessage bool
}

// startSpinner starts a spinner with the given message.
func (sc *spinnerControl) start(message string) {
	if !sc.showingSpinner {
		return
	}

	sc.program = runBubbleTeaSpinner(message)
	go func() {
		// Run spinner in background, ignore error since program is managed by quit channel.
		_, _ = sc.program.Run()
	}()
}

// stop stops the current spinner.
func (sc *spinnerControl) stop() {
	if !sc.showingSpinner || sc.program == nil {
		return
	}

	sc.program.Send(installDoneMsg{})
	// Small delay to ensure spinner is cleared before printing output.
	time.Sleep(100 * time.Millisecond)
}

// restart restarts the spinner with a new message.
func (sc *spinnerControl) restart(message string) {
	sc.stop()
	time.Sleep(50 * time.Millisecond)
	sc.start(message)
}

// resolveLatestVersionWithSpinner resolves "latest" to a concrete version number.
func resolveLatestVersionWithSpinner(owner, repo, version string, isLatest bool, spinner *spinnerControl) (string, error) {
	if !isLatest || version != "latest" {
		return version, nil
	}

	// Resolve "latest" version (network call - can be slow).
	registry := NewAquaRegistry()
	latestVersion, err := registry.GetLatestVersion(owner, repo)
	if err != nil {
		spinner.stop()
		return "", fmt.Errorf("failed to get latest version for %s/%s: %w", owner, repo, err)
	}

	// Update spinner message to show actual version.
	if spinner.showingSpinner {
		spinner.stop()
		_ = ui.Toastf("📦", "Using latest version `%s`", latestVersion)
		message := fmt.Sprintf("Installing %s/%s@%s", owner, repo, latestVersion)
		spinner.restart(message)
	}

	return latestVersion, nil
}

// handleInstallSuccess handles post-installation tasks and displays success messages.
func handleInstallSuccess(result installResult, installer *Installer) {
	// Create latest file if this is a "latest" installation.
	if result.isLatest {
		if err := installer.CreateLatestFile(result.owner, result.repo, result.version); err != nil {
			if result.showMessage {
				_ = ui.Errorf("Failed to create latest file for %s/%s: %v", result.owner, result.repo, err)
			}
		}
	}

	if !result.showMessage {
		return
	}

	// Calculate directory size for the installed version.
	versionDir := filepath.Dir(result.binaryPath)
	sizeStr := ""
	if dirSize, err := calculateDirectorySize(versionDir); err == nil {
		sizeStr = fmt.Sprintf(" (%s)", formatBytes(dirSize))
	}
	_ = ui.Successf("Installed `%s/%s@%s` to `%s`%s", result.owner, result.repo, result.version, result.binaryPath, sizeStr)

	// Register in .tool-versions.
	if err := AddToolToVersions(DefaultToolVersionsFilePath, result.repo, result.version); err == nil {
		_ = ui.Successf("Registered `%s %s` in `.tool-versions`", result.repo, result.version)
		_ = ui.Hintf("Export the `PATH` environment variable for your toolchain tools using `eval \"$(atmos --chdir /path/to/project toolchain env)\"`")
	}
}
