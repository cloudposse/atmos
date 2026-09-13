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

// publicGitHubEndpoints is the fixed, non-configurable Endpoints value for public github.com.
// It recognizes an explicit github.com URL regardless of what RepoEndpoints resolves to (which
// may be a different, GHES, host): IsHost normalizes case, a trailing dot, and the default
// port, so this also matches "GitHub.com", "github.com.", and "github.com:443".
var publicGitHubEndpoints = newEndpoints(defaultGitHubServerURL, defaultGitHubAPIURL)

// IsPublicGitHubHost reports whether host (case-insensitive, with port and trailing dot
// normalized) is public github.com, independent of whatever RepoEndpoints resolves to. Callers
// that must still recognize an explicit public github.com URL even when GITHUB_SERVER_URL
// points at a GitHub Enterprise Server host (e.g. converting a public github.com blob URL to
// raw content for an !include, regardless of the caller's own GHES configuration) should check
// this in addition to RepoEndpoints().IsHost.
func IsPublicGitHubHost(host string) bool {
	defer perf.Track(nil, "github.IsPublicGitHubHost")()

	return publicGitHubEndpoints.IsHost(host)
}

// newGitHubClient creates a new GitHub client. If a token is provided, it returns an
// authenticated client; otherwise, it returns an unauthenticated client. The second return
// value reports whether the client is effectively authenticated -- i.e. whether a token will
// actually be attached to requests against its endpoints (see newGitHubClientForEndpoints) --
// so callers whose behavior depends on auth state (e.g. rate-limit error hints) don't need to
// re-derive it from a raw token lookup that ignores host/scheme scoping.
func newGitHubClient(ctx context.Context) (*github.Client, bool) {
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
// WithEnterpriseURLs instead of public github.com. See newGitHubClient for the meaning of the
// second return value.
func newGitHubClientWithToken(ctx context.Context, token string) (*github.Client, bool) {
	defer perf.Track(nil, "github.newGitHubClientWithToken")()

	return newGitHubClientForEndpoints(ctx, token, RepoEndpoints())
}

// newToolchainGitHubClient creates a new GitHub client scoped to ToolchainEndpoints instead of
// RepoEndpoints. Toolchain-managed repositories (e.g. atmos's own PR/SHA/ref build artifacts
// used for self-install, or an arbitrary tool's release versions) live on public github.com by
// default even for GHES users, which is a separate concern from where the user's own
// repositories live -- see ToolchainEndpoints' doc comment.
//
// GetGitHubToken() resolves a single token without regard to host, so it is treated as scoped
// to RepoEndpoints() (the user's own repository host: GITHUB_SERVER_URL/GITHUB_API_URL) and is
// only forwarded here -- to ToolchainEndpoints -- when tokenForToolchainHost determines
// ToolchainEndpoints resolves to that same host. Otherwise a GHES-scoped token would leak to a
// different host (public github.com by default, or a differently hosted toolchain mirror).
// Separately, newGitHubClientForEndpoints withholds the token when ToolchainEndpoints is not
// https, so both rules apply regardless of call order.
func newToolchainGitHubClient(ctx context.Context) (*github.Client, bool) {
	defer perf.Track(nil, "github.newToolchainGitHubClient")()

	toolchainEndpoints := ToolchainEndpoints()
	token := tokenForToolchainHost(GetGitHubToken(), toolchainEndpoints)
	return newGitHubClientForEndpoints(ctx, token, toolchainEndpoints)
}

// tokenForToolchainHost returns token when it is safe to attach to requests against
// toolchainEndpoints, or "" otherwise. A repo-scoped token (from GetGitHubToken(), which
// resolves without regard to host) belongs to RepoEndpoints() -- it is only forwarded to
// toolchainEndpoints when they resolve to that same host; e.g. a GHES-scoped token must never
// reach the public github.com toolchain endpoints (or any other differently hosted toolchain
// mirror).
func tokenForToolchainHost(token string, toolchainEndpoints Endpoints) string {
	defer perf.Track(nil, "github.tokenForToolchainHost")()

	if token == "" {
		return ""
	}

	repo := RepoEndpoints()
	if !repo.IsHost(toolchainEndpoints.Host) {
		log.Debug("Toolchain endpoints host differs from repo endpoints host; withholding repo-scoped GitHub token",
			"repoHost", repo.Host, "toolchainHost", toolchainEndpoints.Host)
		return ""
	}
	return token
}

// newGitHubClientForEndpoints builds an authenticated (or, if token is empty, unauthenticated)
// *github.Client with an HTTP timeout, pointed at endpoints. The token is attached via the
// scoped-token transport (see NewScopedTokenHTTPClient) rather than golang.org/x/oauth2's
// static token source: oauth2's Transport re-adds Authorization unconditionally on every
// RoundTrip call, including one net/http built to follow a redirect -- covering neither a
// cross-host redirect nor a same-host https-to-http downgrade, both of which would otherwise
// forward the token beyond the endpoint it was scoped to. The scoped-token transport instead
// re-validates the actual request URL (host and scheme) on every call, which also subsumes the
// plain-HTTP check Endpoints.AllowsToken performs (a request never reaches an http:// URL with
// a token attached, whether or not it is a redirect).
//
// The second return value reports whether the resulting client is effectively authenticated:
// TokenForEndpoints applies the same host/scheme scoping NewScopedTokenHTTPClient uses
// internally, so this reflects whether a token will actually be attached to requests -- not
// merely whether the caller happened to resolve a non-empty token from the environment.
func newGitHubClientForEndpoints(_ context.Context, token string, endpoints Endpoints) (*github.Client, bool) {
	defer perf.Track(nil, "github.newGitHubClientForEndpoints")()

	httpClient := NewScopedTokenHTTPClient(token, endpoints, defaultHTTPTimeout)
	authenticated := TokenForEndpoints(endpoints, token) != ""

	return newScopedClient(httpClient, endpoints), authenticated
}

// newScopedClient builds a *github.Client from httpClient, pointed at GitHub.com by default or
// at a GitHub Enterprise Server instance when endpoints resolves a non-default server host or
// API URL. Checking APIURL in addition to Host honors an API-only override (e.g.
// ATMOS_TOOLCHAIN_GITHUB_API_URL pointed at a corporate API proxy while the server/web host
// stays github.com) that would otherwise be silently ignored. An error building the enterprise
// client (malformed URL) falls back to the public client rather than failing the caller
// outright, matching Endpoints' own fail-open behavior for invalid endpoint URLs.
func newScopedClient(httpClient *http.Client, endpoints Endpoints) *github.Client {
	if endpoints.isDefaultGitHubCom() && endpoints.APIURL == defaultGitHubAPIURL {
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

	// Select the endpoints matching this URL's host: an explicit github.com URL always resolves
	// against the public endpoints, even when RepoEndpoints (GITHUB_SERVER_URL) points at a
	// different GitHub Enterprise Server host -- otherwise a literal github.com link would be
	// rewritten to "<GHES>/raw/..." instead of raw.githubusercontent.com. Only URLs on the
	// configured GHES host use RepoEndpoints.
	switch {
	case publicGitHubEndpoints.IsHost(u.Host):
		return parseGitHubDotComURL(publicGitHubEndpoints, u.Path)
	case endpoints.IsHost(u.Host):
		return parseGitHubDotComURL(endpoints, u.Path)
	default:
		return "", fmt.Errorf("%w: %s (expected github.com or the configured GitHub Enterprise Server host)", ErrUnsupportedGitHubHost, u.Host)
	}
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
