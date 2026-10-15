package ci

//go:generate go run go.uber.org/mock/mockgen -typed -destination=mock_labels_test.go -package=ci github.com/cloudposse/atmos/pkg/ci/internal/provider LabelReader

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

type labelsTestProvider struct {
	*MockProvider
	*MockLabelReader
	labels []string
	err    error
	calls  int
}

func setupLabelsProvider(t *testing.T) (*labelsTestProvider, *Context) {
	t.Helper()
	t.Cleanup(SwapRegistryForTest())
	ResetLabelsCache()
	t.Cleanup(ResetLabelsCache)
	ctrl := gomock.NewController(t)
	p := &labelsTestProvider{MockProvider: NewMockProvider(ctrl), MockLabelReader: NewMockLabelReader(ctrl), labels: []string{"preview", "deploy"}}
	run := &Context{RepoOwner: "owner", RepoName: "repo", PullRequest: &PRInfo{Number: 42}}
	p.MockProvider.EXPECT().Name().Return("labels-test").AnyTimes()
	p.MockProvider.EXPECT().Detect().Return(true).AnyTimes()
	p.MockProvider.EXPECT().Context().DoAndReturn(func() (*Context, error) { return run, nil }).AnyTimes()
	p.MockLabelReader.EXPECT().Labels(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *Context) ([]string, error) {
		p.calls++
		return p.labels, p.err
	}).AnyTimes()
	Register(p)
	return p, run
}

func TestLabelsSnapshot(t *testing.T) {
	p, run := setupLabelsProvider(t)
	p.labels = []string{"preview", "deploy", "preview"}
	got, err := Labels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"deploy", "preview"}, got)
	got[0] = "mutated result"
	p.labels[0] = "mutated source"
	got, err = Labels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"deploy", "preview"}, got)
	assert.Equal(t, 1, p.calls)

	ResetLabelsCache()
	p.labels = []string{"new"}
	got, err = Labels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"new"}, got)
	assert.Equal(t, 2, p.calls)

	run.PullRequest.Number++
	p.labels = nil
	got, err = Labels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{}, got)
	assert.Equal(t, 3, p.calls)
}

func TestLabelsConcurrentSnapshot(t *testing.T) {
	p, _ := setupLabelsProvider(t)
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			got, err := Labels(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, []string{"deploy", "preview"}, got)
		})
	}
	wg.Wait()
	assert.Equal(t, 1, p.calls)
}

func TestLabelsNoReview(t *testing.T) {
	p, run := setupLabelsProvider(t)
	run.PullRequest = nil
	got, err := Labels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{}, got)
	assert.Zero(t, p.calls)
	Reset()
	got, err = Labels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{}, got)
}

func TestLabelsErrors(t *testing.T) {
	t.Run("API failure is not cached as an empty list", func(t *testing.T) {
		p, _ := setupLabelsProvider(t)
		p.err = errUtils.ErrGitHubTokenNotFound
		got, err := Labels(context.Background())
		require.ErrorIs(t, err, errUtils.ErrCILabelsFetchFailed)
		require.ErrorIs(t, err, errUtils.ErrGitHubTokenNotFound)
		assert.Nil(t, got)
		p.err = nil
		got, err = Labels(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"deploy", "preview"}, got)
		assert.Equal(t, 2, p.calls)
	})
	t.Run("unknown review identity", func(t *testing.T) {
		p, run := setupLabelsProvider(t)
		run.PullRequest.Number = 0
		_, err := Labels(context.Background())
		require.ErrorIs(t, err, errUtils.ErrCILabelsFetchFailed)
		assert.Zero(t, p.calls)
	})
	t.Run("unsupported provider", func(t *testing.T) {
		p, _ := setupLabelsProvider(t)
		Register(p.MockProvider)
		_, err := Labels(context.Background())
		require.ErrorIs(t, err, errUtils.ErrCIOperationNotSupported)
	})
	t.Run("context error", func(t *testing.T) {
		t.Cleanup(SwapRegistryForTest())
		p := NewMockProvider(gomock.NewController(t))
		p.EXPECT().Name().Return("failed-context").AnyTimes()
		p.EXPECT().Detect().Return(true)
		p.EXPECT().Context().Return(nil, errUtils.ErrInvalidConfig)
		Register(p)
		_, err := Labels(context.Background())
		require.ErrorIs(t, err, errUtils.ErrCILabelsFetchFailed)
		require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	})
}
