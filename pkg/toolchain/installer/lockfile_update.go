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

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/schema"
	toolchainlock "github.com/cloudposse/atmos/pkg/toolchain/lockfile"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
	"github.com/cloudposse/atmos/pkg/xdg"
)

func resolveLockFilePath(config *schema.AtmosConfiguration) string {
	if config == nil {
		return ""
	}
	if config.Toolchain.LockFile != "" {
		return config.Toolchain.LockFile
	}
	return filepath.Join(resolveDefaultInstallPath(config.Toolchain.InstallPath), "toolchain.lock.yaml")
}

// resolveDefaultInstallPath mirrors toolchain.GetInstallPath()'s fallback chain exactly
// (pkg/toolchain/installer cannot import pkg/toolchain -- that would be a cycle -- so both
// independently call the shared pkg/xdg helper the same way). Without this, the lock file
// defaulted to a hardcoded relative ".tools" while actual tool installs defaulted to the XDG
// cache dir, so toolchain.lock.yaml and the tools it's supposed to lock lived in two entirely
// different directory trees whenever install_path was left unset.
func resolveDefaultInstallPath(installPath string) string {
	if installPath != "" {
		return installPath
	}
	if cacheDir, err := xdg.GetXDGCacheDir("toolchain", defaultMkdirPermissions); err == nil && cacheDir != "" {
		return cacheDir
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, ".tools")
	}
	return ".tools"
}

// checkLockFileChecksumMismatch compares a freshly downloaded, verified artifact's checksum
// against any existing toolchain.lock.yaml entry for the same tool/version/platform. It is
// read-only -- unlike updateLockFile, it never creates or mutates lock file entries, so a
// lookup miss (no lock file yet, or no matching tool/version/platform entry) is not an error,
// just "nothing to compare against yet."
//
// Callers that extract/install the downloaded artifact onto disk (installFromTool) MUST call
// this before extraction, not just rely on updateLockFile's (now removed) mismatch check after
// the fact: updateLockFile runs after extractAndInstall/os.Chmod, so a mismatch caught only
// there means the compromised or unexpectedly-changed binary was already placed in the install
// tree by the time the error is returned -- exactly the supply-chain guarantee use_lock_file
// exists to provide. See docs/fixes for the incident this closes.
func (i *Installer) checkLockFileChecksumMismatch(tool *registry.Tool, version string, result *verification.Result) error {
	if !i.verifyAgainstLock || i.lockFilePath == "" || result == nil || result.Checksum == "" {
		return nil
	}
	lf, err := loadInstallerLockFile(i.lockFilePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("load installer lockfile: %w", err)
	}
	toolName := tool.RepoOwner + "/" + tool.RepoName
	platform := runtime.GOOS + "_" + runtime.GOARCH
	existingChecksum := lookupLockedChecksum(lf, toolName, version, platform)
	if existingChecksum == "" || existingChecksum == result.Checksum {
		return nil
	}
	return errUtils.Build(ErrLockfileChecksumMismatch).
		WithExplanationf("the freshly downloaded checksum for %s@%s/%s doesn't match the one recorded in toolchain.lock.yaml", toolName, version, platform).
		WithHint("if this version legitimately changed upstream, run `atmos toolchain lock` to refresh the recorded checksum").
		WithHint("otherwise, this may indicate the download was tampered with or corrupted").
		WithContext("tool", toolName).
		WithContext("version", version).
		WithContext("platform", platform).
		Err()
}

// lookupLockedChecksum returns the checksum recorded for tool/version/platform in an
// already-loaded lock file, or "" if no matching entry exists at any level (unknown tool,
// unknown version, or unknown platform).
func lookupLockedChecksum(lf *installerLockFile, toolName, version, platform string) string {
	tool := lf.Tools[toolName]
	if tool == nil {
		return ""
	}
	versionEntry := tool.Versions[version]
	if versionEntry == nil {
		return ""
	}
	entry := versionEntry.Platforms[platform]
	if entry == nil {
		return ""
	}
	return entry.Checksum
}

// updateLockFile persists a successful, already-verified install's checksum into
// toolchain.lock.yaml. It no longer performs the mismatch check itself -- callers that extract
// an artifact onto disk must call checkLockFileChecksumMismatch beforehand (see its doc
// comment); this function assumes that check, if applicable, already passed.
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
		versionEntry := getOrCreateInstallerToolVersion(lf, toolName, version)
		platform := runtime.GOOS + "_" + runtime.GOARCH
		versionEntry.Source = tool.Registry
		versionEntry.BinaryName = tool.BinaryName
		versionEntry.Platforms[platform] = &toolchainlock.PlatformEntry{
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
	installerLockFile     = toolchainlock.LockFile
	installerLockTool     = toolchainlock.Tool
	installerVersionEntry = toolchainlock.VersionEntry
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
	return lf.GetOrCreateTool(name)
}

func getOrCreateInstallerToolVersion(lf *installerLockFile, name, version string) *installerVersionEntry {
	return getOrCreateInstallerTool(lf, name).GetOrCreateVersion(version)
}
