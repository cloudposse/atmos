package pro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	log "github.com/cloudposse/atmos/pkg/logger"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

const operationCreateCommit = "CreateCommit"

// CreateCommit sends file changes to Atmos Pro to create a server-side commit
// via the GitHub App. It retries on transient 401/5xx failures with exponential
// backoff, refreshing the OIDC token on 401 errors before each retry.
func (c *AtmosProAPIClient) CreateCommit(dto *dtos.CommitRequest) (*dtos.CommitResponse, error) {
	if dto == nil {
		return nil, wrapErr(errUtils.ErrFailedToCreateCommit, errUtils.ErrNilRequestDTO)
	}

	targetURL := fmt.Sprintf("%s/%s/git/commit", c.BaseURL, c.BaseAPIEndpoint)
	log.Debug("Creating commit via Atmos Pro.", logKeyURL, targetURL)

	data, err := json.Marshal(dto)
	if err != nil {
		return nil, wrapErr(errUtils.ErrFailedToMarshalPayload, err)
	}

	var commitResp dtos.CommitResponse

	err = doWithRetry(operationCreateCommit, func() error {
		return c.sendCommitRequest(targetURL, data, &commitResp)
	}, c, defaultRetryConfig())
	if err != nil {
		return nil, wrapErr(errUtils.ErrFailedToCreateCommit, err)
	}

	log.Debug("Created commit via Atmos Pro.", logKeyURL, targetURL, "sha", commitResp.Data.SHA)

	return &commitResp, nil
}

// sendCommitRequest executes a single commit request and parses the response.
func (c *AtmosProAPIClient) sendCommitRequest(targetURL string, data []byte, commitResp *dtos.CommitResponse) error {
	req, reqErr := getAuthenticatedRequest(c, "POST", targetURL, bytes.NewBuffer(data))
	if reqErr != nil {
		return wrapErr(errUtils.ErrFailedToCreateAuthRequest, reqErr)
	}

	// Guard against nil HTTPClient by ensuring a default client with a sane timeout.
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: DefaultHTTPClientTimeout}
	}

	resp, doErr := client.Do(req) //nolint:gosec // URL constructed from trusted config, not user input.
	if doErr != nil {
		// http.Client.Do can return a non-nil response alongside an error
		// (e.g., on redirect failures); close it to avoid leaking the connection.
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return wrapErr(errUtils.ErrFailedToMakeRequest, doErr)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return wrapErr(errUtils.ErrFailedToReadResponseBody, readErr)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return buildCommitAPIError(resp, body)
	}

	if jsonErr := json.Unmarshal(body, commitResp); jsonErr != nil {
		return wrapErr(errUtils.ErrFailedToUnmarshalAPIResponse, jsonErr)
	}

	logProAPIResponse(operationCreateCommit, commitResp.AtmosApiResponse)

	return nil
}

// buildCommitAPIError constructs an APIError from an HTTP error response.
func buildCommitAPIError(resp *http.Response, body []byte) error {
	var apiResponse dtos.AtmosApiResponse
	if jsonErr := json.Unmarshal(body, &apiResponse); jsonErr == nil {
		logProAPIResponse(operationCreateCommit, apiResponse)
		return &APIError{
			StatusCode: resp.StatusCode,
			Operation:  operationCreateCommit,
			Err:        buildProAPIError(operationCreateCommit, resp.StatusCode, apiResponse),
		}
	}

	return &APIError{
		StatusCode: resp.StatusCode,
		Operation:  operationCreateCommit,
		Err: errUtils.Build(errUtils.ErrFailedToUnmarshalAPIResponse).
			WithCausef("HTTP status: %s", resp.Status).
			WithContext("operation", operationCreateCommit).
			WithHint("The API returned an unexpected response format. See troubleshooting: https://atmos-pro.com/docs/learn/troubleshooting").
			Err(),
	}
}
