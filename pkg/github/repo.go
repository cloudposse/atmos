package github

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// scpGitHubURLPattern matches SCP-style GitHub URLs (e.g., git@github.com:owner/repo.git).
// Capture groups: (1) host, (2) owner, (3) repo.
var scpGitHubURLPattern = regexp.MustCompile(`^(?:[\w.-]+@)?([\w.-]+\.[\w.-]+):([\w.-]+)/([\w.-]+?)(?:\.git)?(?://.*)?$`)

// ParseGitHubOwnerRepo extracts the GitHub owner and repository name from a URI.
// It handles various URI formats used in vendor configuration:
//   - github.com/owner/repo//path?ref=v1
//   - https://github.com/owner/repo.git//path?ref=v1
//   - git::https://github.com/owner/repo
//   - git@github.com:owner/repo.git//path
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

	host := strings.ToLower(parsed.Host)
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
// It returns an error when the API call fails; callers that treat the check as
// best-effort should silently ignore the error and continue rather than
// blocking the operation.
func IsRepoArchived(owner, repo string) (bool, error) {
	defer perf.Track(nil, "github.IsRepoArchived")()

	ctx := context.Background()
	client := newGitHubClient(ctx)

	repository, resp, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return false, handleGitHubAPIError(err, resp)
	}

	return repository.GetArchived(), nil
}
