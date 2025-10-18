package github

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v59/github"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ReleasesOptions contains options for fetching GitHub releases.
type ReleasesOptions struct {
	Owner              string
	Repo               string
	Limit              int
	Offset             int
	IncludePrereleases bool
	Since              *time.Time
}

// GetLatestRelease returns the latest release tag for a GitHub repository.
func GetLatestRelease(owner string, repo string) (string, error) {
	defer perf.Track(nil, "github.GetLatestRelease")()

	log.Debug("Fetching latest release from Github API", logFieldOwner, owner, logFieldRepo, repo)

	// Create a new GitHub client with authentication if available.
	ctx := context.Background()
	client := newGitHubClient(ctx)

	// Get the latest release.
	release, _, err := client.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return "", err
	}

	if release == nil || release.TagName == nil {
		return "", nil
	}

	return *release.TagName, nil
}

// GetReleases fetches GitHub releases with pagination, prerelease filtering, and date filtering.
func GetReleases(opts ReleasesOptions) ([]*github.RepositoryRelease, error) {
	defer perf.Track(nil, "github.GetReleases")()

	log.Debug("Fetching releases from GitHub API",
		logFieldOwner, opts.Owner,
		logFieldRepo, opts.Repo,
		"limit", opts.Limit,
		"offset", opts.Offset,
		"includePrereleases", opts.IncludePrereleases,
	)

	ctx := context.Background()
	client := newGitHubClient(ctx)

	// Check rate limits before making requests.
	rateLimits, _, err := client.RateLimit.Get(ctx)
	if err == nil && rateLimits != nil && rateLimits.Core != nil {
		remaining := rateLimits.Core.Remaining
		limit := rateLimits.Core.Limit

		log.Debug("GitHub API rate limits",
			"remaining", remaining,
			"limit", limit,
			"resetAt", rateLimits.Core.Reset.Time,
		)

		if remaining < githubAPIMinRateLimitThreshold {
			resetTime := rateLimits.Core.Reset.Time
			waitDuration := time.Until(resetTime)
			return nil, fmt.Errorf("%w: only %d requests remaining, resets at %s (in %s). Consider setting ATMOS_GITHUB_TOKEN or GITHUB_TOKEN for higher limits",
				errUtils.ErrGitHubRateLimitExceeded,
				remaining,
				resetTime.Format(time.RFC3339),
				waitDuration.Round(time.Second),
			)
		}
	}

	// Fetch releases from GitHub API with pagination.
	allReleases, err := fetchAllReleases(ctx, client, opts)
	if err != nil {
		return nil, err
	}

	// Apply filters.
	allReleases = filterPrereleases(allReleases, opts.IncludePrereleases)
	allReleases = filterByDate(allReleases, opts.Since)

	// Apply offset and limit.
	return applyPagination(allReleases, opts.Offset, opts.Limit), nil
}

// fetchAllReleases fetches releases from GitHub API with pagination.
func fetchAllReleases(ctx context.Context, client *github.Client, opts ReleasesOptions) ([]*github.RepositoryRelease, error) {
	defer perf.Track(nil, "github.fetchAllReleases")()

	// GitHub API uses per_page and page for pagination.
	// We need to fetch enough pages to satisfy offset + limit.
	startPage := (opts.Offset / githubAPIMaxPerPage) + 1
	var allReleases []*github.RepositoryRelease

	// Calculate how many pages we might need.
	estimatedPages := ((opts.Offset + opts.Limit) / githubAPIMaxPerPage) + 2

	for page := startPage; page <= startPage+estimatedPages; page++ {
		listOpts := &github.ListOptions{
			Page:    page,
			PerPage: githubAPIMaxPerPage,
		}

		releases, resp, err := client.Repositories.ListReleases(ctx, opts.Owner, opts.Repo, listOpts)
		if err != nil {
			// GitHub API only returns first 1000 results. If we've gone beyond that, return what we have.
			if resp != nil && resp.StatusCode == githubAPIStatus422 {
				break
			}
			return nil, handleGitHubAPIError(err, resp)
		}

		allReleases = append(allReleases, releases...)

		// Stop if we have enough releases or if this is the last page.
		if len(allReleases) >= opts.Offset+opts.Limit || resp.NextPage == 0 {
			break
		}
	}

	return allReleases, nil
}

// filterPrereleases removes prerelease versions if requested.
func filterPrereleases(releases []*github.RepositoryRelease, includePrereleases bool) []*github.RepositoryRelease {
	defer perf.Track(nil, "github.filterPrereleases")()

	if includePrereleases {
		return releases
	}

	filtered := make([]*github.RepositoryRelease, 0, len(releases))
	for _, release := range releases {
		if !release.GetPrerelease() {
			filtered = append(filtered, release)
		}
	}

	return filtered
}

// filterByDate filters releases by published date.
func filterByDate(releases []*github.RepositoryRelease, since *time.Time) []*github.RepositoryRelease {
	defer perf.Track(nil, "github.filterByDate")()

	if since == nil {
		return releases
	}

	filtered := make([]*github.RepositoryRelease, 0, len(releases))
	for _, release := range releases {
		publishedAt := release.GetPublishedAt().Time
		if publishedAt.After(*since) || publishedAt.Equal(*since) {
			filtered = append(filtered, release)
		}
	}

	return filtered
}

// applyPagination applies offset and limit to the releases slice.
func applyPagination(releases []*github.RepositoryRelease, offset, limit int) []*github.RepositoryRelease {
	defer perf.Track(nil, "github.applyPagination")()

	if offset >= len(releases) {
		return []*github.RepositoryRelease{}
	}

	end := offset + limit
	if end > len(releases) {
		end = len(releases)
	}

	return releases[offset:end]
}

// GetReleaseByTag fetches a specific GitHub release by tag name.
func GetReleaseByTag(owner, repo, tag string) (*github.RepositoryRelease, error) {
	defer perf.Track(nil, "github.GetReleaseByTag")()

	log.Debug("Fetching release by tag from GitHub API", logFieldOwner, owner, logFieldRepo, repo, "tag", tag)

	ctx := context.Background()
	client := newGitHubClient(ctx)

	release, resp, err := client.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, handleGitHubAPIError(err, resp)
	}

	return release, nil
}

// GetLatestReleaseInfo fetches the latest stable release from GitHub.
func GetLatestReleaseInfo(owner, repo string) (*github.RepositoryRelease, error) {
	defer perf.Track(nil, "github.GetLatestReleaseInfo")()

	log.Debug("Fetching latest release from GitHub API", logFieldOwner, owner, logFieldRepo, repo)

	ctx := context.Background()
	client := newGitHubClient(ctx)

	release, resp, err := client.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return nil, handleGitHubAPIError(err, resp)
	}

	return release, nil
}
