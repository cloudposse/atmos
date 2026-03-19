package github

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	ghpkg "github.com/google/go-github/v59/github"

	"github.com/cloudposse/atmos/pkg/perf"
)

// archivedCheckTimeout is the maximum time to wait for a GitHub archived-status check.
// Kept short because the check is best-effort and should never block vendoring for long.
const archivedCheckTimeout = 5 * time.Second

// archivedRepoCache caches (owner/repo → archived) results so that multiple vendor
// sources pointing at the same repository only issue one API call per run.
var archivedRepoCache sync.Map

// scpGitHubURLPattern matches SCP-style GitHub URLs (e.g., git@github.com:owner/repo.git).
// Capture groups: (1) host, (2) owner, (3) repo.
var scpGitHubURLPattern = regexp.MustCompile(`^(?:[\w.-]+@)?([\w.-]+\.[\w.-]+):([\w.-]+)/([\w.-]+?)(?:\.git)?(?://.*)?$`)

// ParseGitHubOwnerRepo extracts the GitHub owner and repository name from a URI.
// It handles various URI formats used in vendor configuration:
//   - github.com/owner/repo//path?ref=v1
//   - https://github.com/owner/repo.git//path?ref=v1
//   - git::https://github.com/owner/repo
//   - git@github.com:owner/repo.git//path
//   - github://owner/repo
//
// Returns owner, repo, and true when successfully parsed; empty strings and false otherwise.
func ParseGitHubOwnerRepo(uri string) (owner, repo string, ok bool) {
	defer perf.Track(nil, "github.ParseGitHubOwnerRepo")()

	if uri == "" {
		return "", "", false
	}

	// Strip go-getter force scheme prefix (e.g., "git::").
	if idx := strings.Index(uri, "::"); idx >= 0 {
		uri = uri[idx+2:]
	}

	// Handle github:// scheme (e.g., github://owner/repo or github://owner/repo/subdir@ref).
	if strings.HasPrefix(uri, "github://") {
		remainder := strings.TrimPrefix(uri, "github://")
		// Strip @ref suffix.
		if atIdx := strings.Index(remainder, "@"); atIdx >= 0 {
			remainder = remainder[:atIdx]
		}
		parts := strings.SplitN(strings.Trim(remainder, "/"), "/", 3)
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1], true
		}
		return "", "", false
	}

	// Handle SCP-style Git URLs (e.g., git@github.com:owner/repo.git).
	if strings.Contains(uri, ":") && !strings.Contains(uri, "://") {
		matches := scpGitHubURLPattern.FindStringSubmatch(uri)
		if len(matches) == 4 {
			host := strings.ToLower(matches[1])
			if host == "github.com" {
				return matches[2], matches[3], true
			}
		}
		return "", "", false
	}

	// Strip go-getter subdirectory delimiter and everything after it.
	// The delimiter "//" separates the repo URL from the subdirectory path.
	if idx := strings.Index(uri, "//"); idx >= 0 {
		// Make sure we're not stripping the "://" scheme separator.
		beforeSlashes := uri[:idx]
		if !strings.HasSuffix(beforeSlashes, ":") {
			uri = beforeSlashes
		}
	}

	// Strip query string.
	if idx := strings.Index(uri, "?"); idx >= 0 {
		uri = uri[:idx]
	}

	// Ensure the URI has a scheme so url.Parse can handle it correctly.
	if !strings.Contains(uri, "://") {
		uri = "https://" + uri
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		return "", "", false
	}

	// Normalize the host: strip any port (e.g., github.com:443 → github.com) and
	// lower-case before comparing.
	rawHost := parsed.Host
	if colonIdx := strings.LastIndex(rawHost, ":"); colonIdx >= 0 {
		rawHost = rawHost[:colonIdx]
	}
	host := strings.ToLower(rawHost)
	if host != "github.com" {
		return "", "", false
	}

	// Path format: /owner/repo or /owner/repo.git//subdir
	path := strings.TrimPrefix(parsed.Path, "/")
	// Strip the go-getter path-level subdirectory delimiter ("//" within the path).
	// This is required for any URL where the scheme's "//" (e.g., "ssh://") caused
	// the top-level stripping above to stop at the scheme separator, leaving the
	// path-level "//" subdirectory still in the URL path after parsing.
	if idx := strings.Index(path, "//"); idx >= 0 {
		path = path[:idx]
	}
	path = strings.TrimSuffix(path, ".git")

	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}

	return parts[0], parts[1], true
}

// IsRepoArchived reports whether a GitHub repository is archived.
// It accepts a parent context so callers can propagate cancellation (e.g., Ctrl+C).
// The check uses a short internal timeout (archivedCheckTimeout) to avoid blocking
// vendoring in air-gapped or slow-network environments. Results are cached for the
// duration of the process so repeated calls for the same repository are free.
//
// It returns an error when the API call fails; callers that treat the check as
// best-effort should silently ignore the error and continue rather than
// blocking the operation.
func IsRepoArchived(ctx context.Context, owner, repo string) (bool, error) {
	defer perf.Track(nil, "github.IsRepoArchived")()

	// Use a short timeout so this best-effort check never hangs vendoring.
	checkCtx, cancel := context.WithTimeout(ctx, archivedCheckTimeout)
	defer cancel()

	client := newGitHubClient(checkCtx)
	return isRepoArchivedWithClient(checkCtx, client.Repositories, owner, repo)
}

// isRepoArchivedWithClient is the internal implementation of IsRepoArchived.
// It is separated so tests can inject a mock client without going through the
// full client-creation path.
func isRepoArchivedWithClient(ctx context.Context, client githubRepositoriesClient, owner, repo string) (bool, error) {
	cacheKey := owner + "/" + repo

	// Return cached result if available.
	if v, ok := archivedRepoCache.Load(cacheKey); ok {
		return v.(bool), nil
	}

	repository, resp, err := client.Get(ctx, owner, repo)
	if err != nil {
		return false, handleGitHubAPIError(err, resp)
	}

	archived := repository.GetArchived()
	archivedRepoCache.Store(cacheKey, archived)

	return archived, nil
}

// githubRepositoriesClient is a minimal interface around the GitHub Repositories
// service so that tests can inject a mock.
type githubRepositoriesClient interface {
	Get(ctx context.Context, owner, repo string) (*ghpkg.Repository, *ghpkg.Response, error)
}
