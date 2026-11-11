//go:build windows

package materialize

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"

	errUtils "github.com/cloudposse/atmos/errors"
)

func exchangeDirectories(_ *os.File, _ string, _ *os.File, _ string) error {
	return fmt.Errorf("%w: atomic directory replacement is unavailable on Windows", errUtils.ErrArtifactUnsupportedCapability)
}

func renameExclusive(from, to string) error {
	source, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFile(source, target)
}
