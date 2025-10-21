package pro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	DefaultHTTPTimeoutSecs           = 30
	DefaultDialTimeoutSecs           = 10
	DefaultIdleConnTimeoutSecs       = 30
	DefaultResponseHeaderTimeoutSecs = 15
	DefaultExpectContinueTimeoutSecs = 1
	logKeyURL                        = "url"
	logKeyOperation                  = "operation"
	logKeyRequest                    = "request"
	errMessageFormat                 = "%w: %s" // Error format for wrapping with message
	logKeyStatus                     = "status"
	logKeySuccess                    = "success"
	logKeyTraceID                    = "trace_id"
	logKeyContext                    = "context"
	logKeyErrorMessage               = "error_message"
)

func logProAPIResponse(operation string, apiResponse dtos.AtmosApiResponse) {
	log.Debug("Pro API Response",
		logKeyOperation, operation,
		logKeyRequest, apiResponse.Request,
		logKeyStatus, apiResponse.Status,
		logKeySuccess, apiResponse.Success,
		logKeyTraceID, apiResponse.TraceID,
		logKeyContext, apiResponse.Context,
	)
}

func logAndReturnProAPIError(operation string, apiResponse dtos.AtmosApiResponse) string {
	errorMsg := apiResponse.ErrorMessage
	traceID := apiResponse.TraceID

	log.Error("Pro API Error",
		logKeyOperation, operation,
		logKeyRequest, apiResponse.Request,
		logKeyStatus, apiResponse.Status,
		logKeySuccess, apiResponse.Success,
		logKeyTraceID, traceID,
		logKeyErrorMessage, errorMsg,
		logKeyContext, apiResponse.Context,
	)

	if errorMsg == "" {
		errorMsg = fmt.Sprintf("API request failed with status %d", apiResponse.Status)
	}

	if traceID != "" {
		errorMsg = fmt.Sprintf("%s (trace_id: %s)", errorMsg, traceID)
	}

	return errorMsg
}

// AtmosProAPIClientInterface defines the interface for the AtmosProAPIClient.
type AtmosProAPIClientInterface interface {
	UploadInstances(req *dtos.InstancesUploadRequest) error
	UploadInstanceStatus(dto *dtos.InstanceStatusUploadRequest) error
	UploadAffectedStacks(dto *dtos.UploadAffectedStacksRequest) error
	LockStack(dto *dtos.LockStackRequest) (dtos.LockStackResponse, error)
	UnlockStack(dto *dtos.UnlockStackRequest) (dtos.UnlockStackResponse, error)
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
		log.Debug("Error while getting GitHub OIDC token.", "error", err)
		return nil, errors.Join(errUtils.ErrFailedToGetGitHubOIDCToken, err)
	}

	// Get workspace ID from environment
	workspaceID := atmosConfig.Settings.Pro.WorkspaceID
	if workspaceID == "" {
		return nil, fmt.Errorf("%w: environment variable: %s", errUtils.ErrOIDCWorkspaceIDRequired, cfg.AtmosProWorkspaceIDEnvVarName)
	}

	// Exchange OIDC token for Atmos token
	apiToken, err = exchangeOIDCTokenForAtmosToken(baseURL, baseAPIEndpoint, oidcToken, workspaceID)
	if err != nil {
		return nil, errors.Join(errUtils.ErrOIDCTokenExchangeFailed, err)
	}

	return NewAtmosProAPIClient(baseURL, baseAPIEndpoint, apiToken), nil
}

func getAuthenticatedRequest(c *AtmosProAPIClient, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, errors.Join(errUtils.ErrFailedToCreateRequest, err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIToken))
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// UploadAffectedStacks uploads information about affected stacks.
func (c *AtmosProAPIClient) UploadAffectedStacks(dto *dtos.UploadAffectedStacksRequest) error {
	url := fmt.Sprintf("%s/%s/affected-stacks", c.BaseURL, c.BaseAPIEndpoint)

	data, err := utils.ConvertToJSON(dto)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToMarshalPayload, err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer([]byte(data)))
	if err != nil {
		return errors.Join(errUtils.ErrFailedToCreateAuthRequest, err)
	}

	log.Debug("Uploading affected components and stacks.", logKeyURL, url)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if err := handleAPIResponse(resp, "UploadAffectedStacks"); err != nil {
		return errors.Join(errUtils.ErrFailedToUploadStacks, err)
	}

	log.Debug("Uploaded affected components and stacks.", logKeyURL, url)

	return nil
}

