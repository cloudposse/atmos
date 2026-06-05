package pro

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

func TestCreateCommit_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/git/commit", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req dtos.CommitRequest
		err = json.Unmarshal(body, &req)
		require.NoError(t, err)

		assert.Equal(t, "feature/test", req.Branch)
		assert.Equal(t, "test commit", req.CommitMessage)
		assert.Len(t, req.Changes.Additions, 1)
		assert.Equal(t, "main.tf", req.Changes.Additions[0].Path)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"success": true,
			"status": 200,
			"data": { "sha": "abc123def456" }
		}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      http.DefaultClient,
	}

	dto := &dtos.CommitRequest{
		Branch: "feature/test",
		Changes: dtos.CommitChanges{
			Additions: []dtos.CommitFileAddition{
				{Path: "main.tf", Contents: "dGVycmFmb3Jt"},
			},
		},
		CommitMessage: "test commit",
	}

	resp, err := client.CreateCommit(dto)
	require.NoError(t, err)
	assert.Equal(t, "abc123def456", resp.Data.SHA)
}

func TestCreateCommit_NilDTO(t *testing.T) {
	client := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      http.DefaultClient,
	}

	resp, err := client.CreateCommit(nil)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToCreateCommit)
	assert.ErrorIs(t, err, errUtils.ErrNilRequestDTO)
}

func TestCreateCommit_HTTPErrors(t *testing.T) {
	testCases := []struct {
		name         string
		statusCode   int
		responseBody string
	}{
		{
			name:         "400 bad request",
			statusCode:   http.StatusBadRequest,
			responseBody: `{"success": false, "errorMessage": "validation failed"}`,
		},
		{
			name:         "401 unauthorized",
			statusCode:   http.StatusUnauthorized,
			responseBody: `{"success": false, "errorMessage": "authentication required"}`,
		},
		{
			name:         "403 forbidden",
			statusCode:   http.StatusForbidden,
			responseBody: `{"success": false, "errorMessage": "missing permission"}`,
		},
		{
			name:         "404 not found",
			statusCode:   http.StatusNotFound,
			responseBody: `{"success": false, "errorMessage": "no GitHub App installation"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statusCode)
				w.Write([]byte(tc.responseBody))
			}))
			defer server.Close()

			client := &AtmosProAPIClient{
				BaseURL:         server.URL,
				BaseAPIEndpoint: "api",
				APIToken:        "test-token",
				HTTPClient:      http.DefaultClient,
			}

			dto := &dtos.CommitRequest{
				Branch:        "feature/test",
				Changes:       dtos.CommitChanges{},
				CommitMessage: "test",
			}

			resp, err := client.CreateCommit(dto)
			assert.Nil(t, resp)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrFailedToCreateCommit)
		})
	}
}

func TestCreateCommit_NetworkError(t *testing.T) {
	client := &AtmosProAPIClient{
		BaseURL:         "http://localhost:1",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      http.DefaultClient,
	}

	dto := &dtos.CommitRequest{
		Branch:        "feature/test",
		Changes:       dtos.CommitChanges{},
		CommitMessage: "test",
	}

	resp, err := client.CreateCommit(dto)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToCreateCommit)
}

// TestSendCommitRequest_RedirectErrorClosesBody triggers the rare
// "non-nil response with non-nil error" return path of http.Client.Do by
// using a CheckRedirect that returns an error. This exercises the defensive
// resp.Body.Close() guard added to avoid leaking connections.
func TestSendCommitRequest_RedirectErrorClosesBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"/loop", http.StatusFound)
	}))
	defer server.Close()

	httpClient := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errRedirectBlocked
		},
	}

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	var resp dtos.CommitResponse
	err := client.sendCommitRequest(server.URL+"/api/git/commit", []byte(`{}`), &resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToMakeRequest)
}

// errRedirectBlocked is used by tests that need CheckRedirect to fail so
// http.Client.Do returns a non-nil response together with the error.
var errRedirectBlocked = errSentinel("redirect blocked")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

func TestSendCommitRequest_NilHTTPClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true, "data": {"sha": "nil-client-test"}}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      nil, // Nil client should trigger fallback.
	}

	var resp dtos.CommitResponse
	err := client.sendCommitRequest(server.URL+"/api/git/commit", []byte(`{}`), &resp)
	require.NoError(t, err)
	assert.Equal(t, "nil-client-test", resp.Data.SHA)
}

func TestSendCommitRequest_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not valid json`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      http.DefaultClient,
	}

	var resp dtos.CommitResponse
	err := client.sendCommitRequest(server.URL+"/api/git/commit", []byte(`{}`), &resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToUnmarshalAPIResponse)
}

func TestBuildCommitAPIError_UnparsableBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
	}

	err := buildCommitAPIError(resp, []byte(`not json`))
	require.Error(t, err)

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Equal(t, operationCreateCommit, apiErr.Operation)
}

func TestBuildCommitAPIError_ParseableBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Status:     "403 Forbidden",
	}

	body := []byte(`{"success": false, "errorMessage": "missing permission"}`)
	err := buildCommitAPIError(resp, body)
	require.Error(t, err)

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	assert.Equal(t, operationCreateCommit, apiErr.Operation)
}
