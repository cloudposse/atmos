package toolchain

import (
	"fmt"
	"os"
	"sort"

	"github.com/Masterminds/semver/v3"
	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/ui"
)

// toolIdentity holds the resolved owner, repo, and alias for a tool.
type toolIdentity struct {
	owner string
	repo  string
	alias string
}

// toolRowInfo holds basic information about a tool for building a row.
type toolRowInfo struct {
	owner     string
	repo      string
	alias     string
	version   string
	isDefault bool
}

// handleToolVersionsLoadError handles errors from loading .tool-versions file.
func handleToolVersionsLoadError(err error, toolVersionsFile string) error {
	// Handle missing file gracefully with a helpful message.
	if os.IsNotExist(err) {
		message := "# No Configuration Found\n\n" +
			fmt.Sprintf("No `.tool-versions` file found at: `%s`\n\n", toolVersionsFile) +
			"## To get started:\n\n" +
			"Add a tool to automatically create your configuration:\n\n" +
			"```shell\n" +
			"atmos toolchain add terraform@1.6.0\n" +
			"```\n\n" +
			"Then list your tools:\n\n" +
			"```shell\n" +
			"atmos toolchain list\n" +
			"```\n"
		_ = ui.MarkdownMessage(message)
		return nil
	}
	return fmt.Errorf("failed to load .tool-versions: %w", err)
}

// buildToolRows creates toolRow entries for all tools and their versions.
func buildToolRows(toolVersions *ToolVersions, installer *Installer) []toolRow {
	var rows []toolRow

	for toolName, versions := range toolVersions.Tools {
		if len(versions) == 0 {
			continue
		}

		// Resolve tool information.
		id, err := resolveToolInfo(toolName, toolVersions, installer)
		if err != nil {
			_ = ui.Warningf("Could not resolve tool '%s': %v", toolName, err)
			continue
		}

		// Add row for default version (first in list).
		row := buildToolRow(installer, toolRowInfo{
			owner:     id.owner,
			repo:      id.repo,
			alias:     id.alias,
			version:   versions[0],
			isDefault: true,
		})
		rows = append(rows, row)

		// Add rows for additional versions.
		for i := 1; i < len(versions); i++ {
			row := buildToolRow(installer, toolRowInfo{
				owner:     id.owner,
				repo:      id.repo,
				alias:     id.alias,
				version:   versions[i],
				isDefault: false,
			})
			rows = append(rows, row)
		}
	}

	return rows
}

// resolveToolInfo resolves owner, repo, and alias for a tool name.
func resolveToolInfo(toolName string, toolVersions *ToolVersions, installer *Installer) (toolIdentity, error) {
	// Use existing infrastructure to resolve tool.
	resolvedKey, _, found := LookupToolVersion(toolName, toolVersions, installer.resolver)
	if !found {
		return toolIdentity{}, fmt.Errorf("%w: %s", ErrToolNotFound, toolName)
	}

	// Get owner/repo from resolved key.
	owner, repo, err := installer.resolver.Resolve(resolvedKey)
	if err != nil {
		return toolIdentity{}, err
	}

	// Determine alias (if resolved key differs from tool name).
	alias := ""
	if resolvedKey != toolName {
		alias = toolName
	}

	return toolIdentity{
		owner: owner,
		repo:  repo,
		alias: alias,
	}, nil
}

// buildToolRow creates a single toolRow for a specific tool version.
func buildToolRow(installer *Installer, info toolRowInfo) toolRow {
	// Binary name defaults to repo name.
	binaryName := info.repo

	// Check installation status.
	binaryPath, err := installer.FindBinaryPath(info.owner, info.repo, info.version)
	isInstalled := err == nil

	// Debug: log the binary path.
	if isInstalled {
		log.Debug("Found binary path", "path", binaryPath, "owner", info.owner, "repo", info.repo, versionLogKey, info.version)
	}

	// Get installation metadata.
	status, installDate, size := getInstallationMetadata(binaryPath, isInstalled)

	return toolRow{
		alias:       info.alias,
		registry:    fmt.Sprintf("%s/%s", info.owner, info.repo),
		binary:      binaryName,
		version:     info.version,
		status:      status,
		installDate: installDate,
		size:        size,
		isDefault:   info.isDefault,
		isInstalled: isInstalled,
	}
}

// getInstallationMetadata retrieves status, install date, and size for a binary.
func getInstallationMetadata(binaryPath string, isInstalled bool) (status, installDate, size string) {
	status = uninstalledIndicator
	installDate = notAvailablePlaceholder
	size = notAvailablePlaceholder

	if !isInstalled {
		return
	}

	status = installedIndicator
	fileInfo, err := os.Stat(binaryPath)
	if err != nil {
		// Debug: log the error.
		log.Debug("Failed to get file info", "path", binaryPath, "error", err)
		return
	}

	size = formatFileSize(fileInfo.Size())
	installDate = fileInfo.ModTime().Format("2006-01-02 15:04")

	// Debug: log the file size.
	log.Debug("File size calculated", "path", binaryPath, sizeLogKey, size, "raw_size", fileInfo.Size())

	return
}

// sortToolRows sorts rows by registry, then by semantic version (newest first).
func sortToolRows(rows []toolRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].registry != rows[j].registry {
			return rows[i].registry < rows[j].registry
		}
		// If same registry, sort by semantic version (newest first).
		vi, errI := semver.NewVersion(rows[i].version)
		vj, errJ := semver.NewVersion(rows[j].version)
		if errI != nil || errJ != nil {
			// Fallback to lexicographic comparison if versions aren't valid semver.
			return rows[i].version > rows[j].version
		}
		return vi.GreaterThan(vj)
	})
}
