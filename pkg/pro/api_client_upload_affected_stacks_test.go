package pro

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestUploadAffectedStacks_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/affected-stacks", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.NoError(t, err)

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      http.DefaultClient,
		Logger:          mockLogger,
	}

	dto := dtos.UploadAffectedStacksRequest{
		HeadSHA:   "test-head-sha",
		BaseSHA:   "test-base-sha",
		RepoURL:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
		Stacks: []schema.Affected{
			{
				Component:     "test-component",
				ComponentType: "terraform",
				Stack:         "test-stack",
				Affected:      "stack.vars",
			},
		},
	}

	err = client.UploadAffectedStacks(&dto)
	assert.NoError(t, err)
}

func TestUploadAffectedStacks_HTTPErrors(t *testing.T) {
	testCases := []struct {
		name          string
		statusCode    int
		expectedError error
	}{
		{
			name:          "server returns 400 bad request",
			statusCode:    http.StatusBadRequest,
			expectedError: ErrFailedToUploadStacks,
		},
		{
			name:          "server returns 401 unauthorized",
			statusCode:    http.StatusUnauthorized,
			expectedError: ErrFailedToUploadStacks,
		},
		{
			name:          "server returns 403 forbidden",
			statusCode:    http.StatusForbidden,
			expectedError: ErrFailedToUploadStacks,
		},
		{
			name:          "server returns 500 internal server error",
			statusCode:    http.StatusInternalServerError,
			expectedError: ErrFailedToUploadStacks,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				w.Write([]byte(`{"error": "server error"}`))
			}))
			defer server.Close()

			mockLogger, err := logger.NewLogger("test", "/dev/stdout")
			assert.NoError(t, err)

			client := &AtmosProAPIClient{
				BaseURL:         server.URL,
				BaseAPIEndpoint: "api",
				APIToken:        "test-token",
				HTTPClient:      http.DefaultClient,
				Logger:          mockLogger,
			}

			dto := dtos.UploadAffectedStacksRequest{
				HeadSHA: "test-head-sha",
				BaseSHA: "test-base-sha",
				Stacks:  []schema.Affected{},
			}

			err = client.UploadAffectedStacks(&dto)
			assert.Error(t, err)
			assert.ErrorIs(t, err, tc.expectedError)
		})
	}
}

func TestUploadAffectedStacks_NetworkError(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.NoError(t, err)

	client := &AtmosProAPIClient{
		BaseURL:         "http://invalid-host-that-does-not-exist:12345",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      http.DefaultClient,
		Logger:          mockLogger,
	}

	dto := dtos.UploadAffectedStacksRequest{
		HeadSHA: "test-head-sha",
		BaseSHA: "test-base-sha",
		Stacks:  []schema.Affected{},
	}

	err = client.UploadAffectedStacks(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrFailedToMakeRequest)
}

func TestUploadAffectedStacks_RequestCreationError(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.NoError(t, err)

	// Use an invalid URL that would cause http.NewRequest to fail
	client := &AtmosProAPIClient{
		BaseURL:         "://invalid-url", // Malformed URL
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      http.DefaultClient,
		Logger:          mockLogger,
	}

	dto := dtos.UploadAffectedStacksRequest{
		HeadSHA: "test-head-sha",
		BaseSHA: "test-base-sha",
		Stacks:  []schema.Affected{},
	}

	err = client.UploadAffectedStacks(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrFailedToCreateAuthRequest)
}
