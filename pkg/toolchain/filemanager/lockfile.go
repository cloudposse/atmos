package filemanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain/lockfile"
)

const lockDirectoryPermissions = 0o755

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

	return m.withExclusiveLock(ctx, func() error {
		lock, err := lockfile.Load(m.filePath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			lock = lockfile.New()
		}
		entry := lock.GetOrCreateTool(tool)
		entry.Version = version
		platform := cfg.Platform
		if platform == "" {
			platform = runtime.GOOS + "_" + runtime.GOARCH
		}
		if cfg.URL != "" || cfg.Checksum != "" {
			entry.Platforms[platform] = &lockfile.PlatformEntry{URL: cfg.URL, Checksum: cfg.Checksum, Size: cfg.Size}
		}
		return lockfile.Save(m.filePath, lock)
	})
}

// RemoveTool removes a tool version.
func (m *LockFileManager) RemoveTool(ctx context.Context, tool, version string) error {
	defer perf.Track(nil, "filemanager.LockFileManager.RemoveTool")()

	if !m.Enabled() {
		return nil
	}

	return m.withExclusiveLock(ctx, func() error {
		lock, err := lockfile.Load(m.filePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		existingTool, exists := lock.Tools[tool]
		if !exists {
			return nil
		}
		if version != "" && existingTool.Version != version {
			return errUtils.Build(errUtils.ErrLockfileVersionMismatch).
				WithExplanationf("Cannot remove tool `%s`: lockfile version does not match requested version", tool).
				WithHint("Update the lockfile or specify the correct version").
				WithHint("Run `atmos toolchain list` to see installed versions").
				WithContext("tool", tool).
				WithContext("lockfile_version", existingTool.Version).
				WithContext("requested_version", version).
				WithContext("lockfile", m.filePath).
				WithExitCode(2).
				Err()
		}
		lock.RemoveTool(tool)
		return lockfile.Save(m.filePath, lock)
	})
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
// Each tool maps to a single version (represented as a one-element slice).
func (m *LockFileManager) GetTools(ctx context.Context) (map[string][]string, error) {
	defer perf.Track(nil, "filemanager.LockFileManager.GetTools")()

	if !m.Enabled() {
		return nil, nil
	}

	var tools map[string][]string
	err := m.withSharedLock(ctx, func() error {
		lock, err := lockfile.Load(m.filePath)
		if err != nil {
			return err
		}
		tools = make(map[string][]string)
		for name, entry := range lock.Tools {
			tools[name] = []string{entry.Version}
		}
		return nil
	})
	return tools, err
}

// Verify verifies the integrity of the managed file.
func (m *LockFileManager) Verify(ctx context.Context) error {
	defer perf.Track(nil, "filemanager.LockFileManager.Verify")()

	if !m.Enabled() {
		return nil
	}

	return m.withSharedLock(ctx, func() error { return lockfile.Verify(m.filePath) })
}

// Name returns the manager name for logging.
func (m *LockFileManager) Name() string {
	defer perf.Track(nil, "filemanager.LockFileManager.Name")()

	return "lockfile"
}

func (m *LockFileManager) withExclusiveLock(ctx context.Context, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(m.filePath), lockDirectoryPermissions); err != nil {
		return err
	}
	return filelock.New(m.filePath+".lock").WithExclusive(ctx, fn)
}

func (m *LockFileManager) withSharedLock(ctx context.Context, fn func() error) error {
	return filelock.New(m.filePath+".lock").WithShared(ctx, fn)
}
