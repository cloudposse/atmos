package toolchain

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"

	httpClient "github.com/cloudposse/atmos/pkg/http"
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

// searchRegistryForTool searches the Aqua registry for a tool by name.
func searchRegistryForTool(toolName string) (string, string, error) {
	defer perf.Track(nil, "searchRegistryForTool")()

	commonPaths := buildCommonRegistryPaths(toolName)
	return tryRegistryPaths(commonPaths, toolName)
}

// buildCommonRegistryPaths builds a list of common registry paths to try for a tool.
func buildCommonRegistryPaths(toolName string) []string {
	return []string{
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/%s/%s/registry.yaml", toolName, toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/hashicorp/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/cloudposse/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/kubernetes/kubernetes/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/helm/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/opentofu/%s/registry.yaml", toolName),
		fmt.Sprintf("https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs/%s/registry.yaml", toolName),
	}
}

// tryRegistryPaths attempts to find a tool in the given registry paths.
func tryRegistryPaths(paths []string, toolName string) (string, string, error) {
	defer perf.Track(nil, "tryRegistryPaths")()

	client := httpClient.NewDefaultClient(
		httpClient.WithGitHubToken(httpClient.GetGitHubTokenFromEnv()),
	)

	for _, path := range paths {
		owner, repo, found := tryRegistryPath(client, path)
		if found {
			return owner, repo, nil
		}
	}

	return "", "", fmt.Errorf("%w: '%s' not found in registry", ErrToolNotFound, toolName)
}

// httpDoer is an interface for making HTTP requests.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// tryRegistryPath attempts to fetch a single registry path.
func tryRegistryPath(client httpDoer, path string) (owner, repo string, found bool) {
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		return "", "", false
	}

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return "", "", false
	}
	resp.Body.Close()

	// Extract owner/repo from the URL.
	// path format: .../pkgs/owner/repo/registry.yaml
	parts := strings.Split(path, "/")
	if len(parts) >= minRegistryPathSegments {
		owner = parts[len(parts)-3]
		repo = parts[len(parts)-2]
		return owner, repo, true
	}

	return "", "", false
}
