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

	// Add component type if present.
	if dto.ComponentType != "" {
		payload["component_type"] = dto.ComponentType
	}

	// Add metadata if present.
	if dto.Metadata != nil {
		payload["metadata"] = dto.Metadata
	}

	// Add last_run if we have atmos_pro_run_id or git_sha
	if dto.AtmosProRunID != "" || dto.GitSHA != "" {
		payload["last_run"] = time.Now().UTC().Format(time.RFC3339)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToMarshalPayload, err)
	}

	req, err := getAuthenticatedRequest(c, "PATCH", targetURL, bytes.NewBuffer(data))
	if err != nil {
		return errors.Join(errUtils.ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if err := handleAPIResponse(resp, "UploadInstanceStatus"); err != nil {
		return errors.Join(errUtils.ErrFailedToUploadInstanceStatus, err)
	}

	log.Debug("Uploaded instance status.", "url", targetURL)

	return nil
}
