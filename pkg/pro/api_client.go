package pro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/viper"
)

var (
	ErrFailedToCreateRequest       = errors.New("failed to create request")
	ErrFailedToMarshalPayload      = errors.New("failed to marshal payload")
	ErrFailedToCreateAuthRequest   = errors.New("failed to create authenticated request")
	ErrFailedToMakeRequest         = errors.New("failed to make request")
	ErrFailedToUploadStacks        = errors.New("failed to upload stacks")
	ErrFailedToMarshalRequestBody  = errors.New("failed to marshal request body")
	ErrFailedToReadResponseBody    = errors.New("error reading response body")
	ErrFailedToUnmarshalJSON       = errors.New("error unmarshaling JSON")
	ErrFailedToLockStack           = errors.New("an error occurred while attempting to lock stack")
	ErrFailedToUnlockStack         = errors.New("an error occurred while attempting to unlock stack")
	ErrOIDCWorkspaceIDRequired     = errors.New("workspace ID environment variable is required for OIDC authentication")
	ErrOIDCTokenExchangeFailed     = errors.New("failed to exchange OIDC token for Atmos token")
	ErrOIDCAuthFailedNoToken       = errors.New("OIDC authentication failed and API token is not set")
	ErrNotInGitHubActions          = errors.New("not running in GitHub Actions or missing OIDC token environment variables")
	ErrFailedToGetOIDCToken        = errors.New("failed to get OIDC token")
	ErrFailedToDecodeOIDCResponse  = errors.New("failed to decode OIDC token response")
	ErrFailedToExchangeOIDCToken   = errors.New("failed to exchange OIDC token")
	ErrFailedToDecodeTokenResponse = errors.New("failed to decode token response")
)

const (
	ErrFormatString = "%w: %s"
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
	baseURL := viper.GetString(cfg.AtmosProBaseUrlEnvVarName)
	if baseURL == "" {
		baseURL = cfg.AtmosProDefaultBaseUrl
	}

	baseAPIEndpoint := viper.GetString(cfg.AtmosProEndpointEnvVarName)
	if baseAPIEndpoint == "" {
		baseAPIEndpoint = cfg.AtmosProDefaultEndpoint
	}

	// First, check if the API key is set via environment variable
	apiToken := viper.GetString(cfg.AtmosProTokenEnvVarName)
	if apiToken != "" {
		return NewAtmosProAPIClient(logger, baseURL, baseAPIEndpoint, apiToken), nil
	}

	// If API key is not set, attempt to use GitHub OIDC token exchange
	oidcToken, err := getGitHubOIDCToken()
	if err != nil {
		return nil, fmt.Errorf(ErrFormatString, ErrOIDCAuthFailedNoToken, cfg.AtmosProTokenEnvVarName)
	}

	// Get workspace ID from environment
	workspaceID := viper.GetString(cfg.AtmosProWorkspaceIDEnvVarName)
	if workspaceID == "" {
		return nil, fmt.Errorf(ErrFormatString, ErrOIDCWorkspaceIDRequired, cfg.AtmosProWorkspaceIDEnvVarName)
	}

	// Exchange OIDC token for Atmos token
	apiToken, err = exchangeOIDCTokenForAtmosToken(baseURL, baseAPIEndpoint, oidcToken, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCTokenExchangeFailed, err)
	}

	return NewAtmosProAPIClient(logger, baseURL, baseAPIEndpoint, apiToken), nil
}

func getAuthenticatedRequest(c *AtmosProAPIClient, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf(ErrFormatString, ErrFailedToCreateRequest, err)
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
		return fmt.Errorf(ErrFormatString, ErrFailedToMarshalPayload, err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer([]byte(data)))
	if err != nil {
		return fmt.Errorf(ErrFormatString, ErrFailedToCreateAuthRequest, err)
	}

	c.Logger.Trace(fmt.Sprintf("\nUploading the affected components and stacks to %s", url))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf(ErrFormatString, ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf(ErrFormatString, ErrFailedToUploadStacks, resp.Status)
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
		return LockStackResponse{}, fmt.Errorf(ErrFormatString, ErrFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return LockStackResponse{}, fmt.Errorf(ErrFormatString, ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf(ErrFormatString, ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf(ErrFormatString, ErrFailedToReadResponseBody, err)
	}

	// Create an instance of the struct
	var responseData LockStackResponse

	// Unmarshal the JSON response into the struct
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return LockStackResponse{}, fmt.Errorf(ErrFormatString, ErrFailedToUnmarshalJSON, err)
	}

	if !responseData.Success {
		var context string
		for key, value := range responseData.Context {
			context += fmt.Sprintf("  %s: %v\n", key, value)
		}

		return LockStackResponse{}, fmt.Errorf(ErrFormatString, ErrFailedToLockStack, responseData.ErrorMessage)
	}

	return responseData, nil
}

// UnlockStack unlocks a specific stack
func (c *AtmosProAPIClient) UnlockStack(dto UnlockStackRequest) (UnlockStackResponse, error) {
	url := fmt.Sprintf("%s/%s/locks", c.BaseURL, c.BaseAPIEndpoint)
	c.Logger.Trace(fmt.Sprintf("\nLocking stack at %s", url))

	data, err := json.Marshal(dto)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf(ErrFormatString, ErrFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "DELETE", url, bytes.NewBuffer(data))
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf(ErrFormatString, ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf(ErrFormatString, ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf(ErrFormatString, ErrFailedToReadResponseBody, err)
	}

	// Create an instance of the struct
	var responseData UnlockStackResponse

	// Unmarshal the JSON response into the struct
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return UnlockStackResponse{}, fmt.Errorf(ErrFormatString, ErrFailedToUnmarshalJSON, err)
	}

	if !responseData.Success {
		var context string
		for key, value := range responseData.Context {
			context += fmt.Sprintf("  %s: %v\n", key, value)
		}

		return UnlockStackResponse{}, fmt.Errorf(ErrFormatString, ErrFailedToUnlockStack, responseData.ErrorMessage)
	}

	return responseData, nil
}

// getGitHubOIDCToken retrieves an OIDC token from GitHub Actions.
func getGitHubOIDCToken() (string, error) {
	requestURL := viper.GetString("ACTIONS_ID_TOKEN_REQUEST_URL")
	requestToken := viper.GetString("ACTIONS_ID_TOKEN_REQUEST_TOKEN")

	if requestURL == "" || requestToken == "" {
		return "", ErrNotInGitHubActions
	}

	// Add audience parameter to the request URL
	requestURL = fmt.Sprintf("%s&audience=app.cloudposse.com", requestURL)

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return "", fmt.Errorf(ErrFormatString, ErrFailedToCreateRequest, err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", requestToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf(ErrFormatString, ErrFailedToGetOIDCToken, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(ErrFormatString, ErrFailedToGetOIDCToken, resp.Status)
	}

	var tokenResp GitHubOIDCResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf(ErrFormatString, ErrFailedToDecodeOIDCResponse, err)
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
		return "", fmt.Errorf(ErrFormatString, ErrFailedToMarshalRequestBody, err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf(ErrFormatString, ErrFailedToCreateRequest, err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf(ErrFormatString, ErrFailedToExchangeOIDCToken, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(ErrFormatString, ErrFailedToExchangeOIDCToken, resp.Status)
	}

	var tokenResp GitHubOIDCAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf(ErrFormatString, ErrFailedToDecodeTokenResponse, err)
	}

	return tokenResp.Token, nil
}
