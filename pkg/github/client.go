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
	"golang.org/x/oauth2"

	errUtils "github.com/cloudposse/atmos/errors"
	httpClient "github.com/cloudposse/atmos/pkg/http"
	log "github.com/cloudposse/atmos/pkg/logger"
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

	// Get GitHub token using the full resolution chain:
	// 1. --github-token CLI flag
	// 2. ATMOS_GITHUB_TOKEN environment variable
	// 3. GITHUB_TOKEN environment variable
	// 4. `gh auth token` CLI fallback
	githubToken := GetGitHubToken()

	return newGitHubClientWithToken(ctx, githubToken)
}

// newGitHubClientWithToken creates a new GitHub client with an explicit token, scoped to
// RepoEndpoints (the user's own repositories: GITHUB_SERVER_URL/GITHUB_API_URL). If token is
// empty, it returns an unauthenticated client. When RepoEndpoints resolves to a GitHub
// Enterprise Server host, the client is pointed at that instance via go-github's
// WithEnterpriseURLs instead of public github.com.
func newGitHubClientWithToken(ctx context.Context, token string) *github.Client {
	defer perf.Track(nil, "github.newGitHubClientWithToken")()

	return newGitHubClientForEndpoints(ctx, token, RepoEndpoints())
}

// newToolchainGitHubClient creates a new GitHub client scoped to ToolchainEndpoints instead of
// RepoEndpoints. Toolchain-managed repositories (e.g. atmos's own PR/SHA/ref build artifacts
// used for self-install, or an arbitrary tool's release versions) live on public github.com by
// default even for GHES users, which is a separate concern from where the user's own
// repositories live -- see ToolchainEndpoints' doc comment.
func newToolchainGitHubClient(ctx context.Context) *github.Client {
	defer perf.Track(nil, "github.newToolchainGitHubClient")()

	return newGitHubClientForEndpoints(ctx, GetGitHubToken(), ToolchainEndpoints())
}

// newGitHubClientForEndpoints builds an authenticated (or, if token is empty, unauthenticated)
// *github.Client with an HTTP timeout, pointed at endpoints.
func newGitHubClientForEndpoints(ctx context.Context, token string, endpoints Endpoints) *github.Client {
	defer perf.Track(nil, "github.newGitHubClientForEndpoints")()

	// Create HTTP client with timeout to prevent hangs in CI environments
	// when network is unavailable or DNS resolution fails.
	baseClient := &http.Client{
		Timeout: defaultHTTPTimeout,
	}

	var httpClient *http.Client
	if token == "" {
		httpClient = baseClient
	} else {
		// Token found, create an authenticated client with timeout.
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		// Create oauth2 client with our timeout-configured base transport.
		tc := oauth2.NewClient(ctx, ts)
		tc.Timeout = defaultHTTPTimeout
		httpClient = tc
	}

	return newScopedClient(httpClient, endpoints)
}

// newScopedClient builds a *github.Client from httpClient, pointed at GitHub.com by default or
// at a GitHub Enterprise Server instance when endpoints resolves one. An error building the
// enterprise client (malformed URL) falls back to the public client rather than failing the
// caller outright, matching Endpoints' own fail-open behavior for invalid endpoint URLs.
func newScopedClient(httpClient *http.Client, endpoints Endpoints) *github.Client {
	if endpoints.Host == defaultGitHubServerHost {
		return github.NewClient(httpClient)
	}

	client, err := github.NewClient(httpClient).WithEnterpriseURLs(endpoints.APIURL, endpoints.UploadURL)
	if err != nil {
		log.Debug("Failed to build GitHub Enterprise Server client; falling back to github.com client",
			"apiURL", endpoints.APIURL, "uploadURL", endpoints.UploadURL, "error", err)
		return github.NewClient(httpClient)
	}
	return client
}

