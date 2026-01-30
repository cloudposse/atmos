package toolchain

import (
	"fmt"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// VersionType indicates the type of version specifier.
type VersionType int

const (
	// VersionTypeSemver represents a semantic version (e.g., "1.2.3", "v1.2.3").
	VersionTypeSemver VersionType = iota
	// VersionTypePR represents a PR number (e.g., "2040", "pr:2040").
	VersionTypePR
	// VersionTypeSHA represents a commit SHA (e.g., "sha:ceb7526", "ceb7526be").
	VersionTypeSHA
	// VersionTypeInvalid represents an invalid version format.
	VersionTypeInvalid
)

const (
	// Prefixes for explicit version specifiers.
	prPrefix  = "pr:"
	shaPrefix = "sha:"

	// Minimum length for auto-detected SHAs (short SHA).
	minSHALength = 7
	// Maximum length for a full SHA.
	maxSHALength = 40

	// Format string for invalid version errors.
	versionErrorFormat = "%w: '%s'"
)

// ParseVersionSpec detects the version type from an input string.
// Supports explicit prefixes (pr:, sha:) and auto-detection.
//   - All digits -> PR number.
//   - Hex string 7-40 chars with at least one letter a-f -> SHA.
//   - Valid semver pattern (X.Y.Z or vX.Y.Z) -> semver.
//   - Everything else -> error.
//
// Returns the version type, normalized value (prefix stripped), and any error.
func ParseVersionSpec(version string) (VersionType, string, error) {
	defer perf.Track(nil, "toolchain.ParseVersionSpec")()

	// Handle empty input.
	if version == "" {
		return VersionTypeInvalid, "", fmt.Errorf("%w: version cannot be empty", errUtils.ErrVersionFormatInvalid)
	}

	// 1. Explicit PR prefix takes precedence.
	if strings.HasPrefix(version, prPrefix) {
		prValue := strings.TrimPrefix(version, prPrefix)
		if !isAllDigits(prValue) {
			return VersionTypeInvalid, "", fmt.Errorf(versionErrorFormat, errUtils.ErrVersionFormatInvalid, version)
		}
		prNum, _ := strconv.Atoi(prValue)
		if prNum <= 0 {
			return VersionTypeInvalid, "", fmt.Errorf(versionErrorFormat, errUtils.ErrVersionFormatInvalid, version)
		}
		return VersionTypePR, prValue, nil
	}

	// 2. Explicit SHA prefix.
	if strings.HasPrefix(version, shaPrefix) {
		return VersionTypeSHA, strings.TrimPrefix(version, shaPrefix), nil
	}

	// 3. All digits -> PR number.
	if isAllDigits(version) {
		prNum, _ := strconv.Atoi(version)
		if prNum <= 0 {
			return VersionTypeInvalid, "", fmt.Errorf(versionErrorFormat, errUtils.ErrVersionFormatInvalid, version)
		}
		return VersionTypePR, version, nil
	}

	// 4. Valid semver pattern -> semver.
	if isValidSemver(version) {
		return VersionTypeSemver, version, nil
	}

	// 5. Auto-detect SHA (hex string 7-40 chars with at least one letter a-f).
	if isValidSHA(version) {
		return VersionTypeSHA, version, nil
	}

	// 6. Invalid format.
	return VersionTypeInvalid, "", fmt.Errorf(versionErrorFormat, errUtils.ErrVersionFormatInvalid, version)
}

// IsPRVersion checks if the version resolves to a PR.
// Returns the PR number and true if it's a PR version, otherwise 0 and false.
func IsPRVersion(version string) (int, bool) {
	defer perf.Track(nil, "toolchain.IsPRVersion")()

	vType, value, err := ParseVersionSpec(version)
	if err != nil || vType != VersionTypePR {
		return 0, false
	}

	prNum, err := strconv.Atoi(value)
	if err != nil || prNum <= 0 {
		return 0, false
	}

	return prNum, true
}

// IsSHAVersion checks if the version resolves to a SHA.
// Returns the SHA string and true if it's a SHA version, otherwise "" and false.
func IsSHAVersion(version string) (string, bool) {
	defer perf.Track(nil, "toolchain.IsSHAVersion")()

	vType, value, err := ParseVersionSpec(version)
	if err != nil || vType != VersionTypeSHA {
		return "", false
	}

	// Validate SHA format.
	if !isValidSHA(value) {
		return "", false
	}

	return value, true
}

// isAllDigits returns true if the string contains only digit characters.
func isAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isValidSemver checks if a string looks like a semantic version.
// Accepts patterns like: "1.2.3", "v1.2.3", "1.0.0", "v0.1.0".
// Also accepts "latest" as a special keyword.
func isValidSemver(s string) bool {
	// Special case: "latest" is a valid version keyword.
	if s == "latest" {
		return true
	}

	// Strip optional 'v' prefix.
	version := strings.TrimPrefix(s, "v")

	// Must contain at least one dot.
	if !strings.Contains(version, ".") {
		return false
	}

	// Split by dots and validate each part is numeric.
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}

	for _, part := range parts {
		if len(part) == 0 {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// isValidSHA checks if a string looks like a git commit SHA.
// A valid SHA is:
//   - 7-40 characters long (short or full SHA)
//   - Contains only lowercase hex characters (0-9, a-f)
//   - Contains at least one letter a-f (to distinguish from PR numbers)
func isValidSHA(s string) bool {
	// Check length bounds.
	if len(s) < minSHALength || len(s) > maxSHALength {
		return false
	}

	hasLetter := false
	for _, r := range s {
		if r >= 'a' && r <= 'f' {
			hasLetter = true
		} else if r < '0' || r > '9' {
			// Not a hex character.
			return false
		}
	}

	// Must have at least one letter to distinguish from PR numbers.
	return hasLetter
}
