package github

import (
	"context"
	"net/http"
	"net/url"

	"github.com/google/go-github/v59/github"

	"github.com/cloudposse/atmos/pkg/perf"
)

// RepositoryHealth retains uncertainty: GitHub hides private repositories behind 404s.
type RepositoryHealth struct {
	Status  string
	Message string
}

// InspectRepository checks repository metadata without cloning or resolving versions.
// Tokens retain the existing host/scheme scoping used by other GitHub operations.
func InspectRepository(ctx context.Context, gitURI string) RepositoryHealth {
	defer perf.Track(nil, "github.InspectRepository")()
	owner, repo, ok := ParseOwnerRepo(gitURI)
	if !ok {
		return RepositoryHealth{"unsupported", "Repository metadata is unavailable for this host"}
	}
	endpoints := RepoEndpoints()
	token := ""
	u, _ := url.Parse(gitURI)
	if IsPublicGitHubHost(u.Host) && !endpoints.IsHost(u.Host) {
		endpoints = publicGitHubEndpoints
	} else {
		token = gitHubTokenForHost(ctx, endpoints.Host)
	}
	client, _ := newGitHubClientForEndpoints(ctx, token, endpoints)
	return inspectRepository(ctx, client, owner, repo)
}

func inspectRepository(ctx context.Context, client *github.Client, owner, repo string) RepositoryHealth {
	repository, response, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		if response != nil {
			switch response.StatusCode {
			case http.StatusNotFound:
				return RepositoryHealth{"unavailable", "Repository was not found or is private; verify its URL and credential access"}
			case http.StatusGone:
				return RepositoryHealth{"missing", "The repository server reports that the repository is gone"}
			case http.StatusUnauthorized, http.StatusForbidden:
				return RepositoryHealth{"inconclusive", "Repository access was denied or rate limited"}
			}
		}
		return RepositoryHealth{"inconclusive", "Repository metadata could not be retrieved; check connectivity and access"}
	}
	if repository.GetArchived() {
		return RepositoryHealth{"archived", "Upstream repository is archived"}
	}
	return RepositoryHealth{"healthy", "Repository is available and is not archived"}
}
