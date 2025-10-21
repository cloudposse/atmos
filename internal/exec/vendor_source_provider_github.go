package exec

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GitHubSourceProvider implements VendorSourceProvider for GitHub repositories.
type GitHubSourceProvider struct {
	httpClient *http.Client
}

// NewGitHubSourceProvider creates a new GitHub source provider.
func NewGitHubSourceProvider() VendorSourceProvider {
	return &GitHubSourceProvider{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetAvailableVersions implements VendorSourceProvider.GetAvailableVersions.
func (g *GitHubSourceProvider) GetAvailableVersions(source string) ([]string, error) {
	// Use existing Git operations to get tags
	gitURI := extractGitURI(source)
	return getGitRemoteTags(gitURI)
}

// VerifyVersion implements VendorSourceProvider.VerifyVersion.
func (g *GitHubSourceProvider) VerifyVersion(source string, version string) (bool, error) {
	gitURI := extractGitURI(source)
	return checkGitRef(gitURI, version)
}

// GetDiff implements VendorSourceProvider.GetDiff using GitHub's Compare API.
//
//nolint:revive // Seven parameters needed for comprehensive diff configuration.
func (g *GitHubSourceProvider) GetDiff(
	atmosConfig *schema.AtmosConfiguration,
	source string,
	fromVersion string,
	toVersion string,
	filePath string,
	contextLines int,
	noColor bool,
) ([]byte, error) {
	defer perf.Track(atmosConfig, "exec.GitHubSourceProvider.GetDiff")()

	// Parse GitHub owner/repo from source
	owner, repo, err := parseGitHubRepo(source)
	if err != nil {
		return nil, err
	}

	// Use GitHub Compare API
	// https://docs.github.com/en/rest/commits/commits#compare-two-commits
	compareURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/compare/%s...%s",
		owner, repo, fromVersion, toVersion)

	req, err := http.NewRequest("GET", compareURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %s", errUtils.ErrGitDiffFailed, err)
	}

	// Add GitHub API headers
	req.Header.Set("Accept", "application/vnd.github.v3.diff")

	// Add authentication if available (from environment)
	if token := getGitHubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch diff from GitHub: %s", errUtils.ErrGitDiffFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: GitHub API returned status %d: %s",
			errUtils.ErrGitDiffFailed, resp.StatusCode, string(body))
	}

	diff, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read diff: %s", errUtils.ErrGitDiffFailed, err)
	}

	// TODO: Apply file filtering if filePath is specified
	// TODO: Apply context line configuration if contextLines is specified
	// TODO: Strip ANSI codes if noColor is true

	return diff, nil
}

// SupportsOperation implements VendorSourceProvider.SupportsOperation.
func (g *GitHubSourceProvider) SupportsOperation(operation SourceOperation) bool {
	switch operation {
	case OperationListVersions, OperationVerifyVersion, OperationGetDiff, OperationFetchSource:
		return true
	default:
		return false
	}
}

// parseGitHubRepo extracts owner and repository name from a GitHub source URL.
func parseGitHubRepo(source string) (owner, repo string, err error) {
	// Remove common prefixes
	source = strings.TrimPrefix(source, "git::")
	source = strings.TrimPrefix(source, "https://")
	source = strings.TrimPrefix(source, "http://")
	source = strings.TrimPrefix(source, "github.com/")

	// Handle SSH format
	if strings.HasPrefix(source, "git@github.com:") {
		source = strings.TrimPrefix(source, "git@github.com:")
	}

	// Remove .git suffix
	source = strings.TrimSuffix(source, ".git")

	// Remove query parameters
	if idx := strings.Index(source, "?"); idx != -1 {
		source = source[:idx]
	}

	// Remove path after repo (e.g., //modules/vpc)
	if idx := strings.Index(source, "//"); idx != -1 {
		source = source[:idx]
	}

	// Split into owner/repo
	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("%w: invalid GitHub repository format: %s", errUtils.ErrParseURL, source)
	}

	return parts[0], parts[1], nil
}

// isGitHubSource checks if a source URL is a GitHub repository.
func isGitHubSource(source string) bool {
	return strings.Contains(source, "github.com")
}

// getGitHubToken retrieves the GitHub token from Atmos settings or environment.
func getGitHubToken() string {
	// This would integrate with Atmos configuration system
	// For now, return empty string - the actual implementation would check:
	// 1. ATMOS_GITHUB_TOKEN
	// 2. GITHUB_TOKEN
	// 3. atmosConfig.Settings.AtmosGithubToken
	return ""
}

// GitHubRateLimitResponse represents the GitHub API rate limit response.
type GitHubRateLimitResponse struct {
	Resources struct {
		Core struct {
			Limit     int   `json:"limit"`
			Remaining int   `json:"remaining"`
			Reset     int64 `json:"reset"`
		} `json:"core"`
	} `json:"resources"`
}

// CheckGitHubRateLimit checks the current GitHub API rate limit.
func (g *GitHubSourceProvider) CheckGitHubRateLimit() (*GitHubRateLimitResponse, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/rate_limit", nil)
	if err != nil {
		return nil, err
	}

	if token := getGitHubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rateLimit GitHubRateLimitResponse
	if err := json.NewDecoder(resp.Body).Decode(&rateLimit); err != nil {
		return nil, err
	}

	return &rateLimit, nil
}