// doStackLockAction is a private helper function that handles the common logic for stack lock/unlock operations.
func (c *AtmosProAPIClient) doStackLockAction(params *schema.StackLockActionParams) error {
	data, err := json.Marshal(params.Body)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToMarshalPayload, err)
	}

	req, err := getAuthenticatedRequest(c, params.Method, params.URL, bytes.NewBuffer(data))
	if err != nil {
		return errors.Join(errUtils.ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToReadResponseBody, err)
	}

	if err := json.Unmarshal(b, params.Out); err != nil {
		return errors.Join(errUtils.ErrFailedToUnmarshalAPIResponse, err)
	}

	// Log the structured response for debugging and check success
	// We need to use type assertion to access the embedded AtmosApiResponse
	switch responseData := params.Out.(type) {
	case *dtos.LockStackResponse:
		logProAPIResponse(params.Op, responseData.AtmosApiResponse)
		if !responseData.Success {
			errorMsg := logAndReturnProAPIError(params.Op, responseData.AtmosApiResponse)
			return fmt.Errorf(errMessageFormat, params.WrapErr, errorMsg)
		}
	case *dtos.UnlockStackResponse:
		logProAPIResponse(params.Op, responseData.AtmosApiResponse)
		if !responseData.Success {
			errorMsg := logAndReturnProAPIError(params.Op, responseData.AtmosApiResponse)
			return fmt.Errorf(errMessageFormat, params.WrapErr, errorMsg)
		}
	}

	return nil
}

// LockStack locks a specific stack.
func (c *AtmosProAPIClient) LockStack(dto *dtos.LockStackRequest) (dtos.LockStackResponse, error) {
	url := fmt.Sprintf("%s/%s/locks", c.BaseURL, c.BaseAPIEndpoint)
	log.Debug("Locking stack.", logKeyURL, url)

	var responseData dtos.LockStackResponse
	err := c.doStackLockAction(&schema.StackLockActionParams{
		Method:  http.MethodPost,
		URL:     url,
		Body:    dto,
		Out:     &responseData,
		Op:      "LockStack",
		WrapErr: errUtils.ErrFailedToLockStack,
	})
	if err != nil {
		return dtos.LockStackResponse{}, err
	}

	return responseData, nil
}

// UnlockStack unlocks a specific stack.
func (c *AtmosProAPIClient) UnlockStack(dto *dtos.UnlockStackRequest) (dtos.UnlockStackResponse, error) {
	url := fmt.Sprintf("%s/%s/locks", c.BaseURL, c.BaseAPIEndpoint)
	log.Debug("Unlocking stack.", logKeyURL, url)

	var responseData dtos.UnlockStackResponse
	err := c.doStackLockAction(&schema.StackLockActionParams{
		Method:  http.MethodDelete,
		URL:     url,
		Body:    dto,
		Out:     &responseData,
		Op:      "UnlockStack",
		WrapErr: errUtils.ErrFailedToUnlockStack,
	})
	if err != nil {
		return dtos.UnlockStackResponse{}, err
	}

	return responseData, nil
}

