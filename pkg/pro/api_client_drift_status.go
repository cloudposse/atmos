package pro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// UploadDriftResultStatus uploads the drift detection result status to the pro API.
func (c *AtmosProAPIClient) UploadDriftResultStatus(dto *DriftStatusUploadRequest) error {
	url := fmt.Sprintf("%s/%s/drift-status", c.BaseURL, c.BaseAPIEndpoint)
	c.Logger.Trace(fmt.Sprintf("\nUploading drift status at %s", url))

	data, err := json.Marshal(dto)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToMarshalRequestBody, err)
	}

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToCreateAuthRequest, err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf(cfg.ErrFormatString, ErrFailedToUploadDriftStatus, resp.Status)
	}

	c.Logger.Trace(fmt.Sprintf("\nUploaded drift status at %s", url))

	return nil
}
