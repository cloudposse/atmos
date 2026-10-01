package flags

import (
	"fmt"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ClosureDepthUnlimited is the NoOptDefVal for depth-carrying closure flags:
// a bare --include-dependencies / --include-dependents means unlimited depth.
const ClosureDepthUnlimited = "all"

// Canonical names of the depth-carrying dependency-closure flags.
const (
	FlagIncludeDependencies = "include-dependencies"
	FlagIncludeDependents   = "include-dependents"
)

// ParseClosureDepth parses a depth-carrying closure flag value into the
// internal encoding used by schema.ConfigAndStacksInfo: 0 = off, -1 =
// unlimited depth, N>0 = N levels.
//
// Accepted values: "" (flag absent → off), "all" (bare flag via NoOptDefVal →
// unlimited), a positive integer (that many levels), and — for backward
// compatibility with the flag's boolean past and ATMOS_INCLUDE_DEPENDENTS
// env values — "true" (unlimited) and "false"/"0" (off).
func ParseClosureDepth(flagName, value string) (int, error) {
	defer perf.Track(nil, "flags.ParseClosureDepth")()

	switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
	case "":
		return 0, nil
	case ClosureDepthUnlimited, "true":
		return -1, nil
	case "false", "0":
		return 0, nil
	default:
		depth, err := strconv.Atoi(normalized)
		if err != nil || depth < 1 {
			return 0, fmt.Errorf("%w: invalid --%s value %q: use the bare flag for unlimited depth, or a positive integer for N levels",
				errUtils.ErrInvalidFlagValue, flagName, value)
		}
		return depth, nil
	}
}
