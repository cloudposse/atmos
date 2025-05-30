package pro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/utils"
)

var (
	ErrFailedToCreateRequest      = errors.New("failed to create request")
	ErrFailedToMarshalPayload     = errors.New("failed to marshal payload")
	ErrFailedToCreateAuthRequest  = errors.New("failed to create authenticated request")
	ErrFailedToMakeRequest        = errors.New("failed to make request")
	ErrFailedToUploadStacks       = errors.New("failed to upload stacks")
	ErrFailedToMarshalRequestBody = errors.New("failed to marshal request body")
	ErrFailedToReadResponseBody   = errors.New("error reading response body")
	ErrFailedToUnmarshalJSON      = errors.New("error unmarshaling JSON")
	ErrFailedToLockStack          = errors.New("an error occurred while attempting to lock stack")
	ErrFailedToUnlockStack        = errors.New("an error occurred while attempting to unlock stack")
	ErrFailedToUploadDriftStatus  = errors.New("failed to upload drift status")
	ErrTokenNotSet                = errors.New("token is not set")
)

// AtmosProAPIClientInterface defines the interface for the AtmosProAPIClient.
type AtmosProAPIClientInterface interface {
	UploadDriftResultStatus(dto *DriftStatusUploadRequest) error
}

// AtmosProAPIClient represents the client to interact with the AtmosPro API.
type AtmosProAPIClient struct {
	APIToken        string
	BaseAPIEndpoint string
	BaseURL         string
	HTTPClient      *http.Client
	Logger          *logger.Logger
}

// NewAtmosProAPIClient creates a new instance of AtmosProAPIClient.
func NewAtmosProAPIClient(logger *logger.Logger, baseURL, baseAPIEndpoint, apiToken string) *AtmosProAPIClient {
	return &AtmosProAPIClient{
		Logger:          logger,
		BaseURL:         baseURL,
		BaseAPIEndpoint: baseAPIEndpoint,
		APIToken:        apiToken,
		HTTPClient:      &http.Client{Timeout: 30 * time.Second},
	}
}

// NewAtmosProAPIClientFromEnv creates a new AtmosProAPIClient from environment variables.
func NewAtmosProAPIClientFromEnv(logger *logger.Logger) (*AtmosProAPIClient, error) {
	baseURL := os.Getenv(cfg.AtmosProBaseUrlEnvVarName)
	if baseURL == "" {
		baseURL = cfg.AtmosProDefaultBaseUrl
	}

	baseAPIEndpoint := os.Getenv(cfg.AtmosProEndpointEnvVarName)
	if baseAPIEndpoint == "" {
		baseAPIEndpoint = cfg.AtmosProDefaultEndpoint
	}

	apiToken := os.Getenv(cfg.AtmosProTokenEnvVarName)
	if apiToken == "" {
		return nil, fmt.Errorf("%w: %s", ErrTokenNotSet, cfg.AtmosProTokenEnvVarName)
	}

	return NewAtmosProAPIClient(logger, baseURL, baseAPIEndpoint, apiToken), nil
}

func getAuthenticatedRequest(c *AtmosProAPIClient, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedToCreateRequest, err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIToken))
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// UploadAffectedStacks uploads information about affected stacks.
func (c *AtmosProAPIClient) UploadAffectedStacks(dto AffectedStacksUploadRequest) error {
	url := fmt.Sprintf("%s/%s/affected-stacks", c.BaseURL, c.BaseAPIEndpoint)

	data, err := utils.ConvertToJSON(dto)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToMarshalPayload, err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer([]byte(data)))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCreateAuthRequest, err)
	}

	c.Logger.Trace(fmt.Sprintf("\nUploading the affected components and stacks to %s", url))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%w: %s", ErrFailedToUploadStacks, resp.Status)
	}
	c.Logger.Trace(fmt.Sprintf("\nUploaded the affected components and stacks to %s", url))

	return nil
}

// LockStack locks a specific stack.
func (c *AtmosProAPIClient) LockStack(dto LockStackRequest) (LockStackResponse, error) {
	url := fmt.Sprintf("%s/%s/locks", c.BaseURL, c.BaseAPIEndpoint)
	c.Logger.Trace(fmt.Sprintf("\nLocking stack at %s", url))

	data, err := json.Marshal(dto)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf("%w: %w", ErrFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return LockStackResponse{}, fmt.Errorf("%w: %w", ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf("%w: %w", ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf("%w: %w", ErrFailedToReadResponseBody, err)
	}

	var responseData LockStackResponse

	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf("%w: %w", ErrFailedToUnmarshalJSON, err)
	}

	if !responseData.Success {
		var context string
		for key, value := range responseData.Context {
			context += fmt.Sprintf("  %s: %v\n", key, value)
		}

		return LockStackResponse{}, fmt.Errorf("%w\n\nError: %s\nContext:\n%s", ErrFailedToLockStack, responseData.ErrorMessage, context)
	}

	return responseData, nil
}

// UnlockStack unlocks a specific stack.
func (c *AtmosProAPIClient) UnlockStack(dto UnlockStackRequest) (UnlockStackResponse, error) {
	url := fmt.Sprintf("%s/%s/locks", c.BaseURL, c.BaseAPIEndpoint)
	c.Logger.Trace(fmt.Sprintf("\nLocking stack at %s", url))

	data, err := json.Marshal(dto)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf("%w: %w", ErrFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "DELETE", url, bytes.NewBuffer(data))
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf("%w: %w", ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf("%w: %w", ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf("%w: %w", ErrFailedToReadResponseBody, err)
	}

	var responseData UnlockStackResponse

	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf("%w: %w", ErrFailedToUnmarshalJSON, err)
	}

	if !responseData.Success {
		var context string
		for key, value := range responseData.Context {
			context += fmt.Sprintf("  %s: %v\n", key, value)
		}

		return UnlockStackResponse{}, fmt.Errorf("%w\n\nError: %s\nContext:\n%s", ErrFailedToUnlockStack, responseData.ErrorMessage, context)
	}

	return responseData, nil
}

// UploadDriftResultStatus uploads the drift detection result status to the pro API.
func (c *AtmosProAPIClient) UploadDriftResultStatus(dto *DriftStatusUploadRequest) error {
	url := fmt.Sprintf("%s/%s/drift-status", c.BaseURL, c.BaseAPIEndpoint)
	c.Logger.Trace(fmt.Sprintf("\nUploading drift status at %s", url))

	data, err := json.Marshal(dto)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%w: %s", ErrFailedToUploadDriftStatus, resp.Status)
	}

	c.Logger.Trace(fmt.Sprintf("\nUploaded drift status at %s", url))

	return nil
}
