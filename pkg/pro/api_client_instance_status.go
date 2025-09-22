package pro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

// UploadInstanceStatus uploads the drift detection result status to the pro API.
func (c *AtmosProAPIClient) UploadInstanceStatus(dto *dtos.InstanceStatusUploadRequest) error {
	// Use the correct endpoint format: /api/v1/repos/{owner}/{repo}/instances/{stack}/{component}
	targetUrl := fmt.Sprintf("%s/%s/repos/%s/%s/instances/%s/%s",
		c.BaseURL, c.BaseAPIEndpoint,
		url.PathEscape(dto.RepoOwner),
		url.PathEscape(dto.RepoName),
		url.PathEscape(dto.Stack),
		url.PathEscape(dto.Component))
	log.Debug("Uploading drift status.", "url", targetUrl)

	// Map HasDrift to the correct status format
	status := "in_sync"
	if dto.HasDrift {
		status = "drifted"
	}

	// Create the correct payload structure expected by the API
	payload := map[string]interface{}{
		"status": status,
	}

	// Add last_run if we have atmos_pro_run_id or git_sha
	if dto.AtmosProRunID != "" || dto.GitSHA != "" {
		payload["last_run"] = time.Now().UTC().Format(time.RFC3339)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "PATCH", targetUrl, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if err := handleAPIResponse(resp, "UploadInstanceStatus"); err != nil {
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrFailedToUploadInstanceStatus, err)
	}

	log.Debug("Uploaded instance status.", "url", targetUrl)

	return nil
}
