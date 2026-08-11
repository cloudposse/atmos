package git

import (
	"context"
	"errors"
	"fmt"
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
