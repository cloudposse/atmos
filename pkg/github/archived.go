package github

import (
	"context"
	"net/url"
	"strings"

	"github.com/google/go-github/v59/github"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// githubHost is the hostname that identifies a GitHub.com repository (as opposed to GitLab,
// Bitbucket, a self-hosted Git server, etc.).
const githubHost = "github.com"

// IsArchived reports whether a GitHub repository is archived. Archived-ness is repository
// metadata only exposed by the GitHub REST API (the `Archived` field on *github.Repository`),
// not by the git wire protocol used for tag listing, so this is a separate API call/round-trip
// from ListTags. Authentication and rate-limit handling follow the shared client behavior (see
// newGitHubClient/handleGitHubAPIError).
func IsArchived(ctx context.Context, owner, repo string) (bool, error) {
	defer perf.Track(nil, "github.IsArchived")()

	log.Debug("Checking repository archived status via GitHub API", logFieldOwner, owner, logFieldRepo, repo)

	client := newGitHubClient(ctx)
	return getArchivedStatus(ctx, client, owner, repo)
}

// getArchivedStatus is the client-injectable core of IsArchived, split out (mirroring
// fetchAllReleases' pattern in releases.go) so tests can exercise it against a mock HTTP server
// instead of the real GitHub API.
func getArchivedStatus(ctx context.Context, client *github.Client, owner, repo string) (bool, error) {
	defer perf.Track(nil, "github.getArchivedStatus")()

	repository, resp, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return false, handleGitHubAPIError(err, resp)
	}

	return repository.GetArchived(), nil
}

// ParseOwnerRepo extracts the owner and repository name from a Git URI, as produced by
// version.ExtractGitURI (e.g. "https://github.com/owner/repo.git"). It returns ok=false for any
// URI that isn't a github.com repository (GitLab, Bitbucket, self-hosted Git servers, etc.), so
// callers can skip GitHub-only capabilities (like archived-repo detection) for those sources
// without treating it as an error.
func ParseOwnerRepo(gitURI string) (owner, repo string, ok bool) {
	defer perf.Track(nil, "github.ParseOwnerRepo")()

	u, err := url.Parse(gitURI)
	if err != nil || u.Host != githubHost {
		return "", "", false
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}

	return parts[0], strings.TrimSuffix(parts[1], ".git"), true
}
