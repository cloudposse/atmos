package github

import (
	"context"

	"github.com/google/go-github/v59/github"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ListTags returns all tags for a repository, newest first as reported by the
// GitHub API, including each tag's commit SHA. Pagination is handled
// internally; authentication and rate-limit handling follow the shared client
// behavior.
func ListTags(ctx context.Context, owner, repo string) ([]*github.RepositoryTag, error) {
	defer perf.Track(nil, "github.ListTags")()

	log.Debug("Fetching tags from GitHub API", logFieldOwner, owner, logFieldRepo, repo)

	client := newGitHubClient(ctx)
	var allTags []*github.RepositoryTag
	page := 1
	for {
		tags, resp, err := client.Repositories.ListTags(ctx, owner, repo, &github.ListOptions{
			Page:    page,
			PerPage: githubAPIMaxPerPage,
		})
		if err != nil {
			return nil, handleGitHubAPIError(err, resp)
		}
		allTags = append(allTags, tags...)
		if resp == nil || resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}
	return allTags, nil
}
