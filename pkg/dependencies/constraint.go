package dependencies

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// latestVersion is the sentinel spec that never restricts a merge or match.
const latestVersion = "latest"

// ValidateConstraint validates that a specific version satisfies a constraint.
func ValidateConstraint(version string, constraint string) error {
	defer perf.Track(nil, "dependencies.ValidateConstraint")()

	// "latest" always satisfies any constraint
	if constraint == latestVersion || version == latestVersion {
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
			result, err := mergeToolVersion(parentVersion, childVersion)
			if err != nil {
				return nil, fmt.Errorf("%w: tool %q child version %q must satisfy parent constraint %q: %w",
					errUtils.ErrDependencyConstraint, tool, childVersion, parentVersion, err)
			}
			merged[tool] = result
			continue
		}

		// Override with child version
		merged[tool] = childVersion
	}

	return merged, nil
}

// mergeToolVersion resolves one tool's merged version spec.
//
// A concrete parent (e.g. a .tool-versions pin) is a default, not policy, so
// any child spec overrides it; the installer later resolves child ranges
// against installed versions. A parent constraint expresses policy: a concrete
// child must satisfy it, and a child constraint is AND-combined with it so
// resolution honors both.
func mergeToolVersion(parentVersion, childVersion string) (string, error) {
	// "latest" or empty on either side never restricts the merge.
	if parentVersion == "" || parentVersion == latestVersion || childVersion == latestVersion {
		return childVersion, nil
	}

	// Concrete parent: a default the child overrides freely.
	if !isConstraint(parentVersion) {
		return childVersion, nil
	}

	// Constraint parent, concrete child: the child must satisfy the policy.
	if !isConstraint(childVersion) {
		if err := ValidateConstraint(childVersion, parentVersion); err != nil {
			return "", err
		}
		return childVersion, nil
	}

	// Both are constraints: combine them (comma = AND) after checking the
	// child parses, so resolution enforces the intersection.
	if _, err := semver.NewConstraint(childVersion); err != nil {
		return "", fmt.Errorf("%w: invalid constraint %q: %w", errUtils.ErrDependencyConstraint, childVersion, err)
	}
	return childVersion + ", " + parentVersion, nil
}
