package pro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

// UploadInstanceStatus uploads the drift detection result status to the pro API.
// It retries on transient 401/5xx failures with exponential backoff, refreshing
// the OIDC token on 401 errors before each retry.
func (c *AtmosProAPIClient) UploadInstanceStatus(dto *dtos.InstanceStatusUploadRequest) error {
	if dto == nil {
		return errors.Join(errUtils.ErrFailedToUploadInstanceStatus, errUtils.ErrNilRequestDTO)
	}
	// Use the correct endpoint format: /api/v1/repos/{owner}/{repo}/instances?stack={stack}&component={component}.
	targetURL := fmt.Sprintf("%s/%s/repos/%s/%s/instances?stack=%s&component=%s",
		c.BaseURL, c.BaseAPIEndpoint,
		url.PathEscape(dto.RepoOwner),
		url.PathEscape(dto.RepoName),
		url.QueryEscape(dto.Stack),
		url.QueryEscape(dto.Component))
	log.Debug("Uploading drift status.", "url", targetURL)

	// Send raw command and exit code — the server interprets them.
	payload := map[string]interface{}{
		"command":   dto.Command,
		"exit_code": dto.ExitCode,
	}

	// Add last_run if we have atmos_pro_run_id or git_sha.
	if dto.AtmosProRunID != "" || dto.GitSHA != "" {
		payload["last_run"] = time.Now().UTC().Format(time.RFC3339)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToMarshalPayload, err)
	}

	// Wrap the HTTP call in retry logic to handle transient 401/5xx failures.
	err = doWithRetry("UploadInstanceStatus", func() error {
		req, reqErr := getAuthenticatedRequest(c, "PATCH", targetURL, bytes.NewBuffer(data))
		if reqErr != nil {
			return errors.Join(errUtils.ErrFailedToCreateAuthRequest, reqErr)
		}

		resp, doErr := c.HTTPClient.Do(req) //nolint:gosec // URL constructed from trusted config, not user input.
		if doErr != nil {
			return errors.Join(errUtils.ErrFailedToMakeRequest, doErr)
		}
		defer resp.Body.Close()

		return handleAPIResponse(resp, "UploadInstanceStatus")
	}, c, defaultRetryConfig())
	if err != nil {
		return errors.Join(errUtils.ErrFailedToUploadInstanceStatus, err)
	}

	log.Debug("Uploaded instance status.", "url", targetURL)

	return nil
}
