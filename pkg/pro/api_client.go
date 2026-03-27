package pro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
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
	logKeyStatus                     = "status"
	logKeySuccess                    = "success"
	logKeyTraceID                    = "trace_id"
	logKeyContext                    = "context"
	logKeyErrorMessage               = "error_message"
)

// oidcHTTPClientOverride may be set by tests to inject a custom TLS-aware HTTP
// client for the GitHub OIDC token request. It is nil by default and should
// only be set in test code (package pro is used by tests directly).
var oidcHTTPClientOverride *http.Client //nolint:gochecknoglobals

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
	// atmosConfig is stored for token refresh on 401 retries. Nil when created via NewAtmosProAPIClient.
	atmosConfig *schema.AtmosConfiguration
	// useOIDC indicates the client was created via OIDC exchange (not a static token),
	// meaning token refresh is possible on 401 errors.
	useOIDC         bool
	MaxPayloadBytes int // Configurable max payload size before chunking. 0 uses default.
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

	maxPayloadBytes := atmosConfig.Settings.Pro.MaxPayloadBytes

	// First, check if the API key is set via environment variable
	apiToken := atmosConfig.Settings.Pro.Token
	if apiToken != "" {
		log.Debug("Creating API client with API token from environment variable")
		client := NewAtmosProAPIClient(baseURL, baseAPIEndpoint, apiToken)
		client.MaxPayloadBytes = maxPayloadBytes
		return client, nil
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

	// Exchange OIDC token for Atmos token.
	apiToken, err = exchangeOIDCTokenForAtmosToken(baseURL, baseAPIEndpoint, oidcToken, workspaceID)
	if err != nil {
		return nil, errors.Join(errUtils.ErrOIDCTokenExchangeFailed, err)
	}

	client := NewAtmosProAPIClient(baseURL, baseAPIEndpoint, apiToken)
	client.atmosConfig = atmosConfig
	client.useOIDC = true
	client.MaxPayloadBytes = maxPayloadBytes

	return client, nil
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

// RefreshToken re-exchanges the OIDC token for a fresh Atmos Pro JWT.
// This is used by retry logic when a 401 suggests the original JWT was signed
// by a different deployment instance. Returns a no-op nil error when the client
// was created with a static token (no OIDC).
func (c *AtmosProAPIClient) RefreshToken() error {
	if !c.useOIDC || c.atmosConfig == nil {
		// Static token — nothing to refresh.
		return nil
	}

	oidcToken, err := getGitHubOIDCToken(c.atmosConfig.Settings.Pro.GithubOIDC)
	if err != nil {
		return errors.Join(errUtils.ErrTokenRefreshFailed, err)
	}

	workspaceID := c.atmosConfig.Settings.Pro.WorkspaceID
	newToken, err := exchangeOIDCTokenForAtmosToken(c.BaseURL, c.BaseAPIEndpoint, oidcToken, workspaceID)
	if err != nil {
		return errors.Join(errUtils.ErrTokenRefreshFailed, err)
	}

	c.APIToken = newToken
	log.Debug("Refreshed Atmos Pro API token via OIDC re-exchange.")

	return nil
}

// UploadAffectedStacks uploads information about affected stacks.
// Large payloads are automatically split into chunks to stay within server body size limits.
// Each chunk is retried on transient 401/5xx failures with exponential backoff, refreshing
// the OIDC token on 401 errors before each retry.
func (c *AtmosProAPIClient) UploadAffectedStacks(dto *dtos.UploadAffectedStacksRequest) error {
	endpoint := fmt.Sprintf("%s/%s/affected-stacks", c.BaseURL, c.BaseAPIEndpoint)

	// Estimate metadata overhead (everything except the stacks array).
	overheadDTO := dtos.UploadAffectedStacksRequest{
		HeadSHA:   dto.HeadSHA,
		BaseSHA:   dto.BaseSHA,
		RepoURL:   dto.RepoURL,
		RepoName:  dto.RepoName,
		RepoOwner: dto.RepoOwner,
		RepoHost:  dto.RepoHost,
		Stacks:    []schema.Affected{},
	}
	overhead := metadataOverhead(overheadDTO)

	return sendChunked(dto.Stacks, c.MaxPayloadBytes, overhead, func(chunk []schema.Affected, batch *BatchInfo) error {
		chunkDTO := &dtos.UploadAffectedStacksRequest{
			HeadSHA:   dto.HeadSHA,
			BaseSHA:   dto.BaseSHA,
			RepoURL:   dto.RepoURL,
			RepoName:  dto.RepoName,
			RepoOwner: dto.RepoOwner,
			RepoHost:  dto.RepoHost,
			Stacks:    chunk,
		}
		if batch != nil {
			chunkDTO.BatchID = batch.BatchID
			chunkDTO.BatchIndex = &batch.BatchIndex
			chunkDTO.BatchTotal = &batch.BatchTotal
		}
		return c.sendAffectedStacksRequest(endpoint, chunkDTO)
	})
}

// sendAffectedStacksRequest sends a single affected stacks upload request.
func (c *AtmosProAPIClient) sendAffectedStacksRequest(url string, dto *dtos.UploadAffectedStacksRequest) error {
	data, err := json.Marshal(dto)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToMarshalPayload, err)
	}

	log.Debug("Uploading affected components and stacks.", logKeyURL, url)

	// Wrap the HTTP call in retry logic to handle transient 401/5xx failures.
	err = doWithRetry("UploadAffectedStacks", func() error {
		req, reqErr := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer(data))
		if reqErr != nil {
			return errors.Join(errUtils.ErrFailedToCreateAuthRequest, reqErr)
		}

		resp, doErr := c.HTTPClient.Do(req) //nolint:gosec // URL constructed from trusted config, not user input.
		if doErr != nil {
			return errors.Join(errUtils.ErrFailedToMakeRequest, doErr)
		}
		defer resp.Body.Close()

		return handleAPIResponse(resp, "UploadAffectedStacks")
	}, c, defaultRetryConfig())
	if err != nil {
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
		// If we can't parse the response as JSON, provide enriched errors for error status codes
		// so users still get troubleshooting hints on lock/unlock failures.
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
			enrichedErr := errUtils.Build(errUtils.ErrFailedToUnmarshalAPIResponse).
				WithCausef("HTTP status: %s", resp.Status).
				WithContext("operation", params.Op).
				WithHint("The API returned an unexpected response format. See troubleshooting: https://atmos-pro.com/docs/learn/troubleshooting").
				Err()
			return errors.Join(params.WrapErr, enrichedErr)
		}
		return errors.Join(errUtils.ErrFailedToUnmarshalAPIResponse, err)
	}

	// Log the structured response for debugging and check success.
	// We need to use type assertion to access the embedded AtmosApiResponse.
	switch responseData := params.Out.(type) {
	case *dtos.LockStackResponse:
		logProAPIResponse(params.Op, responseData.AtmosApiResponse)
		if !responseData.Success {
			return errors.Join(params.WrapErr,
				buildProAPIError(params.Op, resp.StatusCode, responseData.AtmosApiResponse))
		}
	case *dtos.UnlockStackResponse:
		logProAPIResponse(params.Op, responseData.AtmosApiResponse)
		if !responseData.Success {
			return errors.Join(params.WrapErr,
				buildProAPIError(params.Op, resp.StatusCode, responseData.AtmosApiResponse))
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
// It returns an *APIError (which implements error) if the response indicates failure, allowing callers
// to inspect the HTTP status code for retry decisions.
func handleAPIResponse(resp *http.Response, operation string) error {
	// Read the response body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToReadResponseBody, err)
	}

	var apiResponse dtos.AtmosApiResponse

	// Try to unmarshal the response to get structured data.
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		// If we can't parse the response as JSON, handle based on status code.
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
			enrichedErr := errUtils.Build(errUtils.ErrFailedToUnmarshalAPIResponse).
				WithCausef("HTTP status: %s", resp.Status).
				WithContext("operation", operation).
				WithHint("The API returned an unexpected response format. See troubleshooting: https://atmos-pro.com/docs/learn/troubleshooting").
				Err()
			return &APIError{
				StatusCode: resp.StatusCode,
				Operation:  operation,
				Err:        enrichedErr,
			}
		}
		// For successful responses that can't be parsed, just return nil.
		return nil
	}

	// Log the structured response for debugging (only if we successfully unmarshaled).
	logProAPIResponse(operation, apiResponse)

	// For successful HTTP responses, trust the status code over the Success field
	// (some APIs might return minimal responses without the Success field).
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		return nil
	}

	// For error HTTP responses, return an *APIError with the status code so retry logic
	// can distinguish retryable (401, 5xx) from non-retryable (400, 403, 404) failures.
	return &APIError{
		StatusCode: resp.StatusCode,
		Operation:  operation,
		Err:        buildProAPIError(operation, resp.StatusCode, apiResponse),
	}
}

