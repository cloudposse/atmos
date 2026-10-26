package rerun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
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
			RequestWithContext(gomock.Any(), http.MethodGet, "repos/cloudposse/atmos/actions/runs/1/attempts/1/jobs?per_page=30", nil).
			Return(jsonResponse(200, `{"jobs":[{"name":"Build","status":"completed","conclusion":"failure"}]}`, nil), nil) //nolint:bodyclose // closed by the code under test, not this fixture.

		jobs, err := FetchJobs(context.Background(), client, "cloudposse/atmos", "1", "1")
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		assert.Equal(t, "Build", jobs[0].Name)
	})

	t.Run("follows the Link header across pages", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		nextURL := "https://api.github.com/repos/cloudposse/atmos/actions/runs/1/attempts/1/jobs?per_page=30&page=2"

		gomock.InOrder(
			client.EXPECT().
				RequestWithContext(gomock.Any(), http.MethodGet, "repos/cloudposse/atmos/actions/runs/1/attempts/1/jobs?per_page=30", nil).
				Return(jsonResponse(200, `{"jobs":[{"name":"Build (linux)","status":"completed","conclusion":"success"}]}`, //nolint:bodyclose // closed by the code under test, not this fixture.
					http.Header{"Link": {`<` + nextURL + `>; rel="next", <https://api.github.com/x?page=2>; rel="last"`}}), nil),
			client.EXPECT().
				RequestWithContext(gomock.Any(), http.MethodGet, nextURL, nil).
				Return(jsonResponse(200, `{"jobs":[{"name":"Build (windows)","status":"completed","conclusion":"cancelled"}]}`, nil), nil), //nolint:bodyclose // closed by the code under test, not this fixture.
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
			Return(jsonResponse(200, `{"jobs":[`, nil), nil) //nolint:bodyclose // closed by the code under test, not this fixture.

		_, err := FetchJobs(context.Background(), client, "cloudposse/atmos", "1", "1")
		require.Error(t, err)
	})
}

func TestFetchJobsFallsBackToSmallerPagesOnServerErrors(t *testing.T) {
	t.Parallel()

	const base = "repos/cloudposse/atmos/actions/runs/1/attempts/2/jobs?per_page="
	badGateway := &api.HTTPError{StatusCode: http.StatusBadGateway}

	t.Run("a 502 at 30 per page retries at 15", func(t *testing.T) {
		t.Parallel()
		client := NewMockRESTClient(gomock.NewController(t))
		gomock.InOrder(
			client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, base+"30", nil).Return(nil, badGateway),
			client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, base+"15", nil).
				Return(jsonResponse(200, `{"jobs":[{"name":"Build","status":"completed","conclusion":"success"}]}`, nil), nil), //nolint:bodyclose // closed by the code under test, not this fixture.
		)

		jobs, err := FetchJobs(context.Background(), client, "cloudposse/atmos", "1", "2")
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		assert.Equal(t, "Build", jobs[0].Name)
	})

	t.Run("a 502 on a later page restarts from the first page", func(t *testing.T) {
		t.Parallel()
		client := NewMockRESTClient(gomock.NewController(t))
		page2 := "https://api.github.com/" + base + "30&page=2"
		gomock.InOrder(
			client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, base+"30", nil).
				Return(jsonResponse(200, `{"jobs":[{"name":"Build (linux)","status":"completed","conclusion":"success"}]}`, //nolint:bodyclose // closed by the code under test, not this fixture.
					http.Header{"Link": {`<` + page2 + `>; rel="next"`}}), nil),
			client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, page2, nil).Return(nil, badGateway),
			client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, base+"15", nil).
				Return(jsonResponse(200, `{"jobs":[{"name":"Build (linux)","status":"completed","conclusion":"success"},{"name":"Build (windows)","status":"completed","conclusion":"success"}]}`, nil), nil), //nolint:bodyclose // closed by the code under test, not this fixture.
		)

		jobs, err := FetchJobs(context.Background(), client, "cloudposse/atmos", "1", "2")
		require.NoError(t, err)
		require.Len(t, jobs, 2, "the first page fetched at 30 must not be kept alongside the restart")
		assert.Equal(t, "Build (linux)", jobs[0].Name)
		assert.Equal(t, "Build (windows)", jobs[1].Name)
	})

	t.Run("a server error at every size returns the last error", func(t *testing.T) {
		t.Parallel()
		client := NewMockRESTClient(gomock.NewController(t))
		gomock.InOrder(
			client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, base+"30", nil).Return(nil, badGateway),
			client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, base+"15", nil).Return(nil, badGateway),
			client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, base+"10", nil).Return(nil, &api.HTTPError{StatusCode: http.StatusServiceUnavailable}),
		)

		_, err := FetchJobs(context.Background(), client, "cloudposse/atmos", "1", "2")
		var httpErr *api.HTTPError
		require.ErrorAs(t, err, &httpErr)
		assert.Equal(t, http.StatusServiceUnavailable, httpErr.StatusCode)
		assert.True(t, IsTransient(err), "the caller's poll loop must still treat it as retryable")
	})

	t.Run("a 4xx is not retried at a smaller size", func(t *testing.T) {
		t.Parallel()
		client := NewMockRESTClient(gomock.NewController(t))
		client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, base+"30", nil).Return(nil, &api.HTTPError{StatusCode: http.StatusNotFound})

		_, err := FetchJobs(context.Background(), client, "cloudposse/atmos", "1", "2")
		var httpErr *api.HTTPError
		require.ErrorAs(t, err, &httpErr)
		assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	})

	t.Run("a cancelled context is not retried at a smaller size", func(t *testing.T) {
		t.Parallel()
		client := NewMockRESTClient(gomock.NewController(t))
		client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, base+"30", nil).Return(nil, context.Canceled)

		_, err := FetchJobs(context.Background(), client, "cloudposse/atmos", "1", "2")
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestIsTransient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "5xx from the API", err: &api.HTTPError{StatusCode: http.StatusBadGateway}, want: true},
		{name: "wrapped 5xx", err: fmt.Errorf("rerun: fetch jobs: %w", &api.HTTPError{StatusCode: http.StatusServiceUnavailable}), want: true},
		{name: "4xx from the API", err: &api.HTTPError{StatusCode: http.StatusNotFound}, want: false},
		{name: "transport error", err: assert.AnError, want: true},
		{name: "cancelled context", err: fmt.Errorf("wrapped: %w", context.Canceled), want: false},
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsTransient(tt.err))
		})
	}
}

