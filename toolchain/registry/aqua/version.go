package aqua

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// GetLatestVersion fetches the latest non-prerelease, non-draft version from GitHub releases.
func (ar *AquaRegistry) GetLatestVersion(owner, repo string) (string, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.GetLatestVersion")()

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

// evaluateVersionConstraint evaluates an Aqua version constraint expression.
func evaluateVersionConstraint(constraint, version string) (bool, error) {
	defer perf.Track(nil, "aqua.evaluateVersionConstraint")()

	constraint = strings.TrimSpace(constraint)

	if constraint == "true" || constraint == `"true"` {
		return true, nil
	}
	if constraint == "false" || constraint == `"false"` {
		return false, nil
	}

	if strings.HasPrefix(constraint, "Version ==") {
		expectedVersion := strings.TrimSpace(strings.TrimPrefix(constraint, "Version =="))
		expectedVersion = strings.Trim(expectedVersion, `"`)
		return version == expectedVersion, nil
	}

	if strings.HasPrefix(constraint, "semver(") && strings.HasSuffix(constraint, ")") {
		return evaluateSemverConstraint(constraint, version)
	}

	return false, fmt.Errorf("%w: %q", errUtils.ErrUnsupportedVersionConstraint, constraint)
}

// evaluateSemverConstraint evaluates a semver constraint expression.
func evaluateSemverConstraint(constraint, version string) (bool, error) {
	semverConstraint := strings.TrimPrefix(constraint, "semver(")
	semverConstraint = strings.TrimSuffix(semverConstraint, ")")
	semverConstraint = strings.Trim(semverConstraint, `"`)

	v, err := semver.NewVersion(version)
	if err != nil {
		return false, fmt.Errorf("invalid version %q: %w", version, err)
	}

	c, err := semver.NewConstraint(semverConstraint)
	if err != nil {
		return false, fmt.Errorf("invalid semver constraint %q: %w", semverConstraint, err)
	}

	return c.Check(v), nil
}
