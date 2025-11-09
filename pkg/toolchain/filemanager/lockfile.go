package filemanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain/lockfile"
)

// LockFileManager manages toolchain.lock.yaml file.
type LockFileManager struct {
	config   *schema.AtmosConfiguration
	filePath string
}

// NewLockFileManager creates a new lock file manager.
func NewLockFileManager(config *schema.AtmosConfiguration) *LockFileManager {
	defer perf.Track(nil, "filemanager.NewLockFileManager")()

	filePath := config.Toolchain.LockFile
	if filePath == "" {
		// Default: install_path/toolchain.lock.yaml
		installPath := config.Toolchain.InstallPath
		if installPath == "" {
			installPath = ".tools"
		}
		filePath = filepath.Join(installPath, "toolchain.lock.yaml")
	}

	return &LockFileManager{
		config:   config,
		filePath: filePath,
	}
}

// Enabled returns true if this file manager is enabled by configuration.
func (m *LockFileManager) Enabled() bool {
	defer perf.Track(nil, "filemanager.LockFileManager.Enabled")()

	return m.config.Toolchain.UseLockFile
}

// AddTool adds or updates a tool version in the lock file with optional platform-specific metadata.
// Creates a new lock file if one doesn't exist. Platform metadata can be specified via AddOption parameters.
func (m *LockFileManager) AddTool(ctx context.Context, tool, version string, opts ...AddOption) error {
	defer perf.Track(nil, "filemanager.LockFileManager.AddTool")()

	if !m.Enabled() {
		return nil
	}

	cfg := &AddConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Load existing lock file or create new
	lock, err := lockfile.Load(m.filePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		// Create new if doesn't exist
		lock = lockfile.New()
	}

	// Get or create tool entry
	entry := lock.GetOrCreateTool(tool)
	entry.Version = version

	// Add platform-specific information if provided
	platform := cfg.Platform
	if platform == "" {
		platform = runtime.GOOS + "_" + runtime.GOARCH
	}

	if cfg.URL != "" || cfg.Checksum != "" {
		platformEntry := &lockfile.PlatformEntry{
			URL:      cfg.URL,
			Checksum: cfg.Checksum,
			Size:     cfg.Size,
		}
		entry.Platforms[platform] = platformEntry
	}

	// Save lock file
	return lockfile.Save(m.filePath, lock)
}

// RemoveTool removes a tool version.
func (m *LockFileManager) RemoveTool(ctx context.Context, tool, version string) error {
	defer perf.Track(nil, "filemanager.LockFileManager.RemoveTool")()

	if !m.Enabled() {
		return nil
	}

	lock, err := lockfile.Load(m.filePath)
	if err != nil {
		// Treat missing file as no-op.
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Look up the specific tool entry.
	existingTool, exists := lock.Tools[tool]
	if !exists {
		// Tool not in lockfile - no-op.
		return nil
	}

	// If caller specified a version, verify it matches.
	if version != "" && existingTool.Version != version {
		return fmt.Errorf("refusing to remove tool '%s': lockfile has version '%s' but removal requested version '%s'",
			tool, existingTool.Version, version)
	}

	// Remove the tool entry.
	lock.RemoveTool(tool)
	return lockfile.Save(m.filePath, lock)
}

// SetDefault sets a tool version as default.
func (m *LockFileManager) SetDefault(ctx context.Context, tool, version string) error {
	defer perf.Track(nil, "filemanager.LockFileManager.SetDefault")()

	if !m.Enabled() {
		return nil
	}

	// Update tool version in lock file
	return m.AddTool(ctx, tool, version)
}

// GetTools returns all tools managed by the lock file as a map of tool names to version slices.
// Each tool can have multiple versions listed, with the first being the default/active version.
func (m *LockFileManager) GetTools(ctx context.Context) (map[string][]string, error) {
	defer perf.Track(nil, "filemanager.LockFileManager.GetTools")()

	if !m.Enabled() {
		return nil, nil
	}

	lock, err := lockfile.Load(m.filePath)
	if err != nil {
		return nil, err
	}

	// Convert lock file format to simple version map
	tools := make(map[string][]string)
	for name, entry := range lock.Tools {
		tools[name] = []string{entry.Version}
	}
	return tools, nil
}

// Verify verifies the integrity of the managed file.
func (m *LockFileManager) Verify(ctx context.Context) error {
	defer perf.Track(nil, "filemanager.LockFileManager.Verify")()

	if !m.Enabled() {
		return nil
	}

	return lockfile.Verify(m.filePath)
}

// Name returns the manager name for logging.
func (m *LockFileManager) Name() string {
	defer perf.Track(nil, "filemanager.LockFileManager.Name")()

	return "lockfile"
}
