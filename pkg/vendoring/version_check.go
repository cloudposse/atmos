package vendoring

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

// VersionCheckResult represents the result of checking for version updates.
type VersionCheckResult struct {
	Component      string
	CurrentVersion string
	LatestVersion  string
	UpdateType     string // "major", "minor", "patch", "none"
	IsOutdated     bool
	GitURI         string
}

// getGitRemoteTags fetches all tags from a remote Git repository using git ls-remote.
func getGitRemoteTags(gitURI string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "--refs", gitURI)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: getGitRemoteTags %s: %s", errUtils.ErrGitLsRemoteFailed, gitURI, err)
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

// parseSemVer attempts to parse a version string as a semantic version.
func parseSemVer(version string) (*semver.Version, error) {
	// Remove common prefixes
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")

	return semver.NewVersion(version)
}

// findLatestSemVerTag finds the latest semantic version from a list of tags.
func findLatestSemVerTag(tags []string) (*semver.Version, string) {
	var latestVer *semver.Version
	var latestTag string

	for _, tag := range tags {
		ver, err := parseSemVer(tag)
		if err != nil {
			// Skip non-semantic version tags
			continue
		}

		if latestVer == nil || ver.GreaterThan(latestVer) {
			latestVer = ver
			latestTag = tag
		}
	}

	return latestVer, latestTag
}

// checkGitRef verifies that a Git reference (tag, branch, or commit) exists in a remote repository.
func checkGitRef(gitURI string, ref string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try as tag first.
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", gitURI, ref)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("%w: checkGitRef %s %s (tag): %s", errUtils.ErrGitLsRemoteFailed, gitURI, ref, err)
	}
	if len(strings.TrimSpace(string(output))) > 0 {
		return true, nil
	}

	// Try as branch.
	cmd = exec.CommandContext(ctx, "git", "ls-remote", "--heads", gitURI, ref)
	output, err = cmd.Output()
	if err != nil {
		return false, fmt.Errorf("%w: checkGitRef %s %s (branch): %s", errUtils.ErrGitLsRemoteFailed, gitURI, ref, err)
	}
	if len(strings.TrimSpace(string(output))) > 0 {
		return true, nil
	}

	// Try as commit SHA (this requires fetching, so we'll just validate format).
	if isValidCommitSHA(ref) {
		// We assume it exists if it's a valid SHA format.
		// Full validation would require cloning/fetching.
		return true, nil
	}

	return false, nil
}

// isValidCommitSHA checks if a string looks like a valid Git commit SHA.
func isValidCommitSHA(ref string) bool {
	// Full SHA: 40 hex chars, short SHA: 7-40 hex chars
	matched, _ := regexp.MatchString(`^[0-9a-f]{7,40}$`, ref)
	return matched
}