// buildProAPIError creates an enriched error with status-specific hints and documentation links.
// The statusCode should be the HTTP transport status (resp.StatusCode) as the canonical source.
// When unavailable, apiResponse.Status is used as a fallback.
func buildProAPIError(operation string, statusCode int, apiResponse dtos.AtmosApiResponse) error {
	// Normalize: prefer the transport status code, but fall back to apiResponse.Status if needed.
	if statusCode == 0 {
		statusCode = apiResponse.Status
	}
	if apiResponse.Status == 0 {
		apiResponse.Status = statusCode
	}

	errorMsg := logAndReturnProAPIError(operation, apiResponse)

	builder := errUtils.Build(errUtils.ErrAPIResponseError).
		WithCausef("%s", errorMsg).
		WithContext("operation", operation).
		WithContext("status", statusCode)

	if apiResponse.TraceID != "" {
		builder = builder.WithContext("trace_id", apiResponse.TraceID)
	}

	// Add status-specific hints with targeted documentation links.
	// Each hint is self-contained (each renders with its own lightbulb icon).
	switch statusCode {
	case http.StatusForbidden:
		builder = builder.
			WithHint("Permissions are configured per-repository in Atmos Pro. Check that this repo has the required permissions: https://atmos-pro.com/docs/learn/permissions").
			WithHint("For a working example of a properly configured setup, see the quickstart: https://atmos-pro.com/docs/install")
	case http.StatusUnauthorized:
		builder = builder.
			WithHint("The API token may be expired or invalid. If using GitHub OIDC, ensure the workflow has `id-token: write` permission: https://atmos-pro.com/docs/configure/github-workflows").
			WithHint("Learn how Atmos Pro authentication works: https://atmos-pro.com/docs/learn/authentication")
	case http.StatusNotFound:
		builder = builder.
			WithHint("Verify the workspace ID is correct, the repository has been imported, and the Atmos Pro GitHub App is installed: https://atmos-pro.com/docs/install")
	default:
		if statusCode >= http.StatusInternalServerError {
			builder = builder.
				WithHint("This is a server-side error that will be retried automatically. If the problem persists, contact support with the `trace_id` from above: https://atmos-pro.com/docs/learn/troubleshooting")
		}
	}

	return builder.Err()
}

