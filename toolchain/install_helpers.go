package toolchain

import (
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/ui"
)

// spinnerControl manages the lifecycle of a Bubble Tea spinner.
type spinnerControl struct {
	program        *tea.Program
	showingSpinner bool
}

// installResult holds information about a successful tool installation.
type installResult struct {
	owner                  string
	repo                   string
	version                string
	binaryPath             string
	isLatest               bool
	showMessage            bool
	showHint               bool // Controls PATH export hint display.
	skipToolVersionsUpdate bool // Skip .tool-versions update (caller handles it).
}

// startSpinner starts a spinner with the given message.
// Spinner is only started if we're in a TTY environment and debug logging is disabled.
func (sc *spinnerControl) start(message string) {
	if !sc.showingSpinner {
		return
	}

	// Don't start spinner when debug logging is enabled - it suppresses log output.
	// Debug logs go to stderr, and Bubble Tea's TUI also controls stderr, causing
	// log messages to be hidden or garbled.
	if log.GetLevel() <= log.DebugLevel {
		log.Debug("Spinner disabled during debug logging", "message", message)
		return
	}

	// Don't start Bubble Tea spinner in non-TTY environments (CI, piped output).
	// This prevents potential hangs when tea.Program.Run() blocks on terminal init.
	if !isTTY() {
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
		ui.Toastf("📦", "Using latest version `%s`", latestVersion)
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
				ui.Errorf("Failed to create latest file for %s/%s: %v", result.owner, result.repo, err)
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
	ui.Successf("Installed `%s/%s@%s` to `%s`%s", result.owner, result.repo, result.version, result.binaryPath, sizeStr)

	// Register in .tool-versions (unless caller handles it separately).
	if result.skipToolVersionsUpdate {
		// Only show PATH hint when running toolchain install directly, not for dependency installs.
		if result.showHint {
			ui.Hintf("Export the `PATH` environment variable for your toolchain tools using `eval \"$(atmos --chdir /path/to/project toolchain env)\"`")
		}
		return
	}

	if err := AddToolToVersions(DefaultToolVersionsFilePath, result.repo, result.version); err == nil {
		ui.Successf("Registered `%s %s` in `.tool-versions`", result.repo, result.version)
		// Only show PATH hint when running toolchain install directly, not for dependency installs.
		if result.showHint {
			ui.Hintf("Export the `PATH` environment variable for your toolchain tools using `eval \"$(atmos --chdir /path/to/project toolchain env)\"`")
		}
	}
}
