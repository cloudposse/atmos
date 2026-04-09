package version

import (
	"fmt"

	goversion "github.com/hashicorp/go-version"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ValidateConstraint checks if the current Atmos version satisfies the given constraint.
// Returns (satisfied bool, error). If constraintStr is empty, returns (true, nil).
//
// The constraint string uses hashicorp/go-version syntax (same as Terraform):
//   - ">=1.2.3"                    - Minimum version
//   - "<2.0.0"                     - Maximum version (exclusive)
//   - ">=1.2.3, <2.0.0"            - Range (AND logic)
//   - ">=2.5.0, !=2.7.0, <3.0.0"   - Complex (multiple AND constraints)
//   - "~>1.2"                      - Pessimistic constraint (>=1.2.0, <1.3.0)
//   - "~>1.2.3"                    - Pessimistic constraint (>=1.2.3, <1.3.0)
//   - "1.2.3"                      - Exact version
func ValidateConstraint(constraintStr string) (bool, error) {
	defer perf.Track(nil, "version.ValidateConstraint")()

	if constraintStr == "" {
		return true, nil
	}

	current, err := goversion.NewVersion(Version)
	if err != nil {
		return false, fmt.Errorf("invalid current version %q: %w", Version, err)
	}

	constraints, err := goversion.NewConstraint(constraintStr)
	if err != nil {
		return false, fmt.Errorf("invalid version constraint %q: %w", constraintStr, err)
	}

	return constraints.Check(current), nil
}
