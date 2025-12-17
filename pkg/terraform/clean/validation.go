package clean

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// IsValidDataDir validates the TF_DATA_DIR environment variable value.
func IsValidDataDir(tfDataDir string) error {
	defer perf.Track(nil, "clean.IsValidDataDir")()

	if tfDataDir == "" {
		return ErrEmptyEnvDir
	}
	absTFDataDir, err := filepath.Abs(tfDataDir)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrResolveEnvDir, err)
	}

	// Check for root path on both Unix and Windows systems.
	if absTFDataDir == "/" || absTFDataDir == filepath.Clean("/") {
		return fmt.Errorf("%w: %s", ErrRefusingToDeleteDir, absTFDataDir)
	}

	// Windows-specific root path check (like C:\ or D:\).
	if len(absTFDataDir) == 3 && absTFDataDir[1:] == ":\\" {
		return fmt.Errorf("%w: %s", ErrRefusingToDeleteDir, absTFDataDir)
	}

	if strings.Contains(absTFDataDir, "..") {
		return fmt.Errorf("%w: %s", ErrRefusingToDelete, "..")
	}
	return nil
}
