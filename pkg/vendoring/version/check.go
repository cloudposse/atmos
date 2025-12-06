package version

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CheckResult represents the result of checking for version updates.
type CheckResult struct {
	Component      string
	CurrentVersion string
	LatestVersion  string
	UpdateType     string // "major", "minor", "patch", "none"
	IsOutdated     bool
	GitURI         string
}

// GetGitRemoteTags fetches all tags from a remote Git repository using git ls-remote.
func GetGitRemoteTags(gitURI string) ([]string, error) {
	defer perf.Track(nil, "version.GetGitRemoteTags")()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "--refs", gitURI)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: GetGitRemoteTags %s: %w", errUtils.ErrGitLsRemoteFailed, gitURI, err)
	}

	// Parse output: each line is "commit_hash\trefs/tags/tag_name".
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	tags := make([]string, 0, len(lines))

	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			continue
		}

		// Extract tag name from "refs/tags/tag_name".
		tagRef := parts[1]
		if strings.HasPrefix(tagRef, "refs/tags/") {
			tagName := strings.TrimPrefix(tagRef, "refs/tags/")
			tags = append(tags, tagName)
		}
	}

	return tags, nil
}

// ParseSemVer attempts to parse a version string as a semantic version.
func ParseSemVer(version string) (*semver.Version, error) {
	defer perf.Track(nil, "version.ParseSemVer")()

	// Remove common prefixes.
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")

	return semver.NewVersion(version)
}

// FindLatestSemVerTag finds the latest semantic version from a list of tags.
func FindLatestSemVerTag(tags []string) (*semver.Version, string) {
	defer perf.Track(nil, "version.FindLatestSemVerTag")()

	var latestVer *semver.Version
	var latestTag string

	for _, tag := range tags {
		ver, err := ParseSemVer(tag)
		if err != nil {
			// Skip non-semantic version tags.
			continue
		}

		if latestVer == nil || ver.GreaterThan(latestVer) {
			latestVer = ver
			latestTag = tag
		}
	}

	return latestVer, latestTag
}

// CheckGitRef verifies that a Git reference (tag, branch, or commit) exists in a remote repository.
func CheckGitRef(gitURI string, ref string) (bool, error) {
	defer perf.Track(nil, "version.CheckGitRef")()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try as tag first.
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", gitURI, ref)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("%w: CheckGitRef %s %s (tag): %w", errUtils.ErrGitLsRemoteFailed, gitURI, ref, err)
	}
	if len(strings.TrimSpace(string(output))) > 0 {
		return true, nil
	}

	// Try as branch.
	cmd = exec.CommandContext(ctx, "git", "ls-remote", "--heads", gitURI, ref)
	output, err = cmd.Output()
	if err != nil {
		return false, fmt.Errorf("%w: CheckGitRef %s %s (branch): %w", errUtils.ErrGitLsRemoteFailed, gitURI, ref, err)
	}
	if len(strings.TrimSpace(string(output))) > 0 {
		return true, nil
	}

	// Try as commit SHA (this requires fetching, so we'll just validate format).
	if IsValidCommitSHA(ref) {
		// We assume it exists if it's a valid SHA format.
		// Full validation would require cloning/fetching.
		return true, nil
	}

	return false, nil
}

// IsValidCommitSHA checks if a string looks like a valid Git commit SHA.
func IsValidCommitSHA(ref string) bool {
	defer perf.Track(nil, "version.IsValidCommitSHA")()

	// Full SHA: 40 hex chars, short SHA: 7-40 hex chars.
	matched, _ := regexp.MatchString(`^[0-9a-f]{7,40}$`, ref)
	return matched
}

// ExtractGitURI extracts a clean Git URI from a vendor source string.
// It handles git:: prefixes, github.com/ shorthand, query parameters, and .git suffixes.
func ExtractGitURI(source string) string {
	defer perf.Track(nil, "version.ExtractGitURI")()

	// Handle git:: prefix.
	source = strings.TrimPrefix(source, "git::")

	// Handle github.com/ shorthand.
	if strings.HasPrefix(source, "github.com/") {
		source = "https://" + source
	}

	// Remove query parameters and fragments (like ?ref=xxx).
	if idx := strings.Index(source, "?"); idx != -1 {
		source = source[:idx]
	}

	// Clean up .git suffix if present.
	source = strings.TrimSuffix(source, ".git")

	return source
}
