package pro

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	log "github.com/charmbracelet/log"
	atmosErrors "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/utils"
)

// UploadDeployments uploads drift detection data to the API.
func (c *AtmosProAPIClient) UploadDeployments(dto *dtos.DeploymentsUploadRequest) error {
	url := fmt.Sprintf("%s/%s/deployments", c.BaseURL, c.BaseAPIEndpoint)

	data, err := utils.ConvertToJSON(dto)
	if err != nil {
		return fmt.Errorf(atmosErrors.ErrWrappingFormat, atmosErrors.ErrFailedToMarshalPayload, err)
	}

	// Log safe metadata instead of full payload to prevent secret leakage
	hash := sha256.Sum256([]byte(data))
	log.Debug(fmt.Sprintf("Uploading deployments DTO: repo=%s/%s, deployments_count=%d, payload_hash=%s",
		dto.RepoOwner, dto.RepoName, len(dto.Deployments), hex.EncodeToString(hash[:])))

	req, err := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer([]byte(data)))
	if err != nil {
		return fmt.Errorf(atmosErrors.ErrWrappingFormat, atmosErrors.ErrFailedToCreateAuthRequest, err)
	}

	log.Debug(fmt.Sprintf("\nUploading deployments to %s", url))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf(atmosErrors.ErrWrappingFormat, atmosErrors.ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if err := handleAPIResponse(resp, "UploadDeployments"); err != nil {
		return fmt.Errorf(atmosErrors.ErrWrappingFormat, atmosErrors.ErrFailedToUploadDeployments, err)
	}
	log.Debug(fmt.Sprintf("\nUploaded deployments to %s", url))

	return nil
}
