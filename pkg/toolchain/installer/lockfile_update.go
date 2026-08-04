package installer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/schema"
	toolchainlock "github.com/cloudposse/atmos/pkg/toolchain/lockfile"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
)

func resolveLockFilePath(config *schema.AtmosConfiguration) string {
	if config == nil {
		return ""
	}
	if config.Toolchain.LockFile != "" {
		return config.Toolchain.LockFile
	}
	installPath := config.Toolchain.InstallPath
	if installPath == "" {
		installPath = ".tools"
	}
	return filepath.Join(installPath, "toolchain.lock.yaml")
}

func (i *Installer) updateLockFile(tool *registry.Tool, version, assetURL string, result *verification.Result) error {
	if !i.useLockFile || i.lockFilePath == "" || result == nil || result.Checksum == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(i.lockFilePath), defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: mkdir %s: %w", ErrLockfileIO, filepath.Dir(i.lockFilePath), err)
	}
	lock := filelock.New(i.lockFilePath + ".lock")
	return lock.WithExclusive(context.Background(), func() error {
		lf, err := loadInstallerLockFile(i.lockFilePath)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("load installer lockfile: %w", err)
			}
			lf = newInstallerLockFile()
		}
		toolName := tool.RepoOwner + "/" + tool.RepoName
		entry := getOrCreateInstallerTool(lf, toolName)
		entry.Version = version
		entry.Source = tool.Registry
		entry.BinaryName = tool.BinaryName
		platform := runtime.GOOS + "_" + runtime.GOARCH
		entry.Platforms[platform] = &toolchainlock.PlatformEntry{
			URL:               assetURL,
			Checksum:          result.Checksum,
			Size:              result.AssetSize,
			ChecksumAlgorithm: result.ChecksumAlgorithm,
			Verification:      result.SignatureMethods,
		}
		if err := saveInstallerLockFile(i.lockFilePath, lf); err != nil {
			return fmt.Errorf("save installer lockfile: %w", err)
		}
		return nil
	})
}

type (
	installerLockFile = toolchainlock.LockFile
	installerLockTool = toolchainlock.Tool
)

func newInstallerLockFile() *installerLockFile {
	return toolchainlock.New()
}

func loadInstallerLockFile(filePath string) (*installerLockFile, error) {
	lf, err := toolchainlock.Load(filePath)
	if errors.Is(err, toolchainlock.ErrInvalidLockFile) {
		// A version-less (version: 0) lockfile predates the shared loader and was historically
		// tolerated here: load it leniently so its entries survive, and let the next Save
		// stamp version 1 rather than failing the whole install.
		lf, err = lenientInstallerLockFile(filePath)
	}
	if err != nil {
		var pathErr *fs.PathError
		// Filesystem failures (missing file, permission denied) classify as IO, not parse.
		if errors.Is(err, fs.ErrNotExist) || errors.As(err, &pathErr) {
			return nil, fmt.Errorf("%w: read %s: %w", ErrLockfileIO, filePath, err)
		}
		return nil, fmt.Errorf("%w: %s: %w", ErrLockfileParse, filePath, err)
	}
	if lf.Tools == nil {
		lf.Tools = make(map[string]*toolchainlock.Tool)
	}
	return lf, nil
}

func lenientInstallerLockFile(filePath string) (*installerLockFile, error) {
	data, err := os.ReadFile(filePath) // #nosec G304 -- filePath is the resolved toolchain lockfile path from installer configuration.
	if err != nil {
		return nil, err
	}
	var lf installerLockFile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, err
	}
	return &lf, nil
}

func saveInstallerLockFile(filePath string, lf *installerLockFile) error {
	if err := toolchainlock.Save(filePath, lf); err != nil {
		return fmt.Errorf("%w: write %s: %w", ErrLockfileIO, filePath, err)
	}
	return nil
}

func getOrCreateInstallerTool(lf *installerLockFile, name string) *installerLockTool {
	// GetOrCreateTool never returns nil; only the Platforms map needs backfilling for
	// entries loaded from lockfiles written before platforms were recorded.
	tool := lf.GetOrCreateTool(name)
	if tool.Platforms == nil {
		tool.Platforms = make(map[string]*toolchainlock.PlatformEntry)
	}
	return tool
}
