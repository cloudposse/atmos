package git

import (
	"context"
	"fmt"
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
)

// PullRequestOptions is forge-neutral data used to create or reconcile a pull request.
// Head is the branch name without an owner qualifier.
type PullRequestOptions struct {
	Owner      string
	Repository string
	Base       string
	Head       string
	Title      string
	Body       string
	Labels     []string
	Draft      bool
	Reviewers  []string
	Assignees  []string
}

type PullRequestResult struct {
	Number  int    `json:"number"`
	URL     string `json:"url"`
	Created bool   `json:"created"`
}

// PullRequestPublisher hides forge REST APIs behind a small, provider-neutral
// contract. GitLab and Bitbucket implementations can register independently.
type PullRequestPublisher interface {
	Reconcile(ctx context.Context, options *PullRequestOptions) (*PullRequestResult, error)
}

type PullRequestPublisherFactory func() (PullRequestPublisher, error)

var (
	pullRequestPublishersMu sync.RWMutex
	pullRequestPublishers   = map[string]PullRequestPublisherFactory{}
)

func RegisterPullRequestPublisher(name string, factory PullRequestPublisherFactory) {
	pullRequestPublishersMu.Lock()
	defer pullRequestPublishersMu.Unlock()
	pullRequestPublishers[name] = factory
}

func NewPullRequestPublisher(name string) (PullRequestPublisher, error) {
	pullRequestPublishersMu.RLock()
	factory, ok := pullRequestPublishers[name]
	pullRequestPublishersMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q (registered: %v)", errUtils.ErrPullRequestPublisherUnavailable, name, RegisteredPullRequestPublishers())
	}
	return factory()
}

func RegisteredPullRequestPublishers() []string {
	pullRequestPublishersMu.RLock()
	defer pullRequestPublishersMu.RUnlock()
	names := make([]string, 0, len(pullRequestPublishers))
	for name := range pullRequestPublishers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
