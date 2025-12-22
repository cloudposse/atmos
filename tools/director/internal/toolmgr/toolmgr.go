// Package toolmgr manages tool versions for the director.
// It handles downloading, installing, and version pinning for tools like atmos.
package toolmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ToolConfig represents configuration for a single tool.
type ToolConfig struct {
	Version    string `yaml:"version" json:"version"`
	Source     string `yaml:"source" json:"source"`           // github, local, path
	InstallDir string `yaml:"install_dir" json:"install_dir"` // relative to demos/
}

// ToolsConfig represents the tools section in defaults.yaml.
type ToolsConfig struct {
	Atmos *ToolConfig `yaml:"atmos,omitempty" json:"atmos,omitempty"`
	// Future: terraform, vhs, etc.
}

// InstalledTool represents metadata for an installed tool.
type InstalledTool struct {
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
	Path        string    `json:"path"`
	SHA256      string    `json:"sha256,omitempty"`
}

// ToolsCache represents the tools.json cache file.
type ToolsCache struct {
	Tools map[string]*InstalledTool `json:"tools"`
}

// Manager handles tool version management.
type Manager struct {
	config   *ToolsConfig
	demosDir string
	cache    *ToolsCache
}

// New creates a new tool manager.
func New(config *ToolsConfig, demosDir string) *Manager {
	if config == nil {
		config = &ToolsConfig{}
	}
	return &Manager{
		config:   config,
		demosDir: demosDir,
		cache:    &ToolsCache{Tools: make(map[string]*InstalledTool)},
	}
}

// LoadCache loads the tools cache from disk.
func (m *Manager) LoadCache() error {
	cacheFile := filepath.Join(m.demosDir, ".cache", "tools.json")
	data, err := os.ReadFile(cacheFile)
	if os.IsNotExist(err) {
		return nil // No cache yet, that's fine.
	}
	if err != nil {
		return fmt.Errorf("failed to read tools cache: %w", err)
	}

	if err := json.Unmarshal(data, m.cache); err != nil {
		return fmt.Errorf("failed to parse tools cache: %w", err)
	}
	return nil
}

// SaveCache saves the tools cache to disk.
func (m *Manager) SaveCache() error {
	cacheDir := filepath.Join(m.demosDir, ".cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	cacheFile := filepath.Join(cacheDir, "tools.json")
	data, err := json.MarshalIndent(m.cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tools cache: %w", err)
	}

	if err := os.WriteFile(cacheFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write tools cache: %w", err)
	}
	return nil
}

// EnsureInstalled checks if a tool is installed at the correct version,
// installs it if missing, and returns the path to the binary.
func (m *Manager) EnsureInstalled(ctx context.Context, tool string) (string, error) {
	config := m.getToolConfig(tool)
	if config == nil {
		// No version pinning for this tool, use system PATH.
		path, err := exec.LookPath(tool)
		if err != nil {
			return "", fmt.Errorf("tool %s not found in PATH and not configured in defaults.yaml", tool)
		}
		return path, nil
	}

	// Load cache to check if already installed.
	if err := m.LoadCache(); err != nil {
		return "", err
	}

	installDir := config.InstallDir
	if installDir == "" {
		installDir = ".cache/bin"
	}

	absInstallDir := filepath.Join(m.demosDir, installDir)
	binaryPath := filepath.Join(absInstallDir, tool)

	// Check if already installed at correct version.
	if cached, ok := m.cache.Tools[tool]; ok {
		if cached.Version == config.Version {
			// Verify file still exists.
			if _, err := os.Stat(binaryPath); err == nil {
				return binaryPath, nil
			}
		}
	}

	// Need to install.
	fmt.Printf("Installing %s v%s...\n", tool, config.Version)

	if err := os.MkdirAll(absInstallDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create install directory: %w", err)
	}

	var err error
	switch tool {
	case "atmos":
		err = m.installAtmos(ctx, config, binaryPath)
	default:
		return "", fmt.Errorf("don't know how to install tool: %s", tool)
	}

	if err != nil {
		return "", err
	}

	// Update cache.
	m.cache.Tools[tool] = &InstalledTool{
		Version:     config.Version,
		InstalledAt: time.Now(),
		Path:        binaryPath,
	}

	if err := m.SaveCache(); err != nil {
		return "", err
	}

	fmt.Printf("✓ Installed %s v%s at %s\n", tool, config.Version, binaryPath)
	return binaryPath, nil
}

// GetPath returns the path to a tool's binary.
// Returns empty string if tool is not managed.
func (m *Manager) GetPath(tool string) string {
	config := m.getToolConfig(tool)
	if config == nil {
		return ""
	}

	installDir := config.InstallDir
	if installDir == "" {
		installDir = ".cache/bin"
	}

	return filepath.Join(m.demosDir, installDir, tool)
}

// Version returns the installed version of a tool.
func (m *Manager) Version(tool string) (string, error) {
	if err := m.LoadCache(); err != nil {
		return "", err
	}

	if cached, ok := m.cache.Tools[tool]; ok {
		return cached.Version, nil
	}

	// Not in cache, try to get version from binary.
	path := m.GetPath(tool)
	if path == "" {
		// Not managed, try system PATH.
		var err error
		path, err = exec.LookPath(tool)
		if err != nil {
			return "", fmt.Errorf("tool %s not found", tool)
		}
	}

	// Run tool --version.
	cmd := exec.Command(path, "version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get version: %w", err)
	}

	// Parse version from output (e.g., "atmos v1.145.0").
	version := strings.TrimSpace(string(output))
	if strings.HasPrefix(version, tool) {
		version = strings.TrimPrefix(version, tool)
		version = strings.TrimSpace(version)
	}
	if strings.HasPrefix(version, "v") {
		version = strings.TrimPrefix(version, "v")
	}

	return version, nil
}

// getToolConfig returns the configuration for a specific tool.
func (m *Manager) getToolConfig(tool string) *ToolConfig {
	switch tool {
	case "atmos":
		return m.config.Atmos
	default:
		return nil
	}
}

// PrependToPath prepends a directory to PATH environment variable.
func PrependToPath(dir string) {
	currentPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+string(os.PathListSeparator)+currentPath)
}
