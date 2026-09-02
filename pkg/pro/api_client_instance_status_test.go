package pro

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUploadInstanceStatus(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.InstanceStatusUploadRequest{}

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err := apiClient.UploadInstanceStatus(&dto)
	assert.NoError(t, err)

	mockRoundTripper.AssertExpectations(t)
}

func TestUploadInstanceStatus_WithCIData(t *testing.T) {
	t.Run("includes metadata and component_type in payload when set", func(t *testing.T) {
		mockRoundTripper := new(MockRoundTripper)
		httpClient := &http.Client{Transport: mockRoundTripper}
		apiClient := &AtmosProAPIClient{
			BaseURL:         "http://localhost",
			BaseAPIEndpoint: "api",
			APIToken:        "test-token",
			HTTPClient:      httpClient,
		}

		dto := dtos.InstanceStatusUploadRequest{
			Command:       "plan",
			ExitCode:      2,
			ComponentType: "terraform",
			Metadata: map[string]any{
				"has_changes": true,
				"resource_counts": map[string]int{
					"create": 3, "change": 1, "replace": 0, "destroy": 0,
				},
			},
		}

		mockResponse := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
		}

		mockRoundTripper.On("RoundTrip", mock.MatchedBy(func(req *http.Request) bool {
			body, _ := io.ReadAll(req.Body)
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				return false
			}
			// Verify component_type is a top-level field.
			if payload["component_type"] != "terraform" {
				return false
			}
			// Verify metadata block is present.
			md, ok := payload["metadata"].(map[string]any)
			if !ok {
				return false
			}
			return md["has_changes"] == true
		})).Return(mockResponse, nil)

		err := apiClient.UploadInstanceStatus(&dto)
		assert.NoError(t, err)
		mockRoundTripper.AssertExpectations(t)
	})

	t.Run("omits metadata and component_type from payload when not set", func(t *testing.T) {
		mockRoundTripper := new(MockRoundTripper)
		httpClient := &http.Client{Transport: mockRoundTripper}
		apiClient := &AtmosProAPIClient{
			BaseURL:         "http://localhost",
			BaseAPIEndpoint: "api",
			APIToken:        "test-token",
			HTTPClient:      httpClient,
		}

		dto := dtos.InstanceStatusUploadRequest{
			Command:  "plan",
			ExitCode: 0,
		}

		mockResponse := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
		}

		mockRoundTripper.On("RoundTrip", mock.MatchedBy(func(req *http.Request) bool {
			body, _ := io.ReadAll(req.Body)
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				return false
			}
			_, hasMetadata := payload["metadata"]
			_, hasComponentType := payload["component_type"]
			return !hasMetadata && !hasComponentType
		})).Return(mockResponse, nil)

		err := apiClient.UploadInstanceStatus(&dto)
		assert.NoError(t, err)
		mockRoundTripper.AssertExpectations(t)
	})
}

func TestUploadInstanceStatus_Error(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.InstanceStatusUploadRequest{}

	mockResponse := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err := apiClient.UploadInstanceStatus(&dto)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload instance status")

	mockRoundTripper.AssertExpectations(t)
}
