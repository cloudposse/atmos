package downloader

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
)

func prepareGitSSHKey(ctx context.Context, sshKey string) (path string, cleanup func(), resultErr error) {
	cleanup = func() {}
	if sshKey == "" {
		return "", cleanup, nil
	}
	if err := checkGitVersion(ctx, "2.3"); err != nil {
		return "", cleanup, fmt.Errorf("%w: %w", errUtils.ErrSSHKeyUsage, err)
	}
	raw, err := base64.StdEncoding.DecodeString(sshKey)
	if err != nil {
		return "", cleanup, fmt.Errorf("%w: decode SSH key: %w", errUtils.ErrSSHKeyUsage, err)
	}
	file, err := os.CreateTemp("", "go-getter")
	if err != nil {
		return "", cleanup, fmt.Errorf("%w: create SSH key file: %w", errUtils.ErrSSHKeyUsage, err)
	}
	defer file.Close()
	path = file.Name()
	cleanup = func() { _ = os.Remove(file.Name()) }
	defer func() {
		if resultErr != nil {
			cleanup()
		}
	}()
	if err := file.Chmod(sshKeyFileMode); err != nil {
		return "", cleanup, fmt.Errorf("%w: chmod SSH key: %w", errUtils.ErrSSHKeyUsage, err)
	}
	if _, err := file.Write(raw); err != nil {
		return "", cleanup, fmt.Errorf("%w: write SSH key: %w", errUtils.ErrSSHKeyUsage, err)
	}
	return path, cleanup, nil
}
