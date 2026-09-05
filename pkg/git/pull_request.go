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
//
// Owner is the single top-level account every forge has (GitHub owner, GitLab top-level
// namespace, Bitbucket workspace, Azure DevOps organization). Namespace carries any additional
// path segments a forge requires between Owner and Repository -- nil/empty for two-segment
// forges like GitHub, a single project name (len == 1) for Azure DevOps' three-segment
// organization/project/repository addressing, or an arbitrary depth of subgroups for GitLab.
// Providers that don't need it should reject a non-empty Namespace rather than silently
// ignoring it, so misconfiguration surfaces instead of resolving to the wrong repository.
type PullRequestOptions struct {
	Owner      string
	Namespace  []string
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

// PullRequestResult represents the outcome of pull request reconciliation.
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

// PullRequestPublisherFactory creates a pull request publisher.
type PullRequestPublisherFactory func() (PullRequestPublisher, error)

var (
	pullRequestPublishersMu sync.RWMutex
	pullRequestPublishers   = map[string]PullRequestPublisherFactory{}
)

// RegisterPullRequestPublisher registers a named pull request publisher factory.
func RegisterPullRequestPublisher(name string, factory PullRequestPublisherFactory) {
	pullRequestPublishersMu.Lock()
	defer pullRequestPublishersMu.Unlock()
	pullRequestPublishers[name] = factory
}

// NewPullRequestPublisher creates the registered pull request publisher named name.
func NewPullRequestPublisher(name string) (PullRequestPublisher, error) {
	pullRequestPublishersMu.RLock()
	factory, ok := pullRequestPublishers[name]
	pullRequestPublishersMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q (registered: %v)", errUtils.ErrPullRequestPublisherUnavailable, name, RegisteredPullRequestPublishers())
	}
	return factory()
}

// RegisteredPullRequestPublishers returns registered publisher names in sorted order.
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
