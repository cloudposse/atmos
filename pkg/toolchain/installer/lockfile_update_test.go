package installer

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
	"github.com/cloudposse/atmos/pkg/xdg"
)

func TestInstallerLockFileConcurrentUpdatesPreserveAllTools(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "toolchain.lock.yaml")
	var wg sync.WaitGroup
	for _, name := range []string{"one", "two"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			installer := &Installer{useLockFile: true, lockFilePath: lockPath}
			assert.NoError(t, installer.updateLockFile(&registry.Tool{RepoOwner: "owner", RepoName: name}, "1.0.0", "https://example.com/"+name, &verification.Result{Checksum: name}))
		}(name)
	}
	wg.Wait()

	lf, err := loadInstallerLockFile(lockPath)
	require.NoError(t, err)
	assert.Contains(t, lf.Tools, "owner/one")
	assert.Contains(t, lf.Tools, "owner/two")
}

func TestResolveLockFilePath(t *testing.T) {
	assert.Empty(t, resolveLockFilePath(nil))
	assert.Equal(t, "custom.lock", resolveLockFilePath(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{LockFile: "custom.lock"},
	}))
	assert.Equal(t, filepath.Join("bin", "toolchain.lock.yaml"), resolveLockFilePath(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{InstallPath: "bin"},
	}))
}

// TestResolveLockFilePath_UnsetInstallPathMatchesGetInstallPathDefault reproduces a field-test
// finding: when toolchain.install_path is unset, resolveLockFilePath falls back to a hardcoded
// relative ".tools" directory, while toolchain.GetInstallPath() -- used for every actual binary
// install, and by `atmos toolchain clean` -- prefers the XDG cache dir first. On the common
// "nothing configured" setup this means toolchain.lock.yaml and the tools it's supposed to lock
// live in two entirely different directory trees. The fallback here must match
// GetInstallPath()'s XDG-first default (pkg/toolchain/installer cannot import pkg/toolchain
// directly -- that would be a cycle -- but both can and should independently call the shared
// pkg/xdg helper the same way).
func TestResolveLockFilePath_UnsetInstallPathMatchesGetInstallPathDefault(t *testing.T) {
	// Isolate from the real, shared machine cache -- never let this test resolve into the
	// user's actual ~/.cache/atmos/toolchain (see project_shared_cache_root_test_hazard).
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())

	wantCacheDir, err := xdg.GetXDGCacheDir("toolchain", 0o755)
	require.NoError(t, err)

	got := resolveLockFilePath(&schema.AtmosConfiguration{})
	assert.Equal(t, filepath.Join(wantCacheDir, "toolchain.lock.yaml"), got,
		"the lock file's default location must be inside the same directory GetInstallPath() resolves for actual tool installs, not a separate hardcoded '.tools'")
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
	tool := lf.Tools["owner/tool"]
	require.NotNil(t, tool)
	entry := tool.Versions["1.0.0"]
	require.NotNil(t, entry)
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

// TestInstallerLockFileToleratesMissingVersion preserves the pre-shared-loader behavior: a
// version-less lockfile loads with its entries intact (the next save stamps version 1) instead
// of failing the whole install with a parse error.
func TestInstallerLockFileToleratesMissingVersion(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "toolchain.lock.yaml")
	require.NoError(t, os.WriteFile(lockPath, []byte("tools:\n  owner/tool:\n    versions:\n      1.2.3: {}\n"), 0o644))

	lf, err := loadInstallerLockFile(lockPath)
	require.NoError(t, err)
	require.Contains(t, lf.Tools, "owner/tool")
	assert.Contains(t, lf.Tools["owner/tool"].Versions, "1.2.3")
}

func TestInstallerLockFileGetOrCreateTool(t *testing.T) {
	lf := &installerLockFile{}
	entry := getOrCreateInstallerTool(lf, "owner/tool")
	require.NotNil(t, entry)
	assert.NotNil(t, entry.Versions)

	assert.Same(t, entry, getOrCreateInstallerTool(lf, "owner/tool"))
}

func TestInstallerLockFileGetOrCreateToolVersion(t *testing.T) {
	lf := &installerLockFile{}
	entry := getOrCreateInstallerToolVersion(lf, "owner/tool", "1.0.0")
	require.NotNil(t, entry)
	assert.NotEmpty(t, entry.InstalledAt)
	assert.NotNil(t, entry.Platforms)

	entry.Platforms = nil
	assert.Same(t, entry, getOrCreateInstallerToolVersion(lf, "owner/tool", "1.0.0"))
	assert.NotNil(t, entry.Platforms)
}
