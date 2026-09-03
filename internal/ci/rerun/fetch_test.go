package rerun

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func jsonResponse(status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     headers,
	}
}

func TestFetchJobs(t *testing.T) {
	t.Parallel()

	t.Run("single page", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().
			RequestWithContext(gomock.Any(), http.MethodGet, "repos/cloudposse/atmos/actions/runs/1/attempts/1/jobs?per_page=100", nil).
			Return(jsonResponse(200, `{"jobs":[{"name":"Build","status":"completed","conclusion":"failure"}]}`, nil), nil) //nolint:bodyclose // closed by the code under test, not this fixture

		jobs, err := FetchJobs(context.Background(), client, "cloudposse/atmos", "1", "1")
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		assert.Equal(t, "Build", jobs[0].Name)
	})

	t.Run("follows the Link header across pages", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		nextURL := "https://api.github.com/repos/cloudposse/atmos/actions/runs/1/attempts/1/jobs?per_page=100&page=2"

		gomock.InOrder(
			client.EXPECT().
				RequestWithContext(gomock.Any(), http.MethodGet, "repos/cloudposse/atmos/actions/runs/1/attempts/1/jobs?per_page=100", nil).
				Return(jsonResponse(200, `{"jobs":[{"name":"Build (linux)","status":"completed","conclusion":"success"}]}`, //nolint:bodyclose // closed by the code under test, not this fixture
					http.Header{"Link": {`<` + nextURL + `>; rel="next", <https://api.github.com/x?page=2>; rel="last"`}}), nil),
			client.EXPECT().
				RequestWithContext(gomock.Any(), http.MethodGet, nextURL, nil).
				Return(jsonResponse(200, `{"jobs":[{"name":"Build (windows)","status":"completed","conclusion":"cancelled"}]}`, nil), nil), //nolint:bodyclose // closed by the code under test, not this fixture
		)

		jobs, err := FetchJobs(context.Background(), client, "cloudposse/atmos", "1", "1")
		require.NoError(t, err)
		require.Len(t, jobs, 2)
		assert.Equal(t, "Build (linux)", jobs[0].Name)
		assert.Equal(t, "Build (windows)", jobs[1].Name)
	})

	t.Run("request error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("boom"))

		_, err := FetchJobs(context.Background(), client, "cloudposse/atmos", "1", "1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("malformed page is an error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(jsonResponse(200, `{"jobs":[`, nil), nil) //nolint:bodyclose // closed by the code under test, not this fixture

		_, err := FetchJobs(context.Background(), client, "cloudposse/atmos", "1", "1")
		require.Error(t, err)
	})
}

func TestNextLink(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", nextLink(""))
	assert.Equal(t, "", nextLink(`<https://api.github.com/x?page=2>; rel="last"`))
	assert.Equal(t, "https://api.github.com/x?page=2",
		nextLink(`<https://api.github.com/x?page=2>; rel="next", <https://api.github.com/x?page=9>; rel="last"`))
}

func TestPRHeadSHA(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().
			RequestWithContext(gomock.Any(), http.MethodGet, "repos/cloudposse/atmos/pulls/42", nil).
			Return(jsonResponse(200, `{"head":{"sha":"deadbeef"}}`, nil), nil) //nolint:bodyclose // closed by the code under test, not this fixture

		sha, err := PRHeadSHA(context.Background(), client, "cloudposse/atmos", 42)
		require.NoError(t, err)
		assert.Equal(t, "deadbeef", sha)
	})

	t.Run("request error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("not found"))

		_, err := PRHeadSHA(context.Background(), client, "cloudposse/atmos", 42)
		require.Error(t, err)
	})

	t.Run("malformed body is an error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(jsonResponse(200, `{`, nil), nil) //nolint:bodyclose // closed by the code under test, not this fixture

		_, err := PRHeadSHA(context.Background(), client, "cloudposse/atmos", 42)
		require.Error(t, err)
	})
}
