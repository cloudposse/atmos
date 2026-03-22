package github

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	ghpkg "github.com/google/go-github/v59/github"
	"golang.org/x/sync/singleflight"

	"github.com/cloudposse/atmos/pkg/perf"
)

// defaultArchivedCheckTimeout is the default maximum time to wait for a GitHub
// archived-status check. Kept short because the check is best-effort and should
// never block vendoring for long. Override via ATMOS_GITHUB_ARCHIVED_CHECK_TIMEOUT
// (e.g. "0s" to skip the check in air-gapped environments, "10s" for slower networks).
const defaultArchivedCheckTimeout = 5 * time.Second

// archivedCheckTimeoutNs stores the resolved timeout in nanoseconds as an atomic int64.
// Atomic access allows SetArchivedCheckTimeoutForTest to be called safely from test goroutines
// concurrently with IsRepoArchived reading the value. Initialized from
// ATMOS_GITHUB_ARCHIVED_CHECK_TIMEOUT in init(); defaults to defaultArchivedCheckTimeout.
var archivedCheckTimeoutNs = atomic.Int64{}

// getArchivedCheckTimeout returns the current archived-check timeout.
func getArchivedCheckTimeout() time.Duration {
	return time.Duration(archivedCheckTimeoutNs.Load())
}

// setArchivedCheckTimeout sets the archived-check timeout.
func setArchivedCheckTimeout(d time.Duration) {
	archivedCheckTimeoutNs.Store(int64(d))
}

// archivedRepoCache caches (owner/repo → archived) results so that multiple vendor
// sources pointing at the same repository only issue one API call per run.
var archivedRepoCache sync.Map

// archivedRepoSFGroup deduplicates concurrent in-flight checks for the same repo so
// that parallel vendor pulls never race to issue multiple API calls for the same key.
var archivedRepoSFGroup singleflight.Group

// scpGitHubURLPattern matches SCP-style GitHub URLs (e.g., git@github.com:owner/repo.git).
// Capture groups: (1) host, (2) owner, (3) repo.
// In each character class the '-' is placed last (immediately before ']') so that it is
// always treated as a literal hyphen, not as a range operator.
var scpGitHubURLPattern = regexp.MustCompile(`^(?:[A-Za-z0-9_.+-]+@)?([A-Za-z0-9_.-]+\.[A-Za-z0-9_.-]+):([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?(?://.*)?$`)

