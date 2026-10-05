package releasenotes

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestGetRelease(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)
	client.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "https://api.github.com/repos/cloudposse/atmos/releases/123", req.URL.String())
		assert.Equal(t, "Bearer gh-token", req.Header.Get("Authorization"))
		return jsonResponse(http.StatusOK, `{"body":"the body","tag_name":"v1.230.1"}`), nil
	})

	got, err := GetRelease(context.Background(), client, "gh-token", ReleaseRef{Repo: "cloudposse/atmos", ID: "123"})
	require.NoError(t, err)
	assert.Equal(t, ReleaseContent{Body: "the body", TagName: "v1.230.1"}, got)
}

func TestGetRelease_Errors(t *testing.T) {
	tests := []struct {
		name    string
		resp    *http.Response
		doErr   error
		wantErr string
	}{
		{name: "transport error", doErr: assert.AnError, wantErr: "get release"},
		{name: "non-200", resp: jsonResponse(http.StatusNotFound, `{"message":"not found"}`), wantErr: "returned"}, //nolint:bodyclose // closed by the code under test, not this fixture.
		{name: "invalid json", resp: jsonResponse(http.StatusOK, `not json`), wantErr: "decode release"},           //nolint:bodyclose // closed by the code under test, not this fixture.
		{name: "response body read fails", resp: brokenBodyResponse(http.StatusOK), wantErr: "read release"},       //nolint:bodyclose // closed by the code under test, not this fixture.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockHTTPClient(ctrl)
			client.EXPECT().Do(gomock.Any()).Return(tt.resp, tt.doErr)

			_, err := GetRelease(context.Background(), client, "gh-token", ReleaseRef{Repo: "cloudposse/atmos", ID: "123"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestUpdateReleaseBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)
	client.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodPatch, req.Method)
		assert.Equal(t, "https://api.github.com/repos/cloudposse/atmos/releases/123", req.URL.String())
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"body":"new body","tag_name":"v1.230.1"}`, string(body))
		return jsonResponse(http.StatusOK, `{"tag_name":"v1.230.1"}`), nil
	})

	err := UpdateReleaseBody(context.Background(), client, "gh-token", ReleaseRef{Repo: "cloudposse/atmos", ID: "123"}, ReleaseContent{Body: "new body", TagName: "v1.230.1"})
	require.NoError(t, err)
}

func TestUpdateReleaseBody_Errors(t *testing.T) {
	tests := []struct {
		name    string
		resp    *http.Response
		doErr   error
		wantErr string
	}{
		{name: "transport error", doErr: assert.AnError, wantErr: "update release"},
		{name: "non-200", resp: jsonResponse(http.StatusUnprocessableEntity, `{"message":"body is too long"}`), wantErr: "returned"}, //nolint:bodyclose // closed by the code under test, not this fixture.
		{name: "invalid json", resp: jsonResponse(http.StatusOK, `not json`), wantErr: "decode release"},                             //nolint:bodyclose // closed by the code under test.
		{name: "response body read fails", resp: brokenBodyResponse(http.StatusOK), wantErr: "decode release"},                       //nolint:bodyclose // closed by the code under test.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockHTTPClient(ctrl)
			client.EXPECT().Do(gomock.Any()).Return(tt.resp, tt.doErr)

			err := UpdateReleaseBody(context.Background(), client, "gh-token", ReleaseRef{Repo: "cloudposse/atmos", ID: "123"}, ReleaseContent{Body: "body", TagName: "v1.230.1"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestUpdateReleaseBody_RejectsMissingTagBeforeRequest(t *testing.T) {
	for _, tag := range []string{"", " \t"} {
		t.Run(tag, func(t *testing.T) {
			client := NewMockHTTPClient(gomock.NewController(t)) // No HTTP requests expected.
			err := UpdateReleaseBody(context.Background(), client, "gh-token", ReleaseRef{Repo: "cloudposse/atmos", ID: "123"}, ReleaseContent{Body: "body", TagName: tag})
			require.ErrorIs(t, err, errUtils.ErrReleaseTagMissing)
		})
	}
}

func TestUpdateReleaseBody_RejectsChangedOrMissingResponseTag(t *testing.T) {
	for _, response := range []string{
		`{"tag_name":"untagged-ce54d2b635a2ba2ebf07"}`,
		`{"tag_name":"v1.230.2"}`,
		`{"tag_name":""}`,
		`{}`,
	} {
		t.Run(response, func(t *testing.T) {
			client := NewMockHTTPClient(gomock.NewController(t))
			client.EXPECT().Do(gomock.Any()).Return(jsonResponse(http.StatusOK, response), nil) //nolint:bodyclose // closed by UpdateReleaseBody.

			err := UpdateReleaseBody(context.Background(), client, "gh-token", ReleaseRef{Repo: "cloudposse/atmos", ID: "123"}, ReleaseContent{Body: "body", TagName: "v1.230.1"})
			require.ErrorIs(t, err, errUtils.ErrReleaseTagMismatch)
			assert.Contains(t, err.Error(), `expected "v1.230.1"`)
		})
	}
}
