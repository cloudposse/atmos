package releasenotes

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetReleaseBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)
	client.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "https://api.github.com/repos/cloudposse/atmos/releases/123", req.URL.String())
		assert.Equal(t, "Bearer gh-token", req.Header.Get("Authorization"))
		return jsonResponse(http.StatusOK, `{"body":"the body"}`), nil
	})

	got, err := GetReleaseBody(context.Background(), client, "gh-token", ReleaseRef{Repo: "cloudposse/atmos", ID: "123"})
	require.NoError(t, err)
	assert.Equal(t, "the body", got)
}

func TestGetReleaseBody_Errors(t *testing.T) {
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

			_, err := GetReleaseBody(context.Background(), client, "gh-token", ReleaseRef{Repo: "cloudposse/atmos", ID: "123"})
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
		assert.JSONEq(t, `{"body":"new body"}`, string(body))
		return jsonResponse(http.StatusOK, `{}`), nil
	})

	err := UpdateReleaseBody(context.Background(), client, "gh-token", ReleaseRef{Repo: "cloudposse/atmos", ID: "123"}, "new body")
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockHTTPClient(ctrl)
			client.EXPECT().Do(gomock.Any()).Return(tt.resp, tt.doErr)

			err := UpdateReleaseBody(context.Background(), client, "gh-token", ReleaseRef{Repo: "cloudposse/atmos", ID: "123"}, "body")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
