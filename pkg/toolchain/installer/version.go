package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/perf"
)

// CreateLatestFile creates a "latest" file that contains a pointer to the actual version.
func (i *Installer) CreateLatestFile(owner, repo, version string) error {
	defer perf.Track(nil, "toolchain.WriteSymlink")()

	// Create the latest file path.
	latestDir := filepath.Join(i.binDir, owner, repo)
	if err := os.MkdirAll(latestDir, defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create latest directory: %w", ErrFileOperation, err)
	}

	latestFilePath := filepath.Join(latestDir, "latest")

	// Write the version to the latest file.
	if err := os.WriteFile(latestFilePath, []byte(version), defaultFileWritePermissions); err != nil {
		return fmt.Errorf("%w: failed to write latest file: %w", ErrFileOperation, err)
	}

	log.Debug("Created latest file", "path", latestFilePath, "version", version)
	return nil
}

// ReadLatestFile reads the version from a "latest" file.
func (i *Installer) ReadLatestFile(owner, repo string) (string, error) {
	defer perf.Track(nil, "toolchain.Installer.ReadLatestFile")()

	latestFilePath := filepath.Join(i.binDir, owner, repo, "latest")

	data, err := os.ReadFile(latestFilePath)
	if err != nil {
		return "", fmt.Errorf("%w: failed to read latest file: %w", ErrFileOperation, err)
	}

	version := strings.TrimSpace(string(data))
	if version == "" {
		return "", fmt.Errorf("%w: latest file is empty", ErrFileOperation)
	}

	return version, nil
}

// GetResolver returns the tool resolver used by this installer.
func (i *Installer) GetResolver() ToolResolver {
	defer perf.Track(nil, "toolchain.GetResolver")()

	return i.resolver
}

// ListInstalledVersions returns a list of installed versions for a specific tool.
func (i *Installer) ListInstalledVersions(owner, repo string) ([]string, error) {
	defer perf.Track(nil, "toolchain.ListInstalledVersions")()

	toolDir := filepath.Join(i.binDir, owner, repo)

	// Check if the tool directory exists.
	if _, err := os.Stat(toolDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(toolDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read tool directory %s: %w", toolDir, err)
	}

	var versions []string
	for _, entry := range entries {
		// Only include directories that are not "latest" (which is a file pointer).
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}
	return versions, nil
}

// InstalledTool represents a tool found in the install directory with its versions.
type InstalledTool struct {
	Owner    string
	Repo     string
	Versions []string
}

// ListAllInstalledTools scans the install directory and returns all installed tools.
// It walks two levels deep: binDir/{owner}/{repo}/ and collects version directories.
func (i *Installer) ListAllInstalledTools() ([]InstalledTool, error) {
	defer perf.Track(nil, "toolchain.ListAllInstalledTools")()

	if _, err := os.Stat(i.binDir); os.IsNotExist(err) {
		return nil, nil
	}

	ownerEntries, err := os.ReadDir(i.binDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read install directory %s: %w", i.binDir, err)
	}

	var tools []InstalledTool
	for _, ownerEntry := range ownerEntries {
		if !ownerEntry.IsDir() {
			continue
		}
		owner := ownerEntry.Name()
		ownerDir := filepath.Join(i.binDir, owner)

		repoEntries, err := os.ReadDir(ownerDir)
		if err != nil {
			continue
		}

		for _, repoEntry := range repoEntries {
			if !repoEntry.IsDir() {
				continue
			}
			repo := repoEntry.Name()

			versions, err := i.ListInstalledVersions(owner, repo)
			if err != nil || len(versions) == 0 {
				continue
			}

			tools = append(tools, InstalledTool{
				Owner:    owner,
				Repo:     repo,
				Versions: versions,
			})
		}
	}

	return tools, nil
}
