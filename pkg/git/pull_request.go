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

// PullRequestBodyBadger is an optional interface a PullRequestPublisher can implement to supply
// its own branding for the default `vendor.ci.pull_request.body` template (see
// updater.RenderPRTemplates). It's optional, not part of PullRequestPublisher itself, because a
// forge's pull request markdown has its own quirks and capabilities -- raw HTML support, external
// image hosting requirements, light/dark switching, and so on -- that only the provider that
// speaks to it can know; a publisher that doesn't implement this gets a static, dependency-free
// default instead of being forced to declare one.
type PullRequestBodyBadger interface {
	PullRequestBodyBadge() string
}

// PullRequestRepository addresses a repository the way a publisher's Reconcile expects it, with
// the same Owner/Namespace/Repository semantics as PullRequestOptions.
type PullRequestRepository struct {
	Owner      string
	Namespace  []string
	Repository string
}

// PullRequestRepositoryResolver is an optional interface a PullRequestPublisher can implement to
// recognize its own forge's Git remote URLs (HTTPS or SSH) and return the repository address
// they name. It lets callers pick a publisher from a repository URI without knowing which forges
// exist: only the provider that speaks to a forge knows its URL conventions (hosts, SSH path
// prefixes, enterprise-server hostnames).
type PullRequestRepositoryResolver interface {
	// RepositoryFromURL reports ok=false for any URL that is not this forge's.
	RepositoryFromURL(uri string) (PullRequestRepository, bool)
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

// NewPullRequestPublisherForURL returns the registered publisher whose forge owns the Git remote
// URL uri, together with the repository address it resolved from it. Publishers are consulted in
// sorted name order and those that do not implement PullRequestRepositoryResolver are skipped.
// It returns ErrPullRequestPublisherUnavailable when no registered publisher recognizes uri.
func NewPullRequestPublisherForURL(uri string) (PullRequestPublisher, PullRequestRepository, error) {
	for _, name := range RegisteredPullRequestPublishers() {
		publisher, err := NewPullRequestPublisher(name)
		if err != nil {
			return nil, PullRequestRepository{}, err
		}
		resolver, ok := publisher.(PullRequestRepositoryResolver)
		if !ok {
			continue
		}
		if repository, ok := resolver.RepositoryFromURL(uri); ok {
			return publisher, repository, nil
		}
	}
	return nil, PullRequestRepository{}, fmt.Errorf("%w: no registered forge recognizes repository %q (registered: %v)",
		errUtils.ErrPullRequestPublisherUnavailable, uri, RegisteredPullRequestPublishers())
}
