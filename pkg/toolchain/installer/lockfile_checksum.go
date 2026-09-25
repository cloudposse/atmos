package installer

import (
	"crypto/sha256"
	"crypto/sha512"
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

// lockChecksumAlgorithm prefers recorded digest metadata and preserves the upstream
// algorithm when an older entry does not identify a recognizable SHA digest.
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
	if entry.ChecksumAlgorithm != "" {
		return entry.ChecksumAlgorithm, nil
	}
	return inferLockChecksumAlgorithm(entry.Checksum, fallback), nil
}

// inferLockChecksumAlgorithm recognizes legacy SHA digests by their hex length,
// retaining upstream verification metadata when inference is not possible.
func inferLockChecksumAlgorithm(checksum, fallback string) string {
	switch len(checksum) {
	case sha512.Size * 2:
		return "sha512"
	case sha256.Size * 2:
		return "sha256"
	default:
		return fallback
	}
}
