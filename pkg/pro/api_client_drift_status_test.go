package pro

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUploadDriftResultStatus(t *testing.T) {
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

	dto := DriftStatusUploadRequest{}

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err = apiClient.UploadDriftResultStatus(&dto)
	assert.NoError(t, err)

	mockRoundTripper.AssertExpectations(t)
}

func TestUploadDriftResultStatus_Error(t *testing.T) {
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

	dto := DriftStatusUploadRequest{}

	mockResponse := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err = apiClient.UploadDriftResultStatus(&dto)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload drift status")

	mockRoundTripper.AssertExpectations(t)
}
