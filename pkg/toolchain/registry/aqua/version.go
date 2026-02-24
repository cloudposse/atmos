package aqua

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"runtime"
	"strings"

	"github.com/expr-lang/expr"
	goversion "github.com/hashicorp/go-version"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

// GetLatestVersion fetches the latest non-prerelease, non-draft version.
// Checks the tool's version_source setting: "github_tag" uses the tags API,
// otherwise uses the releases API (default).
func (ar *AquaRegistry) GetLatestVersion(owner, repo string) (string, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.GetLatestVersion")()

	// Check if the tool uses github_tag version source.
	tool, err := ar.GetTool(owner, repo)
	if err == nil && tool.VersionSource == "github_tag" {
		return ar.getLatestTag(owner, repo)
	}

	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=%d", ar.githubBaseURL, owner, repo, githubPerPage)

	for apiURL != "" {
		version, nextURL, err := ar.fetchVersionFromPage(apiURL)
		if err != nil {
			return "", err
		}
		if version != "" {
			return version, nil
		}
		apiURL = nextURL
	}

	return "", fmt.Errorf("%w: no non-prerelease, non-draft versions found for %s/%s", registry.ErrNoVersionsFound, owner, repo)
}

// getLatestTag fetches the latest tag from GitHub tags API.
func (ar *AquaRegistry) getLatestTag(owner, repo string) (string, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/tags?per_page=1", ar.githubBaseURL, owner, repo)

	resp, err := ar.get(apiURL)
	if err != nil {
		return "", fmt.Errorf("%w: failed to fetch tags from GitHub: %w", registry.ErrHTTPRequest, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return "", fmt.Errorf("%w: GitHub API returned status %d", registry.ErrHTTPRequest, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("%w: failed to read response body: %w", registry.ErrHTTPRequest, err)
	}

	var tags []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &tags); err != nil {
		return "", fmt.Errorf("%w: failed to parse tags JSON: %w", registry.ErrRegistryParse, err)
	}

	if len(tags) == 0 {
		return "", fmt.Errorf("%w: no tags found for %s/%s", registry.ErrNoVersionsFound, owner, repo)
	}

	return strings.TrimPrefix(tags[0].Name, versionPrefix), nil
}

// fetchVersionFromPage fetches releases from a single page and returns the first valid version.
func (ar *AquaRegistry) fetchVersionFromPage(apiURL string) (version, nextURL string, err error) {
	resp, err := ar.get(apiURL)
	if err != nil {
		return "", "", fmt.Errorf("%w: failed to fetch releases from GitHub: %w", registry.ErrHTTPRequest, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return "", "", fmt.Errorf("%w: GitHub API returned status %d", registry.ErrHTTPRequest, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	linkHeader := resp.Header.Get("Link")
	resp.Body.Close()
	if err != nil {
		return "", "", fmt.Errorf("%w: failed to read response body: %w", registry.ErrHTTPRequest, err)
	}

	releases, err := parseReleasesJSON(body)
	if err != nil {
		return "", "", err
	}

	for _, release := range releases {
		if !release.Prerelease && !release.Draft {
			return strings.TrimPrefix(release.TagName, versionPrefix), "", nil
		}
	}

	return "", parseNextLink(linkHeader), nil
}

// releaseInfo represents a GitHub release.
type releaseInfo struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

// parseReleasesJSON parses the GitHub releases JSON response.
func parseReleasesJSON(body []byte) ([]releaseInfo, error) {
	var releases []releaseInfo
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("%w: failed to parse releases JSON: %w", registry.ErrRegistryParse, err)
	}
	return releases, nil
}

// GetAvailableVersions fetches all available non-prerelease, non-draft versions from GitHub releases.
func (ar *AquaRegistry) GetAvailableVersions(owner, repo string) ([]string, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.GetAvailableVersions")()

	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=%d", ar.githubBaseURL, owner, repo, githubPerPage)

	var versions []string
	for apiURL != "" {
		pageVersions, nextURL, err := ar.fetchVersionsFromPage(apiURL)
		if err != nil {
			return nil, err
		}
		versions = append(versions, pageVersions...)
		apiURL = nextURL
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("%w: no non-prerelease, non-draft versions found for %s/%s", registry.ErrNoVersionsFound, owner, repo)
	}

	return versions, nil
}

// fetchVersionsFromPage fetches all versions from a single page.
func (ar *AquaRegistry) fetchVersionsFromPage(apiURL string) (versions []string, nextURL string, err error) {
	resp, err := ar.get(apiURL)
	if err != nil {
		return nil, "", fmt.Errorf("%w: failed to fetch releases from GitHub: %w", registry.ErrHTTPRequest, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("%w: GitHub API returned status %d", registry.ErrHTTPRequest, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	linkHeader := resp.Header.Get("Link")
	resp.Body.Close()
	if err != nil {
		return nil, "", fmt.Errorf("%w: failed to read response body: %w", registry.ErrHTTPRequest, err)
	}

	releases, err := parseReleasesJSON(body)
	if err != nil {
		return nil, "", err
	}

	for _, release := range releases {
		if !release.Prerelease && !release.Draft {
			versions = append(versions, strings.TrimPrefix(release.TagName, versionPrefix))
		}
	}

	return versions, parseNextLink(linkHeader), nil
}

// getOS returns the current operating system.
func getOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

// getArch returns the current architecture.
func getArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "386":
		return "386"
	default:
		return "amd64"
	}
}

// parseNextLink extracts the next page URL from GitHub API Link header.
func parseNextLink(linkHeader string) string {
	links := strings.Split(linkHeader, ",")
	for _, link := range links {
		if strings.Contains(link, `rel="next"`) {
			start := strings.Index(link, "<")
			end := strings.Index(link, ">")
			if start >= 0 && end > start {
				return link[start+1 : end]
			}
		}
	}
	return ""
}

// commitHashPattern matches 40-character hex strings (git commit hashes).
var commitHashPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// isCommitHash returns true if the version string is a 40-char hex commit hash.
func isCommitHash(version string) bool {
	return commitHashPattern.MatchString(version)
}

// evaluateVersionConstraint evaluates an Aqua version constraint expression using expr-lang/expr.
// This matches Aqua's upstream behavior which uses the expr-lang expression engine,
// supporting semver(), semverWithVersion(), trimPrefix(), Version ==, Version !=,
// compound booleans, startsWith, contains, and other expr-lang built-in operators.
//
// Parameters:
//   - constraint: the version constraint expression (e.g., `semver(">= 1.0.0")`, `Version == "v1.0"`)
//   - version: the full version string including any prefix (e.g., "jq-1.7.1", "v1.0.0")
//   - sv: the semver-stripped version without prefix (e.g., "1.7.1", "1.0.0")
//
// Upstream passes these as separate values: Version (full) and SemVer (prefix-stripped).
// The semver() function uses the stripped version for constraint evaluation.
func evaluateVersionConstraint(constraint, version, sv string) (bool, error) {
	defer perf.Track(nil, "aqua.evaluateVersionConstraint")()

	constraint = strings.TrimSpace(constraint)

	// Handle literal true/false for backward compatibility.
	if constraint == "true" || constraint == `"true"` {
		return true, nil
	}
	if constraint == "false" || constraint == `"false"` {
		return false, nil
	}

	// Empty or whitespace-only constraints evaluate to false.
	if constraint == "" {
		return false, nil
	}

	// Build expr environment and evaluate.
	return evalConstraintExpr(constraint, version, sv)
}

// evalConstraintExpr builds the expr-lang environment and evaluates a version constraint expression.
// Version = full version with prefix, sv = prefix-stripped version for semver evaluation.
func evalConstraintExpr(constraint, version, sv string) (bool, error) {
	// Build semver evaluation function using the stripped version.
	// Upstream's semver() closure captures the stripped SemVer, not the full Version.
	semverFn := func(constraintStr string) bool {
		return compareSemver(constraintStr, sv)
	}

	// Create the expr environment with variables and functions.
	// trimPrefix uses Aqua's argument order: trimPrefix(prefix, s) not Go's TrimPrefix(s, prefix).
	env := map[string]interface{}{
		"Version":           version,
		"SemVer":            sv,
		"semver":            semverFn,
		"semverWithVersion": compareSemver,
		"trimPrefix":        func(prefix, s string) string { return strings.TrimPrefix(s, prefix) },
	}

	// Compile and evaluate the expression.
	program, err := expr.Compile(constraint, expr.Env(env), expr.AsBool())
	if err != nil {
		return false, fmt.Errorf("%w: %q: %w", errUtils.ErrUnsupportedVersionConstraint, constraint, err)
	}

	result, err := expr.Run(program, env)
	if err != nil {
		return false, fmt.Errorf("%w: %q: %w", errUtils.ErrUnsupportedVersionConstraint, constraint, err)
	}

	boolResult, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("%w: expression %q did not return a boolean", errUtils.ErrUnsupportedVersionConstraint, constraint)
	}

	return boolResult, nil
}

// compareSemver evaluates a semver constraint string against a version.
// This matches upstream aquaproj/aqua's compare() function which uses
// hashicorp/go-version for parsing and manual operator matching.
// Returns false for commit hashes, unparseable versions, or invalid constraints.
func compareSemver(constraintStr, ver string) bool {
	// If the version is a commit hash, semver evaluation is not applicable.
	if isCommitHash(ver) {
		return false
	}

	sv1, err := goversion.NewVersion(ver)
	if err != nil {
		return false
	}

	// Handle comma-separated constraints (AND logic): all must match.
	for _, part := range strings.Split(strings.TrimSpace(constraintStr), ",") {
		c := strings.TrimSpace(part)
		if !evaluateOp(sv1, c) {
			return false
		}
	}

	return true
}

// evaluateOp evaluates a single version constraint operator against a parsed version.
// Supported operators: >=, <=, !=, >, <, = (matching upstream aquaproj/aqua behavior).
func evaluateOp(sv1 *goversion.Version, constraint string) bool {
	type opEntry struct {
		prefix string
		fn     func(*goversion.Version) bool
	}

	ops := []opEntry{
		{">=", sv1.GreaterThanOrEqual},
		{"<=", sv1.LessThanOrEqual},
		{"!=", func(v *goversion.Version) bool { return !sv1.Equal(v) }},
		{">", sv1.GreaterThan},
		{"<", sv1.LessThan},
		{"=", sv1.Equal},
	}

	for _, op := range ops {
		if s := strings.TrimPrefix(constraint, op.prefix); s != constraint {
			sv2, err := goversion.NewVersion(strings.TrimSpace(s))
			if err != nil {
				return false
			}
			return op.fn(sv2)
		}
	}

	return false // Unknown operator.
}
