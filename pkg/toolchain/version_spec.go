package toolchain

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/Masterminds/semver/v3"

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
	// VersionTypeRef represents a git ref — a branch or tag name resolved to its
	// latest commit's build artifact (e.g., "ref:main", "ref:v1.2.3", "ref:heads/main").
	VersionTypeRef
	// VersionTypeInvalid represents an invalid version format.
	VersionTypeInvalid
)

const (
	// Prefixes for explicit version specifiers.
	prPrefix  = "pr:"
	shaPrefix = "sha:"
	refPrefix = "ref:"

	// Minimum length for auto-detected SHAs (short SHA).
	minSHALength = 7
	// Maximum length for a full SHA.
	maxSHALength = 40

	// Format string for invalid version errors.
	versionErrorFormat = "%w: '%s'"
)

// ParseVersionSpec detects the version type from an input string.
// Supports explicit prefixes (pr:, sha:, ref:) and auto-detection.
//   - All digits -> PR number.
//   - Hex string 7-40 chars with at least one letter a-f -> SHA.
//   - Valid semver pattern (X.Y.Z or vX.Y.Z) -> semver.
//   - ref:<name> -> git ref (branch or tag), resolved at install time.
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
		return parsePRPrefix(version)
	}

	// 2. Explicit SHA prefix.
	if strings.HasPrefix(version, shaPrefix) {
		return parseSHAPrefix(version)
	}

	// 3. Explicit ref prefix (branch or tag).
	if strings.HasPrefix(version, refPrefix) {
		return parseRefPrefix(version)
	}

	// 4. All digits -> PR number.
	if isAllDigits(version) {
		prNum, _ := strconv.Atoi(version)
		if prNum <= 0 {
			return VersionTypeInvalid, "", fmt.Errorf(versionErrorFormat, errUtils.ErrVersionFormatInvalid, version)
		}
		return VersionTypePR, version, nil
	}

	// 5. Valid semver pattern -> semver.
	if isValidSemver(version) {
		return VersionTypeSemver, version, nil
	}

	// 6. Auto-detect SHA (hex string 7-40 chars with at least one letter a-f).
	if isValidSHA(version) {
		return VersionTypeSHA, version, nil
	}

	// 7. Invalid format.
	return VersionTypeInvalid, "", fmt.Errorf(versionErrorFormat, errUtils.ErrVersionFormatInvalid, version)
}

