package releasenotes

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetPullRequestBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)
	client.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "https://api.github.com/repos/cloudposse/atmos/pulls/42", req.URL.String())
		assert.Equal(t, "Bearer gh-token", req.Header.Get("Authorization"))
		assert.Equal(t, "application/vnd.github+json", req.Header.Get("Accept"))
		return jsonResponse(http.StatusOK, `{"number":42,"body":"## what\n\nDoes x."}`), nil
	})

	got, err := GetPullRequestBody(context.Background(), client, "gh-token", "cloudposse/atmos", 42)
	require.NoError(t, err)
	assert.Equal(t, "## what\n\nDoes x.", got)
}

func TestGetPullRequestBody_Errors(t *testing.T) {
	tests := []struct {
		name    string
		resp    *http.Response
		doErr   error
		wantErr string
	}{
		{name: "transport error", doErr: assert.AnError, wantErr: "get PR #42"},
		{name: "not found", resp: jsonResponse(http.StatusNotFound, `{"message":"Not Found"}`), wantErr: "PR #42 returned"}, //nolint:bodyclose // closed by the code under test, not this fixture.
		{name: "body read fails", resp: brokenBodyResponse(http.StatusOK), wantErr: "read PR #42"},                          //nolint:bodyclose // closed by the code under test, not this fixture.
		{name: "bad json", resp: jsonResponse(http.StatusOK, `nope`), wantErr: "decode PR #42"},                             //nolint:bodyclose // closed by the code under test, not this fixture.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockHTTPClient(ctrl)
			client.EXPECT().Do(gomock.Any()).Return(tt.resp, tt.doErr)

			_, err := GetPullRequestBody(context.Background(), client, "gh-token", "cloudposse/atmos", 42)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
