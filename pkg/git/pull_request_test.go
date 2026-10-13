package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

type testPullRequestPublisher struct{}

func (testPullRequestPublisher) Reconcile(context.Context, *PullRequestOptions) (*PullRequestResult, error) {
	return &PullRequestResult{Number: 1}, nil
}

func TestPullRequestPublisherRegistry(t *testing.T) {
	name := t.Name()
	RegisterPullRequestPublisher(name, func() (PullRequestPublisher, error) {
		return testPullRequestPublisher{}, nil
	})

	publisher, err := NewPullRequestPublisher(name)
	require.NoError(t, err)
	result, err := publisher.Reconcile(context.Background(), &PullRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Number)
	assert.Contains(t, RegisteredPullRequestPublishers(), name)

	_, err = NewPullRequestPublisher("not-registered")
	assert.ErrorIs(t, err, errUtils.ErrPullRequestPublisherUnavailable)
}

func TestPullRequestPublisherFactoryError(t *testing.T) {
	name := fmt.Sprintf("%s-error", t.Name())
	want := errors.New("factory failed")
	RegisterPullRequestPublisher(name, func() (PullRequestPublisher, error) {
		return nil, want
	})
	_, err := NewPullRequestPublisher(name)
	assert.ErrorIs(t, err, want)
}

// resolvingPublisher is a publisher that claims repositories under its host prefix.
type resolvingPublisher struct {
	testPullRequestPublisher
	prefix string
}

func (r resolvingPublisher) RepositoryFromURL(uri string) (PullRequestRepository, bool) {
	if strings.HasPrefix(uri, r.prefix) {
		return PullRequestRepository{Owner: "owner", Repository: strings.TrimPrefix(uri, r.prefix)}, true
	}
	return PullRequestRepository{}, false
}

// isolatePublisherRegistry swaps in an empty publisher registry for the test, so lookups by URL
// only see the publishers the test registers (the real registry is process-global).
func isolatePublisherRegistry(t *testing.T) {
	t.Helper()
	pullRequestPublishersMu.Lock()
	previous := pullRequestPublishers
	pullRequestPublishers = map[string]PullRequestPublisherFactory{}
	pullRequestPublishersMu.Unlock()
	t.Cleanup(func() {
		pullRequestPublishersMu.Lock()
		pullRequestPublishers = previous
		pullRequestPublishersMu.Unlock()
	})
}

// TestNewPullRequestPublisherForURL verifies the publisher whose forge owns the URL is selected,
// publishers without a resolver are skipped, and an unrecognized URL names the registered forges.
func TestNewPullRequestPublisherForURL(t *testing.T) {
	isolatePublisherRegistry(t)
	RegisterPullRequestPublisher("a-plain", func() (PullRequestPublisher, error) { return testPullRequestPublisher{}, nil })
	RegisterPullRequestPublisher("b-forge-one", func() (PullRequestPublisher, error) {
		return resolvingPublisher{prefix: "https://one.example/"}, nil
	})
	RegisterPullRequestPublisher("c-forge-two", func() (PullRequestPublisher, error) {
		return resolvingPublisher{prefix: "https://two.example/"}, nil
	})

	publisher, repository, err := NewPullRequestPublisherForURL("https://two.example/deployments")
	require.NoError(t, err)
	assert.Equal(t, resolvingPublisher{prefix: "https://two.example/"}, publisher)
	assert.Equal(t, PullRequestRepository{Owner: "owner", Repository: "deployments"}, repository)

	_, _, err = NewPullRequestPublisherForURL("https://three.example/deployments")
	require.ErrorIs(t, err, errUtils.ErrPullRequestPublisherUnavailable)
	assert.Contains(t, err.Error(), "https://three.example/deployments")
	assert.Contains(t, err.Error(), "a-plain", "the error lists the registered forges")
}

// TestNewPullRequestPublisherForURLFactoryError verifies a publisher that cannot be created is
// reported rather than silently skipped.
func TestNewPullRequestPublisherForURLFactoryError(t *testing.T) {
	isolatePublisherRegistry(t)
	want := errors.New("factory failed")
	RegisterPullRequestPublisher("broken", func() (PullRequestPublisher, error) { return nil, want })

	_, _, err := NewPullRequestPublisherForURL("https://one.example/deployments")

	assert.ErrorIs(t, err, want)
}
