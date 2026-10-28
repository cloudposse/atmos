package github

import (
	"context"

	"github.com/google/go-github/v59/github"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ReleaseClient reuses effective authentication for an optional background release check.
type ReleaseClient struct{ client *github.Client }

// NewReleaseClient resolves authentication once and reports whether requests carry credentials.
func NewReleaseClient(ctx context.Context) (*ReleaseClient, bool) {
	defer perf.Track(nil, "github.NewReleaseClient")()

	client, authenticated := newGitHubClient(ctx)
	return &ReleaseClient{client: client}, authenticated
}

// CheckBudget permits optional checks only with at least 75 percent of the core quota available.
// Missing quota data and failed lookups reject the check; this method never retries or waits.
func (c *ReleaseClient) CheckBudget(ctx context.Context) error {
	defer perf.Track(nil, "github.ReleaseClient.CheckBudget")()

	return checkRateLimitWithPolicy(ctx, c.client, true, rateLimitPolicy{minimumPercent: 75, strict: true})
}

// Latest returns the latest release using the same client and caller deadline.
func (c *ReleaseClient) Latest(ctx context.Context, owner, repo string) (string, error) {
	defer perf.Track(nil, "github.ReleaseClient.Latest")()

	return getLatestReleaseWithClient(ctx, c.client, owner, repo)
}