// handleAPIResponse processes the HTTP response and logs detailed information including trace IDs and error messages.
// It returns an error if the response indicates failure.
func handleAPIResponse(resp *http.Response, operation string) error {
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToReadResponseBody, err)
	}

	var apiResponse dtos.AtmosApiResponse

	// Try to unmarshal the response to get structured data
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		// If we can't parse the response as JSON, handle based on status code
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
			return fmt.Errorf("%w: HTTP status: %s", errUtils.ErrFailedToUnmarshalAPIResponse, resp.Status)
		}
		// For successful responses that can't be parsed, just return nil
		return nil
	}

	// Log the structured response for debugging (only if we successfully unmarshaled)
	logProAPIResponse(operation, apiResponse)

	// For successful HTTP responses, trust the status code over the Success field
	// (some APIs might return minimal responses without the Success field)
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		return nil
	}

	// For error HTTP responses, return an error
	errorMsg := logAndReturnProAPIError(operation, apiResponse)
	return fmt.Errorf(errMessageFormat, errUtils.ErrAPIResponseError, errorMsg)
}

// getGitHubOIDCToken retrieves an OIDC token from GitHub Actions.
func getGitHubOIDCToken(githubOIDCSettings schema.GithubOIDCSettings) (string, error) {
	requestURL := githubOIDCSettings.RequestURL
	requestToken := githubOIDCSettings.RequestToken

	if requestURL == "" || requestToken == "" {
		return "", errUtils.ErrNotInGitHubActions
	}

	// Add audience parameter to the request URL
	requestOIDCTokenURL := fmt.Sprintf("%s&audience=atmos-pro.com", requestURL)
	log.Debug("requestOIDCTokenURL", "requestOIDCTokenURL", requestOIDCTokenURL)

	req, err := http.NewRequest("GET", requestOIDCTokenURL, nil)
	if err != nil {
		return "", errors.Join(errUtils.ErrFailedToCreateRequest, err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", requestToken))

	client := getHTTPClientWithTimeout()
	resp, err := client.Do(req)
	if err != nil {
		log.Debug("getGitHubOIDCToken", "error", err)
		return "", errors.Join(errUtils.ErrFailedToGetOIDCToken, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Join(errUtils.ErrFailedToReadResponseBody, err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Debug("getGitHubOIDCToken", "resp.StatusCode", resp.StatusCode)
		return "", fmt.Errorf("%w: HTTP status: %s", errUtils.ErrFailedToGetOIDCToken, resp.Status)
	}

	var tokenResp dtos.GetGitHubOIDCResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", errors.Join(errUtils.ErrFailedToDecodeOIDCResponse, err)
	}

	return tokenResp.Value, nil
}

// getHTTPClientWithTimeout returns a configured HTTP client with reasonable timeouts for OIDC operations.
func getHTTPClientWithTimeout() *http.Client {
	return &http.Client{
		Timeout: DefaultHTTPTimeoutSecs * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: DefaultDialTimeoutSecs * time.Second,
			}).DialContext,
			IdleConnTimeout:       DefaultIdleConnTimeoutSecs * time.Second,
			ResponseHeaderTimeout: DefaultResponseHeaderTimeoutSecs * time.Second,
			ExpectContinueTimeout: DefaultExpectContinueTimeoutSecs * time.Second,
		},
	}
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
		return "", errors.Join(errUtils.ErrFailedToMarshalPayload, err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", errors.Join(errUtils.ErrFailedToCreateRequest, err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := getHTTPClientWithTimeout()
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.Join(errUtils.ErrFailedToExchangeOIDCToken, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Join(errUtils.ErrFailedToReadResponseBody, err)
	}

	// Try to parse the response to get trace ID from the response body
	var apiResponse dtos.AtmosApiResponse
	if err := json.Unmarshal(body, &apiResponse); err == nil {
		// Log the full response for debugging (only if we successfully unmarshaled)
		logProAPIResponse("ExchangeOIDCToken", apiResponse)
	}

	var tokenResp dtos.ExchangeGitHubOIDCTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", errors.Join(errUtils.ErrFailedToDecodeTokenResponse, err)
	}

	if !tokenResp.Success {
		errMsg := logAndReturnProAPIError("ExchangeOIDCToken", tokenResp.AtmosApiResponse)
		return "", fmt.Errorf(errMessageFormat, errUtils.ErrFailedToExchangeOIDCToken, errMsg)
	}

	return tokenResp.Data.Token, nil
}
