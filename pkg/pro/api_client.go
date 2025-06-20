package pro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/charmbracelet/log"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

var (
	ErrFailedToCreateRequest          = errors.New("failed to create request")
	ErrFailedToMarshalPayload         = errors.New("failed to marshal payload")
	ErrFailedToCreateAuthRequest      = errors.New("failed to create authenticated request")
	ErrFailedToMakeRequest            = errors.New("failed to make request")
	ErrFailedToUploadStacks           = errors.New("failed to upload stacks")
	ErrFailedToMarshalRequestBody     = errors.New("failed to marshal request body")
	ErrFailedToReadResponseBody       = errors.New("error reading response body")
	ErrFailedToUnmarshalJSON          = errors.New("error unmarshaling JSON")
	ErrFailedToLockStack              = errors.New("an error occurred while attempting to lock stack")
	ErrFailedToUnlockStack            = errors.New("an error occurred while attempting to unlock stack")
	ErrOIDCWorkspaceIDRequired        = errors.New("workspace ID environment variable is required for OIDC authentication")
	ErrOIDCTokenExchangeFailed        = errors.New("failed to exchange OIDC token for Atmos token")
	ErrOIDCAuthFailedNoToken          = errors.New("OIDC authentication failed")
	ErrNotInGitHubActions             = errors.New("not running in GitHub Actions or missing OIDC token environment variables")
	ErrFailedToGetOIDCToken           = errors.New("failed to get OIDC token")
	ErrFailedToDecodeOIDCResponse     = errors.New("failed to decode OIDC token response")
	ErrFailedToExchangeOIDCToken      = errors.New("failed to exchange OIDC token")
	ErrFailedToDecodeTokenResponse    = errors.New("failed to decode token response")
	ErrFailedToGetGitHubOIDCToken     = errors.New("failed to get GitHub OIDC token")
	ErrFailedToUploadDeployments      = errors.New("failed to upload deployments")
	ErrFailedToUploadDeploymentStatus = errors.New("failed to upload deployment status")
)

const (
	DefaultHTTPTimeoutSecs = 30
)

// AtmosProAPIClientInterface defines the interface for the AtmosProAPIClient.
type AtmosProAPIClientInterface interface {
	UploadDeploymentStatus(dto *dtos.DeploymentStatusUploadRequest) error
}

// AtmosProAPIClient represents the client to interact with the AtmosPro API.
type AtmosProAPIClient struct {
	APIToken        string
	BaseAPIEndpoint string
	BaseURL         string
	HTTPClient      *http.Client
}

// NewAtmosProAPIClient creates a new instance of AtmosProAPIClient.
func NewAtmosProAPIClient(baseURL, baseAPIEndpoint, apiToken string) *AtmosProAPIClient {
	return &AtmosProAPIClient{
		BaseURL:         baseURL,
		BaseAPIEndpoint: baseAPIEndpoint,
		APIToken:        apiToken,
		HTTPClient:      &http.Client{Timeout: DefaultHTTPTimeoutSecs * time.Second},
	}
}

// NewAtmosProAPIClientFromEnv creates a new AtmosProAPIClient from environment variables.
func NewAtmosProAPIClientFromEnv(atmosConfig *schema.AtmosConfiguration) (*AtmosProAPIClient, error) {
	baseURL := atmosConfig.Settings.Pro.BaseURL

	if baseURL == "" {
		baseURL = cfg.AtmosProDefaultBaseUrl
	}
	log.Debug("Using baseURL", "baseURL", baseURL)

	baseAPIEndpoint := atmosConfig.Settings.Pro.Endpoint
	if baseAPIEndpoint == "" {
		baseAPIEndpoint = cfg.AtmosProDefaultEndpoint
	}
	log.Debug("Using baseAPIEndpoint", "baseAPIEndpoint", baseAPIEndpoint)

	// First, check if the API key is set via environment variable
	apiToken := atmosConfig.Settings.Pro.Token
	if apiToken != "" {
		log.Debug("Creating API client with API token from environment variable")
		return NewAtmosProAPIClient(baseURL, baseAPIEndpoint, apiToken), nil
	}

	// If API key is not set, attempt to use GitHub OIDC token exchange
	oidcToken, err := getGitHubOIDCToken(atmosConfig.Settings.Pro.GithubOIDC)
	if err != nil {
		log.Debug("Error while getting GitHub OIDC token", "err", err)
		return nil, fmt.Errorf(cfg.ErrFormatString, ErrFailedToGetGitHubOIDCToken, err)
	}

	// Get workspace ID from environment
	workspaceID := atmosConfig.Settings.Pro.WorkspaceID
	if workspaceID == "" {
		return nil, fmt.Errorf(cfg.ErrFormatString, ErrOIDCWorkspaceIDRequired, cfg.AtmosProWorkspaceIDEnvVarName)
	}

	// Exchange OIDC token for Atmos token
	apiToken, err = exchangeOIDCTokenForAtmosToken(baseURL, baseAPIEndpoint, oidcToken, workspaceID)
	if err != nil {
		return nil, fmt.Errorf(cfg.ErrFormatString, ErrOIDCTokenExchangeFailed, err)
	}

	return NewAtmosProAPIClient(baseURL, baseAPIEndpoint, apiToken), nil
}

