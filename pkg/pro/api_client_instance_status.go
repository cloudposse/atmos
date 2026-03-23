package pro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

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

	// Marshal the full DTO — omitempty tags ensure only populated fields are sent.
	// This sends all fields including version/OS/arch metadata and resource metrics.
	data, err := json.Marshal(dto)
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
