//go:build windows

package materialize

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"

	errUtils "github.com/cloudposse/atmos/errors"
)

func exchangeDirectories(_ *os.File, _ string, _ *os.File, _ string) error {
	return fmt.Errorf("%w: atomic directory replacement is unavailable on Windows", errUtils.ErrArtifactUnsupportedCapability)
}

func renameExclusive(from, to string) error {
	source, err := moveFilePath(from)
	if err != nil {
		return err
	}
	target, err := moveFilePath(to)
	if err != nil {
		return err
	}
	return windows.MoveFile(source, target)
}

// moveFilePath returns the UTF-16 form of path as MoveFile accepts it: an absolute path beyond MAX_PATH
// gets the extended-length prefix, and a short path keeps its value.
func moveFilePath(path string) (*uint16, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return windows.UTF16PtrFromString(extendedLengthPath(absolute))
}
