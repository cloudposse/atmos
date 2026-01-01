package dependencies

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ValidateConstraint validates that a specific version satisfies a constraint.
func ValidateConstraint(version string, constraint string) error {
	defer perf.Track(nil, "dependencies.ValidateConstraint")()

	// "latest" always satisfies any constraint
	if constraint == "latest" || version == "latest" {
		return nil
	}

	// Empty constraint means any version is valid
	if constraint == "" {
		return nil
	}

	// Clean version strings (remove leading 'v' if present)
	cleanVersion := strings.TrimPrefix(version, "v")
	cleanConstraint := constraint

	// Parse constraint (e.g., "~> 1.10.0", "^0.54.0", ">= 1.9.0")
	c, err := semver.NewConstraint(cleanConstraint)
	if err != nil {
		return fmt.Errorf("%w: invalid constraint %q: %w", errUtils.ErrDependencyConstraint, constraint, err)
	}

	// Parse version
	v, err := semver.NewVersion(cleanVersion)
	if err != nil {
		return fmt.Errorf("%w: invalid version %q: %w", errUtils.ErrDependencyConstraint, version, err)
	}

	// Validate
	if !c.Check(v) {
		return fmt.Errorf("%w: version %q does not satisfy constraint %q", errUtils.ErrDependencyConstraint, version, constraint)
	}

	return nil
}

// MergeDependencies merges child dependencies into parent with constraint validation.
// Child values override parent values, but must satisfy parent constraints.
func MergeDependencies(parent map[string]string, child map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "dependencies.MergeDependencies")()

	// Start with parent dependencies
	merged := make(map[string]string)
	for tool, version := range parent {
		merged[tool] = version
	}

	// Merge child dependencies with validation
	for tool, childVersion := range child {
		parentVersion, hasParent := merged[tool]

		if hasParent {
			// Child overrides parent, but must satisfy parent constraint
			if err := ValidateConstraint(childVersion, parentVersion); err != nil {
				return nil, fmt.Errorf("%w: tool %q child version %q must satisfy parent constraint %q: %w",
					errUtils.ErrDependencyConstraint, tool, childVersion, parentVersion, err)
			}
		}

		// Override with child version
		merged[tool] = childVersion
	}

	return merged, nil
}
