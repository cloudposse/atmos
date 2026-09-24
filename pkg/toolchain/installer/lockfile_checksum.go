package installer

import (
	"errors"
	"os"
	"runtime"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
)

// prepareLockChecksum honors the locked digest algorithm even if upstream
// checksum metadata is missing, disabled, or has changed since locking.
// Upstream signature/checksum verification has already completed independently.
func (i *Installer) prepareLockChecksum(tool *registry.Tool, version, path string, result *verification.Result) error {
	if !i.useLockFile {
		return nil
	}
	algorithm := result.ChecksumAlgorithm
	if i.verifyAgainstLock {
		var err error
		algorithm, err = i.lockChecksumAlgorithm(tool.RepoOwner+"/"+tool.RepoName, version, algorithm)
		if err != nil {
			return err
		}
	}
	if algorithm == "" {
		algorithm = "sha256"
	}
	checksum, err := verification.DigestFile(path, algorithm)
	if err != nil {
		return err
	}
	result.Checksum, result.ChecksumAlgorithm = checksum, algorithm
	return nil
}

func (i *Installer) lockChecksumAlgorithm(toolName, version, fallback string) (string, error) {
	lf, err := loadInstallerLockFile(i.lockFilePath)
	if errors.Is(err, os.ErrNotExist) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	tool := lf.Tools[toolName]
	if tool == nil || tool.Versions[version] == nil {
		return fallback, nil
	}
	entry := tool.Versions[version].Platforms[runtime.GOOS+"_"+runtime.GOARCH]
	if entry == nil || entry.Checksum == "" {
		return fallback, nil
	}
	return entry.ChecksumAlgorithm, nil
}
