package release

import (
	"context"

	"github.com/cloudposse/atmos/magefiles/release/milestone"
)

type Milestone = milestone.Milestone

type ReleasePR = milestone.ReleasePR

var ErrMilestonePolicy = milestone.ErrMilestonePolicy

type milestoneClient struct {
	*milestone.Client
	watcher releaseWatcher
}

func newMilestoneClient(ctx context.Context, repo string) (*milestoneClient, error) {
	client, err := milestone.NewClient(ctx, repo)
	if err != nil {
		return nil, err
	}
	return &milestoneClient{Client: client, watcher: ghReleaseWatcher{}}, nil
}

func milestonePages[T any](c *milestoneClient, path string) ([]T, error) {
	return milestone.Pages[T](c.Client, path)
}