func getAuthenticatedRequest(c *AtmosProAPIClient, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf(cfg.ErrFormatString, ErrFailedToCreateRequest, err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIToken))
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// UploadAffectedStacks uploads information about affected stacks.
func (c *AtmosProAPIClient) UploadAffectedStacks(dto *dtos.UploadAffectedStacksRequest) error {
	url := fmt.Sprintf("%s/%s/affected-stacks", c.BaseURL, c.BaseAPIEndpoint)

	data, err := utils.ConvertToJSON(*dto)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToMarshalPayload, err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer([]byte(data)))
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToCreateAuthRequest, err)
	}

	log.Debug(fmt.Sprintf("\nUploading the affected components and stacks to %s", url))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToUploadStacks, resp.Status)
	}
	log.Debug(fmt.Sprintf("\nUploaded the affected components and stacks to %s", url))

	return nil
}

// LockStack locks a specific stack.
func (c *AtmosProAPIClient) LockStack(dto dtos.LockStackRequest) (dtos.LockStackResponse, error) {
	url := fmt.Sprintf("%s/%s/locks", c.BaseURL, c.BaseAPIEndpoint)
	log.Debug(fmt.Sprintf("\nLocking stack at %s", url))

	data, err := json.Marshal(dto)
	if err != nil {
		return dtos.LockStackResponse{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return dtos.LockStackResponse{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return dtos.LockStackResponse{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return dtos.LockStackResponse{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToReadResponseBody, err)
	}

	// Create an instance of the struct
	var responseData dtos.LockStackResponse

	// Unmarshal the JSON response into the struct
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return dtos.LockStackResponse{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToUnmarshalJSON, err)
	}

	if !responseData.Success {
		var context string
		for key, value := range responseData.Context {
			context += fmt.Sprintf("  %s: %v\n", key, value)
		}

		return dtos.LockStackResponse{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToLockStack, responseData.ErrorMessage)

	}

	return responseData, nil
}

// UnlockStack unlocks a specific stack.
func (c *AtmosProAPIClient) UnlockStack(dto dtos.UnlockStackRequest) (dtos.UnlockStackResponse, error) {
	url := fmt.Sprintf("%s/%s/locks", c.BaseURL, c.BaseAPIEndpoint)
	log.Debug(fmt.Sprintf("\nLocking stack at %s", url))

	data, err := json.Marshal(dto)
	if err != nil {
		return dtos.UnlockStackResponse{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "DELETE", url, bytes.NewBuffer(data))
	if err != nil {
		return dtos.UnlockStackResponse{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return dtos.UnlockStackResponse{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return dtos.UnlockStackResponse{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToReadResponseBody, err)
	}

	// Create an instance of the struct
	var responseData dtos.UnlockStackResponse

	// Unmarshal the JSON response into the struct
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return dtos.UnlockStackResponse{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToUnmarshalJSON, err)
	}

	if !responseData.Success {
		var context string
		for key, value := range responseData.Context {
			context += fmt.Sprintf("  %s: %v\n", key, value)
		}

		return dtos.UnlockStackResponse{}, fmt.Errorf(cfg.ErrFormatString, ErrFailedToUnlockStack, responseData.ErrorMessage)
	}

	return responseData, nil
}

// getGitHubOIDCToken retrieves an OIDC token from GitHub Actions.
func getGitHubOIDCToken(githubOIDCSettings schema.GithubOIDCSettings) (string, error) {
	requestURL := githubOIDCSettings.RequestURL
	requestToken := githubOIDCSettings.RequestToken

	if requestURL == "" || requestToken == "" {
		return "", ErrNotInGitHubActions
	}

	// Add audience parameter to the request URL
	requestOIDCTokenURL := fmt.Sprintf("%s&audience=atmos-pro.com", requestURL)
	log.Debug("requestOIDCTokenURL", "requestOIDCTokenURL", requestOIDCTokenURL)

	req, err := http.NewRequest("GET", requestOIDCTokenURL, nil)
	if err != nil {
		return "", fmt.Errorf(cfg.ErrFormatString, ErrFailedToCreateRequest, err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", requestToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Debug("got error", "err", err)
		return "", fmt.Errorf(cfg.ErrFormatString, ErrFailedToGetOIDCToken, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Debug("got error", "resp.StatusCode", resp.StatusCode)
		return "", fmt.Errorf(cfg.ErrFormatString, ErrFailedToGetOIDCToken, resp.Status)
	}

	var tokenResp dtos.GetGitHubOIDCResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf(cfg.ErrFormatString, ErrFailedToDecodeOIDCResponse, err)
	}

	return tokenResp.Value, nil
}

// exchangeOIDCTokenForAtmosToken exchanges a GitHub OIDC token for an Atmos Pro token.
func exchangeOIDCTokenForAtmosToken(baseURL, baseAPIEndpoint, oidcToken, workspaceID string) (string, error) {
	url := fmt.Sprintf("%s/%s/auth/github-oidc", baseURL, baseAPIEndpoint)

	reqBody := dtos.ExchangeGitHubOIDCTokenRequest{
		Token:       oidcToken,
		WorkspaceID: workspaceID,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf(cfg.ErrFormatString, ErrFailedToMarshalRequestBody, err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf(cfg.ErrFormatString, ErrFailedToCreateRequest, err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf(cfg.ErrFormatString, ErrFailedToExchangeOIDCToken, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(cfg.ErrFormatString, ErrFailedToExchangeOIDCToken, resp.Status)
	}

	var tokenResp dtos.ExchangeGitHubOIDCTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf(cfg.ErrFormatString, ErrFailedToDecodeTokenResponse, err)
	}

	if !tokenResp.Success {
		return "", fmt.Errorf(cfg.ErrFormatString, ErrFailedToExchangeOIDCToken, tokenResp.ErrorMessage)
	}

	return tokenResp.Data.Token, nil
}
