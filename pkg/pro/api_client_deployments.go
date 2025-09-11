package pro

import (
	"bytes"
	"fmt"

	log "github.com/charmbracelet/log"
	atmosErrors "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/utils"
)

// UploadDeployments uploads drift detection data to the API.
func (c *AtmosProAPIClient) UploadDeployments(dto *dtos.DeploymentsUploadRequest) error {
	url := fmt.Sprintf("%s/%s/deployments", c.BaseURL, c.BaseAPIEndpoint)

	data, err := utils.ConvertToJSON(dto)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, atmosErrors.ErrFailedToMarshalPayload, err)
	}

	log.Debug(fmt.Sprintf("Uploading deployments DTO: %s", data))

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer([]byte(data)))
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, atmosErrors.ErrFailedToCreateAuthRequest, err)
	}

	log.Debug(fmt.Sprintf("\nUploading deployments to %s", url))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, atmosErrors.ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if err := handleAPIResponse(resp, "UploadDeployments"); err != nil {
		return fmt.Errorf(cfg.ErrFormatString, atmosErrors.ErrFailedToUploadDeploymentStatus, err)
	}
	log.Debug(fmt.Sprintf("\nUploaded deployments to %s", url))

	return nil
}
