package pro

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

func TestUploadInstances(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.InstancesUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		Instances: []dtos.UploadInstance{
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
			},
		},
	}

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err := apiClient.UploadInstances(&dto)
	assert.NoError(t, err)

	mockRoundTripper.AssertExpectations(t)
}

func TestUploadInstances_Error(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.InstancesUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		Instances: []dtos.UploadInstance{
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
			},
		},
	}

	mockResponse := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": false, "errorMessage": "Internal Server Error"}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err := apiClient.UploadInstances(&dto)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToUploadInstances))

	mockRoundTripper.AssertExpectations(t)
}
