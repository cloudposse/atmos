package pro

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

func newTestSecurityFindingsRequest() *dtos.SecurityFindingsUploadRequest {
	return &dtos.SecurityFindingsUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		GitSHA:    "deadbeef",
		Stack:     "tenant1-ue1-prod",
		Component: "vpc",
		Format:    "sarif",
		SARIF:     json.RawMessage(`{"version":"2.1.0","runs":[]}`),
	}
}

func TestUploadSecurityFindings_Success(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api/v1",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.MatchedBy(func(req *http.Request) bool {
		if req.Method != http.MethodPost {
			return false
		}
		if req.URL.String() != "http://localhost/api/v1/security-findings" {
			return false
		}
		if req.Header.Get("Authorization") != "Bearer test-token" {
			return false
		}
		if req.Header.Get("Content-Type") != "application/json" {
			return false
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return false
		}
		var decoded dtos.SecurityFindingsUploadRequest
		if err := json.Unmarshal(body, &decoded); err != nil {
			return false
		}
		return decoded.Format == "sarif" && decoded.RepoOwner == "org" && decoded.GitSHA == "deadbeef"
	})).Return(mockResponse, nil)

	err := apiClient.UploadSecurityFindings(newTestSecurityFindingsRequest())
	require.NoError(t, err)
	mockRoundTripper.AssertExpectations(t)
}

func TestUploadSecurityFindings_NilDTO(t *testing.T) {
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api/v1",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{},
	}
	err := apiClient.UploadSecurityFindings(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAWSSecurityUploadFailed))
	assert.True(t, errors.Is(err, errUtils.ErrNilRequestDTO))
}

func TestUploadSecurityFindings_ServerError(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api/v1",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	mockResponse := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": false, "errorMessage": "bad sarif"}`)),
	}

	// 400 is non-retryable — RoundTrip is called exactly once.
	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil).Once()

	err := apiClient.UploadSecurityFindings(newTestSecurityFindingsRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAWSSecurityUploadFailed))
	mockRoundTripper.AssertExpectations(t)
}

func TestUploadSecurityFindings_TransportError(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api/v1",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	// Transport-level failure surfaces through retry; the default retry config
	// retries transient errors several times. Always return the same error so the
	// final wrapped error is deterministic.
	mockRoundTripper.On("RoundTrip", mock.Anything).Return((*http.Response)(nil), errors.New("dial tcp: connection refused"))

	err := apiClient.UploadSecurityFindings(newTestSecurityFindingsRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAWSSecurityUploadFailed))
}
