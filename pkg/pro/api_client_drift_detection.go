package pro

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/utils"
)

// UploadDriftDetection uploads drift detection data to the API.
func (c *AtmosProAPIClient) UploadDriftDetection(dto *dtos.DriftDetectionUploadRequest) error {
	url := fmt.Sprintf("%s/%s/drift-detection", c.BaseURL, c.BaseAPIEndpoint)

	data, err := utils.ConvertToJSON(dto)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	c.Logger.Debug(fmt.Sprintf("Uploading drift detection DTO: %s", data))

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer([]byte(data)))
	if err != nil {
		return fmt.Errorf("failed to create authenticated request: %w", err)
	}

	c.Logger.Trace(fmt.Sprintf("\nUploading drift detection results to %s", url))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("failed to upload drift detection results, status: %s", resp.Status)
	}
	c.Logger.Trace(fmt.Sprintf("\nUploaded drift detection results to %s", url))

	return nil
}