// getGitHubOIDCToken retrieves an OIDC token from GitHub Actions.
// An optional *http.Client can be passed as the second argument; when omitted,
// getHTTPClientWithTimeout is used. This is primarily for test injection.
func getGitHubOIDCToken(githubOIDCSettings schema.GithubOIDCSettings, clients ...*http.Client) (string, error) {
	requestURL := githubOIDCSettings.RequestURL
	requestToken := githubOIDCSettings.RequestToken

	if requestURL == "" || requestToken == "" {
		return "", errUtils.ErrNotInGitHubActions
	}

	// Parse and validate the URL to prevent SSRF: scheme must be https and host must
	// be non-empty.
	u, err := url.Parse(requestURL)
	if err != nil {
		return "", fmt.Errorf("%w: invalid ACTIONS_ID_TOKEN_REQUEST_URL: %w", errUtils.ErrFailedToGetGitHubOIDCToken, err)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_URL must use https scheme, got %q", errUtils.ErrFailedToGetGitHubOIDCToken, u.Scheme)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_URL must have a non-empty host", errUtils.ErrFailedToGetGitHubOIDCToken)
	}

	// Add audience parameter to the request URL using proper URL manipulation.
	q := u.Query()
	q.Set("audience", "atmos-pro.com")
	u.RawQuery = q.Encode()
	requestOIDCTokenURL := u.String()
	log.Debug("requestOIDCTokenURL", "requestOIDCTokenURL", requestOIDCTokenURL)

	req, err := http.NewRequest("GET", requestOIDCTokenURL, nil)
	if err != nil {
		return "", errors.Join(errUtils.ErrFailedToCreateRequest, err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", requestToken))

	var client *http.Client
	if len(clients) > 0 && clients[0] != nil {
		client = clients[0]
	} else if oidcHTTPClientOverride != nil {
		client = oidcHTTPClientOverride
	} else {
		client = getHTTPClientWithTimeout()
	}
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
		// If we can't parse the response as JSON, provide enriched errors for error status codes.
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
			enrichedErr := errUtils.Build(errUtils.ErrFailedToDecodeTokenResponse).
				WithCausef("HTTP status: %s", resp.Status).
				WithContext("operation", "ExchangeOIDCToken").
				WithHint("The API returned an unexpected response format. See troubleshooting: https://atmos-pro.com/docs/learn/troubleshooting").
				Err()
			return "", errors.Join(errUtils.ErrFailedToExchangeOIDCToken, enrichedErr)
		}
		return "", errors.Join(errUtils.ErrFailedToDecodeTokenResponse, err)
	}

	if !tokenResp.Success {
		return "", errors.Join(errUtils.ErrFailedToExchangeOIDCToken,
			buildProAPIError("ExchangeOIDCToken", resp.StatusCode, tokenResp.AtmosApiResponse))
	}

	return tokenResp.Data.Token, nil
}
