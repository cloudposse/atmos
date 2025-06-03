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

// AtmosProAPIClient represents the client to interact with the AtmosPro API.
type AtmosProAPIClient struct {
	APIToken        string
	BaseAPIEndpoint string
	BaseURL         string
	HTTPClient      *http.Client
	Logger          *logger.Logger
}

// NewAtmosProAPIClient creates a new instance of AtmosProAPIClient
func NewAtmosProAPIClient(logger *logger.Logger, baseURL, baseAPIEndpoint, apiToken string) *AtmosProAPIClient {
	return &AtmosProAPIClient{
		Logger:          logger,
		BaseURL:         baseURL,
		BaseAPIEndpoint: baseAPIEndpoint,
		APIToken:        apiToken,
		HTTPClient:      &http.Client{Timeout: 30 * time.Second},
	}
}

// NewAtmosProAPIClientFromEnv creates a new AtmosProAPIClient from environment variables
func NewAtmosProAPIClientFromEnv(logger *logger.Logger) (*AtmosProAPIClient, error) {
	baseURL := os.Getenv(cfg.AtmosProBaseUrlEnvVarName)
	if baseURL == "" {
		baseURL = cfg.AtmosProDefaultBaseUrl
	}

	baseAPIEndpoint := os.Getenv(cfg.AtmosProEndpointEnvVarName)
	if baseAPIEndpoint == "" {
		baseAPIEndpoint = cfg.AtmosProDefaultEndpoint
	}

	var apiToken string
	var err error

	// Try OIDC authentication first
	oidcToken, err := getGitHubOIDCToken()
	if err == nil {
		// Get workspace ID from environment
		workspaceID := os.Getenv(cfg.AtmosProWorkspaceIDEnvVarName)
		if workspaceID == "" {
			return nil, fmt.Errorf("%s environment variable is required for OIDC authentication", cfg.AtmosProWorkspaceIDEnvVarName)
		}

		// Exchange OIDC token for Atmos token
		apiToken, err = exchangeOIDCTokenForAtmosToken(baseURL, baseAPIEndpoint, oidcToken, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange OIDC token for Atmos token: %w", err)
		}
	} else {
		// Fall back to API token from environment
		apiToken = os.Getenv(cfg.AtmosProTokenEnvVarName)
		if apiToken == "" {
			return nil, fmt.Errorf("OIDC authentication failed and %s is not set", cfg.AtmosProTokenEnvVarName)
		}
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

// UploadAffectedStacks uploads information about affected stacks
func (c *AtmosProAPIClient) UploadAffectedStacks(dto AffectedStacksUploadRequest) error {
	url := fmt.Sprintf("%s/%s/affected-stacks", c.BaseURL, c.BaseAPIEndpoint)

	data, err := utils.ConvertToJSON(dto)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer([]byte(data)))
	if err != nil {
		return fmt.Errorf("failed to create authenticated request: %w", err)
	}

	c.Logger.Trace(fmt.Sprintf("\nUploading the affected components and stacks to %s", url))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("failed to upload stacks, status: %s", resp.Status)
	}
	c.Logger.Trace(fmt.Sprintf("\nUploaded the affected components and stacks to %s", url))

	return nil
}

// LockStack locks a specific stack
func (c *AtmosProAPIClient) LockStack(dto LockStackRequest) (LockStackResponse, error) {
	url := fmt.Sprintf("%s/%s/locks", c.BaseURL, c.BaseAPIEndpoint)
	c.Logger.Trace(fmt.Sprintf("\nLocking stack at %s", url))

	data, err := json.Marshal(dto)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return LockStackResponse{}, fmt.Errorf("failed to create authenticated request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf("error reading response body: %s", err)
	}

	// Create an instance of the struct
	var responseData LockStackResponse

	// Unmarshal the JSON response into the struct
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf("error unmarshaling JSON: %s", err)
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

// UnlockStack unlocks a specific stack
func (c *AtmosProAPIClient) UnlockStack(dto UnlockStackRequest) (UnlockStackResponse, error) {
	url := fmt.Sprintf("%s/%s/locks", c.BaseURL, c.BaseAPIEndpoint)
	c.Logger.Trace(fmt.Sprintf("\nLocking stack at %s", url))

	data, err := json.Marshal(dto)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := getAuthenticatedRequest(c, "DELETE", url, bytes.NewBuffer(data))
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf("failed to create authenticated request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf("error reading response body: %s", err)
	}

	// Create an instance of the struct
	var responseData UnlockStackResponse

	// Unmarshal the JSON response into the struct
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf("error unmarshaling JSON: %s", err)
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

// getGitHubOIDCToken retrieves an OIDC token from GitHub Actions
func getGitHubOIDCToken() (string, error) {
	requestURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	requestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")

	if requestURL == "" || requestToken == "" {
		return "", fmt.Errorf("not running in GitHub Actions or missing OIDC token environment variables")
	}

	// Add audience parameter to the request URL
	requestURL = fmt.Sprintf("%s&audience=app.cloudposse.com", requestURL)

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", requestToken))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get OIDC token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get OIDC token, status: %s", resp.Status)
	}

	var tokenResp GitHubOIDCResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode OIDC token response: %w", err)
	}

	return tokenResp.Value, nil
}

// exchangeOIDCTokenForAtmosToken exchanges a GitHub OIDC token for an Atmos Pro token.
func exchangeOIDCTokenForAtmosToken(baseURL, baseAPIEndpoint, oidcToken, workspaceID string) (string, error) {
	url := fmt.Sprintf("%s/%s/auth/github-oidc", baseURL, baseAPIEndpoint)

	reqBody := GitHubOIDCAuthRequest{
		Token:       oidcToken,
		WorkspaceID: workspaceID,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to exchange OIDC token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to exchange OIDC token, status: %s", resp.Status)
	}

	var tokenResp GitHubOIDCAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	return tokenResp.Token, nil
}
