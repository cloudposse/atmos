package pro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	errFailedToCreateRequest      = "failed to create request: %w"
	errFailedToMarshalPayload     = "failed to marshal payload: %w"
	errFailedToCreateAuthRequest  = "failed to create authenticated request: %w"
	errFailedToMakeRequest        = "failed to make request: %w"
	errFailedToUploadStacks       = "failed to upload stacks, status: %s"
	errFailedToMarshalRequestBody = "failed to marshal request body: %w"
	errFailedToReadResponseBody   = "error reading response body: %s"
	errFailedToUnmarshalJSON      = "error unmarshaling JSON: %s"
	errFailedToLockStack          = "an error occurred while attempting to lock stack.\n\nError: %s\nContext:\n%s"
	errFailedToUnlockStack        = "an error occurred while attempting to unlock stack.\n\nError: %s\nContext:\n%s"
	errFailedToUploadDriftStatus  = "failed to upload drift status, status: %s"
	errTokenNotSet                = "%s is not set"
)

// AtmosProAPIClientInterface defines the interface for the AtmosProAPIClient.
type AtmosProAPIClientInterface interface {
	UploadDriftResultStatus(dto DriftStatusUploadRequest) error
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
		return nil, fmt.Errorf(errTokenNotSet, cfg.AtmosProTokenEnvVarName)
	}

	return NewAtmosProAPIClient(logger, baseURL, baseAPIEndpoint, apiToken), nil
}

func getAuthenticatedRequest(c *AtmosProAPIClient, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
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
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer([]byte(data)))
	if err != nil {
		return fmt.Errorf(errFailedToCreateAuthRequest, err)
	}

	c.Logger.Trace(fmt.Sprintf("\nUploading the affected components and stacks to %s", url))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf(errFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("failed to upload stacks, status: %s", resp.Status)
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
		return LockStackResponse{}, fmt.Errorf(errFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return LockStackResponse{}, fmt.Errorf(errFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf(errFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf(errFailedToReadResponseBody, err)
	}

	var responseData LockStackResponse

	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf(errFailedToUnmarshalJSON, err)
	}

	if !responseData.Success {
		var context string
		for key, value := range responseData.Context {
			context += fmt.Sprintf("  %s: %v\n", key, value)
		}

		return LockStackResponse{}, fmt.Errorf("an error occurred while attempting to lock stack.\n\nError: %s\nContext:\n%s", responseData.ErrorMessage, context)
	}

	return responseData, nil
}

// UnlockStack unlocks a specific stack.
func (c *AtmosProAPIClient) UnlockStack(dto UnlockStackRequest) (UnlockStackResponse, error) {
	url := fmt.Sprintf("%s/%s/locks", c.BaseURL, c.BaseAPIEndpoint)
	c.Logger.Trace(fmt.Sprintf("\nLocking stack at %s", url))

	data, err := json.Marshal(dto)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf(errFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "DELETE", url, bytes.NewBuffer(data))
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf(errFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf(errFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf(errFailedToReadResponseBody, err)
	}

	var responseData UnlockStackResponse

	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf(errFailedToUnmarshalJSON, err)
	}

	if !responseData.Success {
		var context string
		for key, value := range responseData.Context {
			context += fmt.Sprintf("  %s: %v\n", key, value)
		}

		return UnlockStackResponse{}, fmt.Errorf("an error occurred while attempting to unlock stack.\n\nError: %s\nContext:\n%s", responseData.ErrorMessage, context)
	}

	return responseData, nil
}

// UploadDriftResultStatus uploads the drift detection result status to the pro API.
func (c *AtmosProAPIClient) UploadDriftResultStatus(dto DriftStatusUploadRequest) error {
	url := fmt.Sprintf("%s/%s/drift-status", c.BaseURL, c.BaseAPIEndpoint)
	c.Logger.Trace(fmt.Sprintf("\nUploading drift status at %s", url))

	data, err := json.Marshal(dto)
	if err != nil {
		return fmt.Errorf(errFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf(errFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf(errFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf(errFailedToUploadDriftStatus, resp.Status)
	}

	c.Logger.Trace(fmt.Sprintf("\nUploaded drift status at %s", url))

	return nil
}
