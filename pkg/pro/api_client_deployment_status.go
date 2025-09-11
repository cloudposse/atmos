package pro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	log "github.com/charmbracelet/log"
	atmosErrors "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

// UploadDeploymentStatus uploads the drift detection result status to the pro API.
func (c *AtmosProAPIClient) UploadDeploymentStatus(dto *dtos.DeploymentStatusUploadRequest) error {
	// Use the correct endpoint format: /api/v1/repos/{owner}/{repo}/deployments/{stack}/{component}
	url := fmt.Sprintf("%s/%s/repos/%s/%s/deployments/%s/%s",
		c.BaseURL, c.BaseAPIEndpoint,
		url.PathEscape(dto.RepoOwner),
		url.PathEscape(dto.RepoName),
		url.PathEscape(dto.Stack),
		url.PathEscape(dto.Component))
	log.Debug(fmt.Sprintf("\nUploading drift status at %s", url))

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
		return fmt.Errorf(atmosErrors.ErrWrappingFormat, atmosErrors.ErrFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "PATCH", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf(atmosErrors.ErrWrappingFormat, atmosErrors.ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf(atmosErrors.ErrWrappingFormat, atmosErrors.ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if err := handleAPIResponse(resp, "UploadDeploymentStatus"); err != nil {
		return fmt.Errorf(atmosErrors.ErrWrappingFormat, atmosErrors.ErrFailedToUploadDeploymentStatus, err)
	}

	log.Debug(fmt.Sprintf("\nUploaded deployment status at %s", url))

	return nil
}
