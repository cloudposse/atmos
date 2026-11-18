//go:build !linux && !darwin && !windows

package materialize

import (
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
)

func exchangeDirectories(_ *os.File, _ string, _ *os.File, _ string) error {
	return fmt.Errorf("%w: atomic directory replacement is unavailable on this platform", errUtils.ErrArtifactUnsupportedCapability)
}

func renameExclusive(_, _ string) error {
	return fmt.Errorf("%w: exclusive directory install is unavailable on this platform", errUtils.ErrArtifactUnsupportedCapability)
}
