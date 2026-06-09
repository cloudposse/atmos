package installer

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
)

func TestResolveLockFilePath(t *testing.T) {
	assert.Empty(t, resolveLockFilePath(nil))
	assert.Equal(t, "custom.lock", resolveLockFilePath(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{LockFile: "custom.lock"},
	}))
	assert.Equal(t, filepath.Join(".tools", "toolchain.lock.yaml"), resolveLockFilePath(&schema.AtmosConfiguration{}))
	assert.Equal(t, filepath.Join("bin", "toolchain.lock.yaml"), resolveLockFilePath(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{InstallPath: "bin"},
	}))
}

func TestInstallerLockFileLoadSaveAndUpdate(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "locks", "toolchain.lock.yaml")
	installer := &Installer{useLockFile: true, lockFilePath: lockPath}

	err := installer.updateLockFile(&registry.Tool{
		RepoOwner:  "owner",
		RepoName:   "tool",
		Registry:   "aqua",
		BinaryName: "tool-bin",
	}, "1.0.0", "https://example.com/tool.tar.gz", &verification.Result{
		Checksum:          "abc123",
		ChecksumAlgorithm: "sha256",
		AssetSize:         42,
		SignatureMethods:  []string{"cosign"},
	})
	require.NoError(t, err)

	lf, err := loadInstallerLockFile(lockPath)
	require.NoError(t, err)
	entry := lf.Tools["owner/tool"]
	require.NotNil(t, entry)
	assert.Equal(t, "1.0.0", entry.Version)
	assert.Equal(t, "aqua", entry.Source)
	assert.Equal(t, "tool-bin", entry.BinaryName)
	platform := runtime.GOOS + "_" + runtime.GOARCH
	require.Contains(t, entry.Platforms, platform)
	assert.Equal(t, "abc123", entry.Platforms[platform].Checksum)
	assert.Equal(t, []string{"cosign"}, entry.Platforms[platform].Verification)
}

func TestInstallerLockFileSkipsWithoutRequiredData(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "toolchain.lock.yaml")
	installer := &Installer{useLockFile: false, lockFilePath: lockPath}
	require.NoError(t, installer.updateLockFile(&registry.Tool{}, "1.0.0", "url", &verification.Result{Checksum: "abc"}))
	assert.NoFileExists(t, lockPath)

	installer.useLockFile = true
	require.NoError(t, installer.updateLockFile(&registry.Tool{}, "1.0.0", "url", &verification.Result{}))
	assert.NoFileExists(t, lockPath)
}

func TestInstallerLockFileErrors(t *testing.T) {
	_, err := loadInstallerLockFile(filepath.Join(t.TempDir(), "missing.yaml"))
	require.ErrorIs(t, err, ErrLockfileIO)
	require.True(t, errors.Is(err, fs.ErrNotExist))

	badYAML := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(badYAML, []byte("tools: ["), 0o644))
	_, err = loadInstallerLockFile(badYAML)
	require.ErrorIs(t, err, ErrLockfileParse)

	dirPath := t.TempDir()
	err = saveInstallerLockFile(dirPath, newInstallerLockFile())
	require.ErrorIs(t, err, ErrLockfileIO)
}

func TestInstallerLockFileGetOrCreateTool(t *testing.T) {
	lf := &installerLockFile{}
	entry := lf.getOrCreateTool("owner/tool")
	require.NotNil(t, entry)
	assert.NotEmpty(t, entry.InstalledAt)
	assert.NotNil(t, entry.Platforms)

	entry.Platforms = nil
	assert.Same(t, entry, lf.getOrCreateTool("owner/tool"))
	assert.NotNil(t, entry.Platforms)
}
