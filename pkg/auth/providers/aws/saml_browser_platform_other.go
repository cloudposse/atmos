//go:build !darwin

package aws

import (
	"fmt"
	"runtime"

	errUtils "github.com/cloudposse/atmos/errors"
)

// darwinKernelInfo is only implemented on macOS.
func darwinKernelInfo() (string, string, error) {
	return "", "", fmt.Errorf("%w: no Darwin kernel information on %s", errUtils.ErrUnsupportedPlatform, runtime.GOOS)
}
