package pro

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/utils"
)

var ErrFailedToUploadDriftDetection = errors.New("failed to upload drift detection results")

// UploadDriftDetection uploads drift detection data to the API.
func (c *AtmosProAPIClient) UploadDriftDetection(dto *dtos.DriftDetectionUploadRequest) error {
	url := fmt.Sprintf("%s/%s/drift-detection", c.BaseURL, c.BaseAPIEndpoint)

	data, err := utils.ConvertToJSON(dto)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToMarshalPayload, err)
	}

	c.Logger.Debug(fmt.Sprintf("Uploading drift detection DTO: %s", data))

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer([]byte(data)))
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToCreateAuthRequest, err)
	}

	c.Logger.Trace(fmt.Sprintf("\nUploading drift detection results to %s", url))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToUploadDriftDetection, resp.Status)
	}
	c.Logger.Trace(fmt.Sprintf("\nUploaded drift detection results to %s", url))

	return nil
}