func init() {
	// Initialise with the default. The atomic is used instead of a plain var so that
	// SetArchivedCheckTimeoutForTest can safely modify it concurrently with IsRepoArchived.
	setArchivedCheckTimeout(defaultArchivedCheckTimeout)

	if v := os.Getenv("ATMOS_GITHUB_ARCHIVED_CHECK_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			// Warn using fmt so the message is visible even before the logger is
			// initialized, and to avoid an import cycle with pkg/logger.
			_, _ = fmt.Fprintf(os.Stderr,
				"atmos: WARN invalid ATMOS_GITHUB_ARCHIVED_CHECK_TIMEOUT %q"+
					" (expected a Go duration such as \"5s\" or \"0s\"); using default %s\n",
				v, defaultArchivedCheckTimeout)
		} else {
			setArchivedCheckTimeout(d)
		}
	}
}

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

	// Handle github:// scheme (case-insensitive, e.g., github://owner/repo or GITHUB://owner/repo/subdir@ref).
	if strings.HasPrefix(strings.ToLower(uri), "github://") {
		remainder := uri[len("github://"):]
		// Strip @ref suffix.
		if atIdx := strings.Index(remainder, "@"); atIdx >= 0 {
			remainder = remainder[:atIdx]
		}
		// Strip #fragment suffix.
		if hashIdx := strings.Index(remainder, "#"); hashIdx >= 0 {
			remainder = remainder[:hashIdx]
		}
		parts := strings.SplitN(strings.Trim(remainder, "/"), "/", 3)
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			// Strip .git suffix from the repository name, as the github:// scheme
			// does not automatically strip it (unlike the HTTPS/SCP paths).
			return parts[0], strings.TrimSuffix(parts[1], ".git"), true
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

	// Strip all leading slashes. url.Parse preserves multiple leading slashes in the
	// path (e.g., "https://github.com//owner/repo" → Path="//owner/repo"), so we use
	// TrimLeft to normalize double-slash URIs produced by some template expansions.
	path := strings.TrimLeft(parsed.Path, "/")
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
// When archivedCheckTimeout is zero or negative (e.g., ATMOS_GITHUB_ARCHIVED_CHECK_TIMEOUT=0s),
// the check is skipped entirely and false is returned so vendoring proceeds normally.
// In a slow-network environment with many unique repositories the per-repo timeout
// (default 5s) bounds each API call; the total delay is at most 5s × number_of_repos.
//
// It returns an error when the API call fails; callers that treat the check as
// best-effort should silently ignore the error and continue rather than
// blocking the operation.
func IsRepoArchived(ctx context.Context, owner, repo string) (bool, error) {
	defer perf.Track(nil, "github.IsRepoArchived")()

	timeout := getArchivedCheckTimeout()

	// Honour a zero or negative timeout as an explicit opt-out: skip the API call
	// entirely so air-gapped environments never pay any latency for this check.
	if timeout <= 0 {
		return false, nil
	}

	// Use a short timeout so this best-effort check never hangs vendoring.
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := newGitHubClient(checkCtx)
	return isRepoArchivedWithClient(checkCtx, client.Repositories, owner, repo)
}

// SeedArchivedRepoCache pre-populates the archived-repo cache for the given owner/repo
// pair. This is primarily intended for tests to exercise callers of IsRepoArchived
// (e.g., warnIfArchivedGitHubRepo) without making real GitHub API calls.
//
// NOTE: This is a test utility. It is exported because it is needed from tests in
// multiple packages (e.g., internal/exec), which Go's export_test.go mechanism cannot
// satisfy across package boundaries. Do not rely on this function in production code.
func SeedArchivedRepoCache(owner, repo string, archived bool) {
	archivedRepoCache.Store(owner+"/"+repo, archived)
}

// ResetArchivedRepoCache clears the in-memory archived-repo cache and resets the
// singleflight group. It is intended for use in tests to prevent cache entries from
// one sub-test leaking into subsequent sub-tests.
//
// NOTE: This is a test utility — see SeedArchivedRepoCache for the rationale.
//
// CONCURRENCY: Must only be called when no concurrent IsRepoArchived or
// isRepoArchivedWithClient calls are in progress. Resetting the singleflight group
// while an in-flight call is running may cause the in-flight callback to write to
// the old group after it has been replaced.
func ResetArchivedRepoCache() {
	archivedRepoCache.Range(func(k, _ any) bool {
		archivedRepoCache.Delete(k)
		return true
	})
	// Reset the singleflight group so in-flight calls don't return stale results
	// after a cache reset. Tests must not have concurrent calls in progress when
	// calling this function.
	archivedRepoSFGroup = singleflight.Group{}
}

// isRepoArchivedWithClient is the internal implementation of IsRepoArchived.
// It is separated so tests can inject a mock client without going through the
// full client-creation path.
//
// Concurrent callers for the same key are coalesced via archivedRepoSFGroup so that
// only one API call is ever in-flight at a time for a given owner/repo pair, even
// under parallel vendor pulls.
func isRepoArchivedWithClient(ctx context.Context, client githubRepositoriesClient, owner, repo string) (bool, error) {
	cacheKey := owner + "/" + repo

	// Return cached result if available (fast path, no singleflight overhead).
	if v, ok := archivedRepoCache.Load(cacheKey); ok {
		return v.(bool), nil
	}

	// Deduplicate concurrent callers: only the first goroutine issues the API call;
	// all others wait and share the result.
	result, err, _ := archivedRepoSFGroup.Do(cacheKey, func() (interface{}, error) {
		// Re-check cache inside singleflight in case a previous call populated it.
		if v, ok := archivedRepoCache.Load(cacheKey); ok {
			return v.(bool), nil
		}

		repository, resp, apiErr := client.Get(ctx, owner, repo)
		if apiErr != nil {
			return false, handleGitHubAPIError(apiErr, resp)
		}

		archived := repository.GetArchived()
		archivedRepoCache.Store(cacheKey, archived)

		return archived, nil
	})

	if err != nil {
		return false, err
	}

	return result.(bool), nil
}

// githubRepositoriesClient is a minimal interface around the GitHub Repositories
// service so that tests can inject a mock.
type githubRepositoriesClient interface {
	Get(ctx context.Context, owner, repo string) (*ghpkg.Repository, *ghpkg.Response, error)
}
