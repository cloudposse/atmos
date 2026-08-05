//go:build !windows

package cache

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
)

func installWindowsTrust(certPath string) error {
	return fmt.Errorf("%w: Windows trust store installation is unavailable on this platform: %s", errUtils.ErrInvalidConfig, certPath)
}

func removeWindowsTrust(commonName string) error {
	return fmt.Errorf("%w: Windows trust store removal is unavailable on this platform: %s", errUtils.ErrInvalidConfig, commonName)
}
