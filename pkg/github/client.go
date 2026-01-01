package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Error definitions for the github package.
var (
	// ErrInvalidGitHubURL indicates the GitHub URL format is invalid.
	ErrInvalidGitHubURL = errors.New("invalid GitHub URL")

	// ErrUnsupportedGitHubHost indicates the GitHub host is not supported.
	ErrUnsupportedGitHubHost = errors.New("unsupported GitHub host")

	// ErrNoVersionsFound indicates no versions were found for a repository.
	ErrNoVersionsFound = errors.New("no versions found")
)

const (
	// GithubAPIMaxPerPage is the maximum number of items per page allowed by GitHub API.
	githubAPIMaxPerPage = 100
	// GithubAPIStatus422 is the HTTP status code returned when pagination exceeds limits.
	githubAPIStatus422 = 422
	// GithubAPIMinRateLimitThreshold is the minimum number of remaining requests before warning.
	githubAPIMinRateLimitThreshold = 5
	// DefaultHTTPTimeout is the timeout for HTTP requests to prevent hangs in CI environments.
	defaultHTTPTimeout = 30 * time.Second
	// Logging field name constants.
	logFieldOwner = "owner"
	logFieldRepo  = "repo"
)

// newGitHubClient creates a new GitHub client. If a token is provided, it returns an authenticated client;
// otherwise, it returns an unauthenticated client.
func newGitHubClient(ctx context.Context) *github.Client {
	defer perf.Track(nil, "github.newGitHubClient")()

	// Get GitHub token (bound to check ATMOS_GITHUB_TOKEN then GITHUB_TOKEN).
	githubToken := viper.GetString("ATMOS_GITHUB_TOKEN")

	// Create HTTP client with timeout to prevent hangs in CI environments
	// when network is unavailable or DNS resolution fails.
	httpClient := &http.Client{
		Timeout: defaultHTTPTimeout,
	}

	if githubToken == "" {
		return github.NewClient(httpClient)
	}

	// Token found, create an authenticated client with timeout.
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	// Create oauth2 client with our timeout-configured base transport.
	tc := oauth2.NewClient(ctx, ts)
	tc.Timeout = defaultHTTPTimeout

	return github.NewClient(tc)
}

// handleGitHubAPIError converts GitHub API errors to more descriptive error messages,
// especially for rate limiting.
func handleGitHubAPIError(err error, resp *github.Response) error {
	defer perf.Track(nil, "github.handleGitHubAPIError")()

	if resp != nil && resp.Rate.Remaining == 0 {
		resetTime := resp.Rate.Reset.Time
		waitDuration := time.Until(resetTime)

		return fmt.Errorf("%w: rate limit exceeded, resets at %s (in %s). Consider setting ATMOS_GITHUB_TOKEN or GITHUB_TOKEN for higher limits",
			errUtils.ErrGitHubRateLimitExceeded,
			resetTime.Format(time.RFC3339),
			waitDuration.Round(time.Second),
		)
	}

	return err
}

// ConvertToRawURL converts a GitHub repository URL to its raw content URL.
// Supports various GitHub URL formats and converts them to raw.githubusercontent.com URLs.
//
// Examples:
//   - https://github.com/owner/repo/blob/main/path/file.yaml
//     → https://raw.githubusercontent.com/owner/repo/main/path/file.yaml
//   - https://github.com/owner/repo/tree/v1.0.0/path
//     → https://raw.githubusercontent.com/owner/repo/v1.0.0/path
//   - github://owner/repo/path/file.yaml@branch
//     → https://raw.githubusercontent.com/owner/repo/branch/path/file.yaml
//   - github://owner/repo@v1.0.0
//     → https://raw.githubusercontent.com/owner/repo/v1.0.0
func ConvertToRawURL(githubURL string) (string, error) {
	defer perf.Track(nil, "github.ConvertToRawURL")()

	// Handle github:// scheme.
	if strings.HasPrefix(githubURL, "github://") {
		return convertGitHubSchemeToRaw(githubURL)
	}

	// Parse the URL.
	u, err := url.Parse(githubURL)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidGitHubURL, err)
	}

	// Already a raw URL.
	if u.Host == "raw.githubusercontent.com" {
		return githubURL, nil
	}

	// Must be github.com.
	if u.Host != "github.com" {
		return "", fmt.Errorf("%w: %s (expected github.com)", ErrUnsupportedGitHubHost, u.Host)
	}

	return parseGitHubDotComURL(u.Path)
}

// parseGitHubDotComURL parses a github.com URL path and converts it to raw URL.
func parseGitHubDotComURL(path string) (string, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("%w: path %s (expected at least owner/repo)", ErrInvalidGitHubURL, path)
	}

	owner := parts[0]
	repo := parts[1]

	// Default to main branch if no additional parts.
	if len(parts) == 2 {
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main", owner, repo), nil
	}

	return parseGitHubPathWithRef(owner, repo, parts[2:], path)
}

// parseGitHubPathWithRef parses a GitHub path with blob/tree and ref components.
func parseGitHubPathWithRef(owner, repo string, pathParts []string, originalPath string) (string, error) {
	if len(pathParts) < 2 {
		return "", fmt.Errorf("%w: path %s (expected owner/repo/blob|tree/ref)", ErrInvalidGitHubURL, originalPath)
	}

	urlType := pathParts[0] // blob or tree
	if urlType != "blob" && urlType != "tree" {
		return "", fmt.Errorf("%w: type %s (expected blob or tree)", ErrInvalidGitHubURL, urlType)
	}

	ref := pathParts[1]
	fileParts := pathParts[2:]

	return buildRawURL(owner, repo, ref, fileParts), nil
}

// buildRawURL constructs a raw.githubusercontent.com URL.
func buildRawURL(owner, repo, ref string, pathParts []string) string {
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", owner, repo, ref)
	if len(pathParts) > 0 {
		rawURL = fmt.Sprintf("%s/%s", rawURL, strings.Join(pathParts, "/"))
	}
	return rawURL
}

// convertGitHubSchemeToRaw converts github:// scheme URLs to raw content URLs.
// Format: github://owner/repo/path/to/file@ref
func convertGitHubSchemeToRaw(githubURL string) (string, error) {
	// Remove github:// prefix
	remainder := strings.TrimPrefix(githubURL, "github://")

	// Split on @ to separate path from ref
	var pathPart, ref string
	if strings.Contains(remainder, "@") {
		parts := strings.SplitN(remainder, "@", 2)
		pathPart = parts[0]
		ref = parts[1]
	} else {
		pathPart = remainder
		ref = "main" // default to main
	}

	// Parse owner/repo/path
	pathComponents := strings.Split(strings.Trim(pathPart, "/"), "/")
	if len(pathComponents) < 2 {
		return "", fmt.Errorf("%w: %s (expected at least owner/repo)", ErrInvalidGitHubURL, githubURL)
	}

	owner := pathComponents[0]
	repo := pathComponents[1]
	filePath := ""
	if len(pathComponents) > 2 {
		filePath = "/" + strings.Join(pathComponents[2:], "/")
	}

	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s%s", owner, repo, ref, filePath), nil
}