// parsePRPrefix handles explicit "pr:NNNN" version specifiers.
func parsePRPrefix(version string) (VersionType, string, error) {
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

// parseSHAPrefix handles explicit "sha:XXXXXXX" version specifiers.
func parseSHAPrefix(version string) (VersionType, string, error) {
	shaValue := strings.TrimPrefix(version, shaPrefix)
	if !isValidSHA(shaValue) {
		return VersionTypeInvalid, "", fmt.Errorf(versionErrorFormat, errUtils.ErrVersionFormatInvalid, version)
	}
	return VersionTypeSHA, shaValue, nil
}

// parseRefPrefix handles explicit "ref:<name>" version specifiers.
// The ref is a git branch or tag name (e.g. "main", "v1.2.3", "heads/main", "tags/v1.2.3")
// that is resolved to its latest commit SHA at install time. Validation is intentionally
// light because git refs permit a wide range of characters; only clearly-invalid input
// (empty, or containing whitespace/control characters) is rejected here.
func parseRefPrefix(version string) (VersionType, string, error) {
	refValue := strings.TrimPrefix(version, refPrefix)
	if !isValidRef(refValue) {
		return VersionTypeInvalid, "", fmt.Errorf(versionErrorFormat, errUtils.ErrVersionFormatInvalid, version)
	}
	return VersionTypeRef, refValue, nil
}

// versionRangeChars are characters that are either forbidden in git tag/ref
// names (per git-check-ref-format: space, ^, ~, :, ?, *, [, \) or are used
// exclusively as SemVer range/constraint-operator punctuation (>, <, =, comma,
// |). A real literal release tag — including vendor-specific formats like
// "jq-1.7.1" or "release-2024.01" — never legitimately contains any of these,
// so their presence reliably identifies range/constraint syntax rather than a
// tag atmos just doesn't happen to recognize the shape of.
const versionRangeChars = " \t^~:?*[\\><=,|"

// ValidateVersionSpec rejects version strings that can't possibly be a literal
// release tag — in particular SemVer range/constraint syntax (^1.2.0, ~>1.0.0,
// >=1.0.0, ">=1.0.0,<2.0.0") — before the value ever reaches a network call.
// Without this check that syntax was silently written to .tool-versions and
// only failed later with a raw, unhelpful HTTP 404 from installer.Install
// treating the range as a literal (nonexistent) release tag.
//
// This is deliberately permissive about what counts as a valid literal tag:
// .tool-versions/add/install accept whatever tag string a tool's registry
// actually uses (e.g. jq tags its releases "jq-1.7.1", not "1.7.1" or
// "v1.7.1"), so this does NOT require the strict semver shape ParseVersionSpec
// checks for — only the pr:/sha:/ref: explicit-prefix forms and "latest" get
// that stricter validation; everything else just needs to be free of
// characters that are either forbidden in git refs or are exclusively
// range/constraint-operator syntax.
func ValidateVersionSpec(version string) error {
	defer perf.Track(nil, "toolchain.ValidateVersionSpec")()

	if version == "" {
		return errUtils.Build(errUtils.ErrVersionFormatInvalid).
			WithExplanation("Version cannot be empty").
			WithHint("Supported formats: an exact version (e.g. `1.7.0`), `latest`, `pr:NNNN`, `sha:xxxxxxx`, or `ref:<name>`").
			WithExitCode(2).
			Err()
	}

	if version == "latest" || strings.HasPrefix(version, prPrefix) ||
		strings.HasPrefix(version, shaPrefix) || strings.HasPrefix(version, refPrefix) {
		if _, _, err := ParseVersionSpec(version); err != nil {
			return errUtils.Build(errUtils.ErrVersionFormatInvalid).
				WithExplanationf("`%s` is not a valid `%s` specifier", version, strings.SplitN(version, ":", 2)[0]).
				WithCause(err).
				WithExitCode(2).
				Err()
		}
		return nil
	}

	if strings.ContainsAny(version, versionRangeChars) {
		return errUtils.Build(errUtils.ErrVersionFormatInvalid).
			WithExplanationf("`%s` looks like a range/constraint expression, not a literal version", version).
			WithHint("`.tool-versions` is exact-version-only — supported formats: an exact version (e.g. `1.7.0` or a tool-specific tag like `jq-1.7.1`), `latest`, `pr:NNNN`, `sha:xxxxxxx`, or `ref:<name>`").
			WithHint("For range-based pinning (`^1.2.0`, `~>1.0.0`, `>=1.0.0`), use `dependencies.tools` in a stack manifest, or `atmos version track`").
			WithContext("version", version).
			WithExitCode(2).
			Err()
	}
	return nil
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

// IsRefVersion checks if the version resolves to a git ref (branch or tag).
// Returns the ref name and true if it's a ref version, otherwise "" and false.
// Only the explicit "ref:" prefix produces a ref — refs are never auto-detected.
func IsRefVersion(version string) (string, bool) {
	defer perf.Track(nil, "toolchain.IsRefVersion")()

	vType, value, err := ParseVersionSpec(version)
	if err != nil || vType != VersionTypeRef {
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
// Accepts patterns like: "1.2.3", "v1.2.3", "1.0.0", "v0.1.0", and
// pre-release/build-metadata suffixes like "1.2.3-rc.3", "1.2.3+build.5".
// Also accepts "latest" as a special keyword.
func isValidSemver(s string) bool {
	// Special case: "latest" is a valid version keyword.
	if s == "latest" {
		return true
	}

	// Require at least one dot so bare numbers (PR numbers) and bare "v1"
	// are rejected here and left to the other classifiers.
	if !strings.Contains(s, ".") {
		return false
	}

	// Delegate to the vendored semver library (already used elsewhere in this
	// package, e.g. get.go, list.go) instead of hand-rolling semver grammar:
	// a naive dot-split can't distinguish version-core dots from the dots
	// inside a pre-release identifier (e.g. "rc.3" in "1.2.3-rc.3").
	_, err := semver.NewVersion(s)
	return err == nil
}

// isValidRef checks if a string is a plausible git ref name (branch or tag).
// Git refs permit a wide range of characters, so this only rejects clearly-invalid
// input: empty/whitespace-only refs and refs containing any whitespace or control
// characters. The actual existence of the ref is validated remotely at resolve time.
func isValidRef(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// isValidSHA checks if a string looks like a git commit SHA.
// A valid SHA is:
//   - 7-40 characters long (short or full SHA).
//   - Contains only lowercase hex characters (0-9, a-f).
//   - Contains at least one letter a-f (to distinguish from PR numbers).
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