// handleGitHubAPIError converts GitHub API errors to more descriptive error messages,
// especially for rate limiting.
func handleGitHubAPIError(err error, resp *github.Response) error {
	defer perf.Track(nil, "github.handleGitHubAPIError")()

	// A 401 means the credentials were rejected (bad or expired token), not a rate limit.
	// GitHub returns X-RateLimit-Remaining: 0 on 401 responses, so this must be checked
	// before the rate-limit heuristic below to avoid misreporting bad credentials.
	if resp != nil && resp.StatusCode == http.StatusUnauthorized {
		return errUtils.Build(errUtils.ErrAuthenticationFailed).
			WithCause(err).
			WithExplanation("GitHub rejected the provided credentials (bad or expired token)").
			WithHint("Verify your token: `gh auth status`").
			WithHint("Try re-authenticating: `gh auth login`").
			WithHint("Or set a valid `ATMOS_GITHUB_TOKEN` or `GITHUB_TOKEN`").
			WithExitCode(1).
			Err()
	}

	// Genuine rate limiting is signalled with HTTP 403 (primary) or 429 (secondary),
	// always alongside X-RateLimit-Remaining: 0. Requiring the status code prevents
	// other zero-remaining responses (e.g. 401) from being misclassified.
	if resp != nil && resp.Rate.Remaining == 0 &&
		(resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) {
		resetTime := resp.Rate.Reset.Time
		waitDuration := time.Until(resetTime)

		builder := errUtils.Build(errUtils.ErrGitHubRateLimitExceeded).
			WithCause(err).
			WithExplanation(fmt.Sprintf("Rate limit exceeded, resets at %s (in %s)",
				resetTime.Format(time.RFC3339),
				waitDuration.Round(time.Second)))

		if httpClient.GetGitHubTokenFromEnv() != "" {
			builder.
				WithHint("Your GitHub token may be invalid or expired").
				WithHint("Verify your token: `gh auth status`").
				WithHint("Try re-authenticating: `gh auth login`")
		} else {
			builder.
				WithHint("Authenticate with GitHub CLI: `gh auth login`").
				WithHint("Or set `ATMOS_GITHUB_TOKEN` or `GITHUB_TOKEN` environment variable")
		}

		return builder.Err()
	}

	return err
}

// ConvertToRawURL converts a GitHub repository URL to its raw content URL.
// Supports various GitHub URL formats and converts them to raw content URLs: on
// github.com that is raw.githubusercontent.com; on a GitHub Enterprise Server host
// configured via RepoEndpoints (GITHUB_SERVER_URL/GITHUB_API_URL), raw content is served
// from the same host under /raw/ instead.
//
// Examples (github.com):
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

	// Already a raw URL (github.com's raw.githubusercontent.com, or a GHES host's own /raw/ path).
	endpoints := RepoEndpoints()
	if u.Host == "raw.githubusercontent.com" || (endpoints.Host != defaultGitHubServerHost && endpoints.IsHost(u.Host) && strings.HasPrefix(u.Path, "/raw/")) {
		return githubURL, nil
	}

	// Must be github.com or the configured GHES host.
	if u.Host != "github.com" && !endpoints.IsHost(u.Host) {
		return "", fmt.Errorf("%w: %s (expected github.com or the configured GitHub Enterprise Server host)", ErrUnsupportedGitHubHost, u.Host)
	}

	return parseGitHubDotComURL(endpoints, u.Path)
}

// parseGitHubDotComURL parses a github.com (or GHES) URL path and converts it to a raw URL.
func parseGitHubDotComURL(endpoints Endpoints, path string) (string, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("%w: path %s (expected at least owner/repo)", ErrInvalidGitHubURL, path)
	}

	owner := parts[0]
	repo := parts[1]

	// Default to main branch if no additional parts.
	if len(parts) == 2 {
		return endpoints.RawURL(owner, repo, "main", ""), nil
	}

	return parseGitHubPathWithRef(endpoints, owner, repo, parts[2:], path)
}

// parseGitHubPathWithRef parses a GitHub path with blob/tree and ref components.
func parseGitHubPathWithRef(endpoints Endpoints, owner, repo string, pathParts []string, originalPath string) (string, error) {
	if len(pathParts) < 2 {
		return "", fmt.Errorf("%w: path %s (expected owner/repo/blob|tree/ref)", ErrInvalidGitHubURL, originalPath)
	}

	urlType := pathParts[0] // blob or tree
	if urlType != "blob" && urlType != "tree" {
		return "", fmt.Errorf("%w: type %s (expected blob or tree)", ErrInvalidGitHubURL, urlType)
	}

	ref := pathParts[1]
	fileParts := pathParts[2:]

	return endpoints.RawURL(owner, repo, ref, strings.Join(fileParts, "/")), nil
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
		filePath = strings.Join(pathComponents[2:], "/")
	}

	return RepoEndpoints().RawURL(owner, repo, ref, filePath), nil
}
