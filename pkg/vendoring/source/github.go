package source

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

const (
	// DefaultHTTPTimeout is the default timeout for HTTP requests to GitHub API.
	defaultHTTPTimeout = 30 * time.Second
)

// GitHubProvider implements Provider for GitHub repositories.
type GitHubProvider struct {
	httpClient *http.Client
}

// NewGitHubProvider creates a new GitHub source provider.
func NewGitHubProvider() Provider {
	defer perf.Track(nil, "source.NewGitHubProvider")()

	return &GitHubProvider{
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}
}

// GetAvailableVersions implements Provider.GetAvailableVersions.
func (g *GitHubProvider) GetAvailableVersions(source string) ([]string, error) {
	defer perf.Track(nil, "source.GitHubProvider.GetAvailableVersions")()

	// Use existing Git operations to get tags.
	gitURI := version.ExtractGitURI(source)
	return version.GetGitRemoteTags(gitURI)
}

// VerifyVersion implements Provider.VerifyVersion.
func (g *GitHubProvider) VerifyVersion(source string, ver string) (bool, error) {
	defer perf.Track(nil, "source.GitHubProvider.VerifyVersion")()

	gitURI := version.ExtractGitURI(source)
	return version.CheckGitRef(gitURI, ver)
}

// GetDiff implements Provider.GetDiff using GitHub's Compare API.
//
//nolint:revive // Seven parameters needed for comprehensive diff configuration.
func (g *GitHubProvider) GetDiff(
	atmosConfig *schema.AtmosConfiguration,
	source string,
	fromVersion string,
	toVersion string,
	filePath string,
	contextLines int,
	noColor bool,
) ([]byte, error) {
	defer perf.Track(atmosConfig, "source.GitHubProvider.GetDiff")()

	// Parse GitHub owner/repo from source.
	owner, repo, err := ParseGitHubRepo(source)
	if err != nil {
		return nil, err
	}

	// Use GitHub Compare API.
	// https://docs.github.com/en/rest/commits/commits#compare-two-commits
	// Path-escape ref names since they may contain slashes.
	compareURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/compare/%s...%s",
		owner, repo, url.PathEscape(fromVersion), url.PathEscape(toVersion))

	req, err := http.NewRequest("GET", compareURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %s", errUtils.ErrGitDiffFailed, err)
	}

	// Add GitHub API headers.
	req.Header.Set("Accept", "application/vnd.github.v3.diff")

	// Add authentication if available (from environment).
	if token := GetGitHubToken(); token != "" {
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

	// TODO: Apply file filtering if filePath is specified.
	// TODO: Apply context line configuration if contextLines is specified.
	// TODO: Strip ANSI codes if noColor is true.

	return diff, nil
}

// SupportsOperation implements Provider.SupportsOperation.
func (g *GitHubProvider) SupportsOperation(operation Operation) bool {
	defer perf.Track(nil, "source.GitHubProvider.SupportsOperation")()
	switch operation {
	case OperationListVersions, OperationVerifyVersion, OperationGetDiff, OperationFetchSource:
		return true
	default:
		return false
	}
}

// ParseGitHubRepo extracts owner and repository name from a GitHub source URL.
func ParseGitHubRepo(source string) (owner, repo string, err error) {
	defer perf.Track(nil, "source.ParseGitHubRepo")()

	// Remove common prefixes.
	source = strings.TrimPrefix(source, "git::")
	source = strings.TrimPrefix(source, "https://")
	source = strings.TrimPrefix(source, "http://")
	source = strings.TrimPrefix(source, "github.com/")

	// Handle SSH format.
	source = strings.TrimPrefix(source, "git@github.com:")

	// Remove .git suffix.
	source = strings.TrimSuffix(source, ".git")

	// Remove query parameters.
	if idx := strings.Index(source, "?"); idx != -1 {
		source = source[:idx]
	}

	// Remove path after repo (e.g., //modules/vpc).
	if idx := strings.Index(source, "//"); idx != -1 {
		source = source[:idx]
	}

	// Split into owner/repo.
	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("%w: invalid GitHub repository format: %s", errUtils.ErrParseURL, source)
	}

	return parts[0], parts[1], nil
}

// IsGitHubSource checks if a source URL is a GitHub repository.
func IsGitHubSource(source string) bool {
	defer perf.Track(nil, "source.IsGitHubSource")()

	return strings.Contains(source, "github.com")
}

// GetGitHubToken retrieves the GitHub token from environment.
// Checks ATMOS_GITHUB_TOKEN first (per ATMOS_ prefix convention), then GITHUB_TOKEN.
func GetGitHubToken() string {
	defer perf.Track(nil, "source.GetGitHubToken")()

	// Use viper to check environment variables per project conventions.
	// BindEnv maps the key to environment variables.
	v := viper.New()
	_ = v.BindEnv("github_token", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN")
	return v.GetString("github_token")
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
func (g *GitHubProvider) CheckGitHubRateLimit() (*GitHubRateLimitResponse, error) {
	defer perf.Track(nil, "source.CheckGitHubRateLimit")()

	req, err := http.NewRequest("GET", "https://api.github.com/rate_limit", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create rate limit request: %w", errUtils.ErrFailedToCreateRequest, err)
	}

	if token := GetGitHubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to execute rate limit request: %w", errUtils.ErrHTTPRequestFailed, err)
	}
	defer resp.Body.Close()

	var rateLimit GitHubRateLimitResponse
	if err := json.NewDecoder(resp.Body).Decode(&rateLimit); err != nil {
		return nil, fmt.Errorf("%w: failed to decode rate limit response: %w", errUtils.ErrFailedToUnmarshalAPIResponse, err)
	}

	return &rateLimit, nil
}
