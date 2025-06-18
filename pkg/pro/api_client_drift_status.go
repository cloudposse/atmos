package pro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	log "github.com/charmbracelet/log"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

// UploadDriftResultStatus uploads the drift detection result status to the pro API.
func (c *AtmosProAPIClient) UploadDriftResultStatus(dto *dtos.DeploymentStatusUploadRequest) error {
	url := fmt.Sprintf("%s/%s/deployments", c.BaseURL, c.BaseAPIEndpoint)
	log.Debug(fmt.Sprintf("\nUploading drift status at %s", url))

	data, err := json.Marshal(dto)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "PATCH", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToUploadDriftStatus, resp.Status)
	}

	log.Debug(fmt.Sprintf("\nUploaded deployment status at %s", url))

	return nil
}
