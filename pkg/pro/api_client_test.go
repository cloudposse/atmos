package pro

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// MockRoundTripper is an implementation of http.RoundTripper for testing purposes.
type MockRoundTripper struct {
	mock.Mock
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

func TestLockStack(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.Nil(t, err)

	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		Logger:          mockLogger,
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := LockStackRequest{ /* populate fields */ }

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	response, err := apiClient.LockStack(dto)
	assert.NoError(t, err)
	assert.True(t, response.Success)

	mockRoundTripper.AssertExpectations(t)
}

func TestLockStack_Error(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.Nil(t, err)

	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		Logger:          mockLogger,
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := LockStackRequest{ /* populate fields */ }

	mockResponse := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": false, "errorMessage": "Internal Server Error"}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	response, err := apiClient.LockStack(dto)
	assert.Error(t, err)
	assert.False(t, response.Success)
	assert.Contains(t, err.Error(), "Internal Server Error")

	mockRoundTripper.AssertExpectations(t)
}

func TestUnlockStack(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.Nil(t, err)

	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		Logger:          mockLogger,
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := UnlockStackRequest{}

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	response, err := apiClient.UnlockStack(dto)
	assert.NoError(t, err)
	assert.True(t, response.Success)

	mockRoundTripper.AssertExpectations(t)
}

func TestUnlockStack_Error(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.Nil(t, err)

	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		Logger:          mockLogger,
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := UnlockStackRequest{}

	mockResponse := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString(`{"request":"1", "success": false, "errorMessage": "Internal Server Error"}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	response, err := apiClient.UnlockStack(dto)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Internal Server Error")
	assert.False(t, response.Success)

	mockRoundTripper.AssertExpectations(t)
}

func TestUploadDriftDetection(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.Nil(t, err)

	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		Logger:          mockLogger,
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := DriftDetectionUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		Stacks: []schema.Deployment{
			{
				Component:     "vpc",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
					},
				},
				Vars: map[string]any{
					"environment": "dev",
					"tenant":      "tenant1",
					"region":      "ue2",
					"cidr_block":  "10.0.0.0/16",
				},
			},
			{
				Component:     "eks",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
					},
				},
				Vars: map[string]any{
					"environment":        "dev",
					"tenant":             "tenant1",
					"region":             "ue2",
					"cluster_name":       "tenant1-ue2-dev",
					"kubernetes_version": "1.27",
				},
			},
		},
	}

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err = apiClient.UploadDriftDetection(&dto)
	assert.NoError(t, err)

	mockRoundTripper.AssertExpectations(t)
}

func TestUploadDriftDetection_Error(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.Nil(t, err)

	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		Logger:          mockLogger,
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := DriftDetectionUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		Stacks: []schema.Deployment{
			{
				Component:     "vpc",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
					},
				},
				Vars: map[string]any{
					"environment": "dev",
					"tenant":      "tenant1",
					"region":      "ue2",
					"cidr_block":  "10.0.0.0/16",
				},
			},
			{
				Component:     "eks",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
					},
				},
				Vars: map[string]any{
					"environment":        "dev",
					"tenant":             "tenant1",
					"region":             "ue2",
					"cluster_name":       "tenant1-ue2-dev",
					"kubernetes_version": "1.27",
				},
			},
		},
	}

	mockResponse := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": false, "errorMessage": "Internal Server Error"}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err = apiClient.UploadDriftDetection(&dto)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload drift detection results")

	mockRoundTripper.AssertExpectations(t)
}