func TestNextLink(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", nextLink(""))
	assert.Equal(t, "", nextLink(`<https://api.github.com/x?page=2>; rel="last"`))
	assert.Equal(t, "https://api.github.com/x?page=2",
		nextLink(`<https://api.github.com/x?page=2>; rel="next", <https://api.github.com/x?page=9>; rel="last"`))
}

// testLookupErrors runs the two error-path subtests shared by every single-
// request/JSON-decode lookup (PRHeadSHA, RunAttempt): a transport error and
// a malformed response body must both surface as errors.
func testLookupErrors[T any](t *testing.T, call func(client RESTClient) (T, error)) {
	t.Helper()

	t.Run("request error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("not found"))

		_, err := call(client)
		require.Error(t, err)
	})

	t.Run("malformed body is an error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(jsonResponse(200, `{`, nil), nil) //nolint:bodyclose // closed by the code under test, not this fixture.

		_, err := call(client)
		require.Error(t, err)
	})
}

func TestPRHeadSHA(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().
			RequestWithContext(gomock.Any(), http.MethodGet, "repos/cloudposse/atmos/pulls/42", nil).
			Return(jsonResponse(200, `{"head":{"sha":"deadbeef"}}`, nil), nil) //nolint:bodyclose // closed by the code under test, not this fixture.

		sha, err := PRHeadSHA(context.Background(), client, "cloudposse/atmos", 42)
		require.NoError(t, err)
		assert.Equal(t, "deadbeef", sha)
	})

	testLookupErrors(t, func(client RESTClient) (string, error) {
		return PRHeadSHA(context.Background(), client, "cloudposse/atmos", 42)
	})
}

func TestRunAttempt(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := NewMockRESTClient(ctrl)
		client.EXPECT().
			RequestWithContext(gomock.Any(), http.MethodGet, "repos/cloudposse/atmos/actions/runs/123", nil).
			Return(jsonResponse(200, `{"run_attempt":2}`, nil), nil) //nolint:bodyclose // closed by the code under test, not this fixture.

		attempt, err := RunAttempt(context.Background(), client, "cloudposse/atmos", "123")
		require.NoError(t, err)
		assert.Equal(t, 2, attempt)
	})

	for _, body := range []string{`{}`, `{"run_attempt":null}`, `{"run_attempt":0}`} {
		t.Run("body "+body+" is an error", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			client := NewMockRESTClient(ctrl)
			client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(jsonResponse(200, body, nil), nil) //nolint:bodyclose // closed by the code under test, not this fixture.

			_, err := RunAttempt(context.Background(), client, "cloudposse/atmos", "123")
			require.ErrorIs(t, err, errInvalidRunAttempt)
		})
	}

	testLookupErrors(t, func(client RESTClient) (int, error) {
		return RunAttempt(context.Background(), client, "cloudposse/atmos", "123")
	})
}
