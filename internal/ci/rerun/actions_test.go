package rerun

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRerunFailedJobs(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().
			RequestWithContext(gomock.Any(), http.MethodPost, "repos/cloudposse/atmos/actions/runs/123/rerun-failed-jobs", nil).
			Return(jsonResponse(201, "", nil), nil) //nolint:bodyclose // closed by the code under test, not this fixture

		stillRunning, err := RerunFailedJobs(context.Background(), client, "cloudposse/atmos", "123")
		require.NoError(t, err)
		assert.False(t, stillRunning)
	})

	t.Run("still running is not an error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, &api.HTTPError{StatusCode: 403, Message: "Run is already running", RequestURL: &url.URL{}})

		stillRunning, err := RerunFailedJobs(context.Background(), client, "cloudposse/atmos", "123")
		require.NoError(t, err)
		assert.True(t, stillRunning)
	})

	t.Run("in progress phrasing also counts as still running", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, &api.HTTPError{StatusCode: 422, Message: "workflow run is in progress", RequestURL: &url.URL{}})

		stillRunning, err := RerunFailedJobs(context.Background(), client, "cloudposse/atmos", "123")
		require.NoError(t, err)
		assert.True(t, stillRunning)
	})

	t.Run("a genuine API error is returned", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, &api.HTTPError{StatusCode: 404, Message: "Not Found", RequestURL: &url.URL{}})

		stillRunning, err := RerunFailedJobs(context.Background(), client, "cloudposse/atmos", "123")
		require.Error(t, err)
		assert.False(t, stillRunning)
		assert.Contains(t, err.Error(), "Not Found")
	})

	t.Run("a non-HTTP error is returned", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("connection reset"))

		stillRunning, err := RerunFailedJobs(context.Background(), client, "cloudposse/atmos", "123")
		require.Error(t, err)
		assert.False(t, stillRunning)
	})
}
