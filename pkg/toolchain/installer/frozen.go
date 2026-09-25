package installer

import (
	"fmt"
	"runtime"

	errUtils "github.com/cloudposse/atmos/errors"
)

// requireFrozenEntry checks policy even for warm binary caches. A cache hit must
// not make a missing project lock entry acceptable in CI.
func (i *Installer) requireFrozenEntry(owner, repo, version string) error {
	if !i.frozenLockFile {
		return nil
	}
	if version == "latest" {
		return fmt.Errorf("%w: latest is not an exact version", errUtils.ErrFrozenLockfile)
	}
	lf, err := loadInstallerLockFile(i.lockFilePath)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrFrozenLockfile, i.lockFilePath, err)
	}
	platform := runtime.GOOS + "_" + runtime.GOARCH
	tool := lf.Tools[owner+"/"+repo]
	if tool != nil && tool.Versions[version] != nil {
		entry := tool.Versions[version].Platforms[platform]
		if entry != nil && entry.Checksum != "" && entry.URL != "" {
			return nil
		}
	}
	return fmt.Errorf("%w: %s has no complete entry for %s/%s@%s (%s); run atmos toolchain lock with frozen mode disabled",
		errUtils.ErrFrozenLockfile, i.lockFilePath, owner, repo, version, platform)
}
