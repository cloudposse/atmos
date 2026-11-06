package artifact

import (
	"fmt"
	"io/fs"
	"regexp"
	"strconv"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultArchivePermissions is the mode of a local archive file when permissions are not configured.
const DefaultArchivePermissions fs.FileMode = 0o600

const (
	// The numeric base of permission strings.
	octalBase = 8

	// The bit size of a parsed permission value.
	permissionBitSize = 32

	// The largest accepted mode: ordinary permission bits without setuid, setgid, or sticky.
	maxPermissionBits = 0o777
)

// permissionsPattern accepts three or four octal digits, for example "600" or "0600".
var permissionsPattern = regexp.MustCompile(`^[0-7]{3,4}$`)

// ParsePermissions parses a quoted octal permission string such as "0600". Only ordinary
// permission bits (at most 0777) are accepted: privileged bits (setuid, setgid, sticky) are
// rejected, as are "preserve" and non-octal forms such as "0o600".
func ParsePermissions(value string) (fs.FileMode, error) {
	defer perf.Track(nil, "artifact.ParsePermissions")()

	if !permissionsPattern.MatchString(value) {
		return 0, fmt.Errorf("%w: permissions %q must be a quoted octal mode such as \"0600\"", errUtils.ErrArtifactConfigInvalid, value)
	}
	parsed, err := strconv.ParseUint(value, octalBase, permissionBitSize)
	if err != nil {
		return 0, fmt.Errorf("%w: permissions %q: %w", errUtils.ErrArtifactConfigInvalid, value, err)
	}
	if parsed > maxPermissionBits {
		return 0, fmt.Errorf("%w: permissions %q must not set privileged mode bits (maximum \"0777\")", errUtils.ErrArtifactConfigInvalid, value)
	}
	return fs.FileMode(parsed), nil
}
