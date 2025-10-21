package exec

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/hashicorp/go-getter"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// handleDryRunInstall handles dry-run installation checks.
func handleDryRunInstall(p *pkgAtmosVendor, atmosConfig *schema.AtmosConfiguration) tea.Msg {
	log.Debug("Entering dry-run flow for generic (non component/mixin) vendoring ", "package", p.name)

	if needsCustomDetection(p.uri) {
		log.Debug("Custom detection required for URI", "uri", p.uri)
		detector := downloader.NewCustomGitDetector(atmosConfig, "")
		_, _, err := detector.Detect(p.uri, "")
		if err != nil {
			return installedPkgMsg{
				err:  fmt.Errorf("dry-run: detection failed: %w", err),
				name: p.name,
			}
		}
	} else {
		log.Debug("Skipping custom detection; URI already supported by go getter", "uri", p.uri)
	}

	time.Sleep(500 * time.Millisecond)
	return installedPkgMsg{
		err:  nil,
		name: p.name,
	}
}

// needsCustomDetection checks if a source needs custom detection.
// This is a replica of getForce method from go getter library, had to make it as it is not exported.
// The idea is to call Detect method in dry run only for those links where go getter does this.
// Otherwise, Detect is run for every link being vendored which isn't correct.
func needsCustomDetection(src string) bool {
	_, getSrc := "", src
	if idx := strings.Index(src, "::"); idx >= 0 {
		_, getSrc = src[:idx], src[idx+2:]
	}

	getSrc, _ = getter.SourceDirSubdir(getSrc)

	if absPath, err := filepath.Abs(getSrc); err == nil {
		if u.FileExists(absPath) {
			return false
		}
		isDir, err := u.IsDirectory(absPath)
		if err == nil && isDir {
			return false
		}
	}

	parsed, err := url.Parse(getSrc)
	if err != nil || parsed.Scheme == "" {
		return true
	}

	supportedSchemes := map[string]bool{
		"http":      true,
		"https":     true,
		"git":       true,
		"hg":        true,
		"s3":        true,
		"gcs":       true,
		"file":      true,
		"oci":       true,
		"ssh":       true,
		"git+ssh":   true,
		"git+https": true,
	}

	if _, ok := supportedSchemes[parsed.Scheme]; ok {
		return false
	}

	return true
}

// createTempDir creates a temporary directory for vendor operations.
func createTempDir() (string, error) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "atmos-vendor")
	if err != nil {
		return "", err
	}

	// Ensure directory permissions are restricted
	if err := os.Chmod(tempDir, tempDirPermissions); err != nil {
		return "", err
	}

	return tempDir, nil
}

// newInstallError creates a new installation error message.
func newInstallError(err error, name string) installedPkgMsg {
	return installedPkgMsg{
		err:  fmt.Errorf("%s: %w", name, err),
		name: name,
	}
}

// ExecuteInstall executes the installation of a vendor package.
func ExecuteInstall(installer pkgVendor, dryRun bool, atmosConfig *schema.AtmosConfiguration) tea.Cmd {
	if installer.atmosPackage != nil {
		return downloadAndInstall(installer.atmosPackage, dryRun, atmosConfig)
	}

	if installer.componentPackage != nil {
		return downloadComponentAndInstall(installer.componentPackage, dryRun, atmosConfig)
	}

	if installer.diffPackage != nil {
		return executeDiffCheck(installer.diffPackage)
	}

	// No valid package provided
	return func() tea.Msg {
		err := fmt.Errorf("%w: %s", errUtils.ErrValidPackage, installer.name)
		return installedPkgMsg{
			err:  err,
			name: installer.name,
		}
	}
}

// executeDiffCheck performs the version diff check for a component.
func executeDiffCheck(p *pkgVendorDiff) tea.Cmd {
	return func() tea.Msg {
		// If we already have the latest version from pre-check, use that directly
		if p.outdatedOnly && p.latestVersion != "" {
			// Format the result to show the update with pre-filled latest version
			return installedPkgMsg{
				err:  nil,
				name: fmt.Sprintf("📦 %s: %s → %s", p.name, p.currentVersion, p.latestVersion),
			}
		}

		// Otherwise, check for updates using the existing logic
		updateAvailable, latestInfo, err := checkForVendorUpdates(&p.source, true)
		if err != nil {
			return installedPkgMsg{
				err:  fmt.Errorf("%w: %v", errUtils.ErrCheckingForUpdates, err),
				name: p.name,
			}
		}

		if updateAvailable && latestInfo != "" {
			p.latestVersion = latestInfo
			// Format the result to show the update
			return installedPkgMsg{
				err:  nil,
				name: fmt.Sprintf("📦 %s: %s → %s", p.name, p.currentVersion, latestInfo),
			}
		} else if !p.outdatedOnly {
			return installedPkgMsg{
				err:  nil,
				name: fmt.Sprintf("%s %s: %s (up to date)", checkMark, p.name, p.currentVersion),
			}
		}

		// For outdatedOnly mode, don't show up-to-date components
		return installedPkgMsg{
			err:  nil,
			name: "", // Empty name means don't display
		}
	}
}

// max returns the maximum of two integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
