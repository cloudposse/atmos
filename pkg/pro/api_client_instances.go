package pro

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/utils"
)

// UploadInstances uploads drift detection data to the API.
func (c *AtmosProAPIClient) UploadInstances(dto *dtos.InstancesUploadRequest) error {
	endpoint := fmt.Sprintf("%s/%s/instances", c.BaseURL, c.BaseAPIEndpoint)

	// Guard against nil HTTPClient by ensuring a default client with a sane timeout
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	data, err := utils.ConvertToJSON(dto)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrFailedToMarshalPayload, err)
	}

	// Log safe metadata instead of full payload to prevent secret leakage
	hash := sha256.Sum256([]byte(data))
	log.Debug("Uploading instances DTO.", "repo_owner", dto.RepoOwner, "repo_name", dto.RepoName, "instances_count", len(dto.Instances), "payload_hash", hex.EncodeToString(hash[:]))

	req, err := getAuthenticatedRequest(c, "POST", endpoint, strings.NewReader(data))
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrFailedToCreateAuthRequest, err)
	}

	log.Debug("Uploading instances.", "endpoint", endpoint)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrFailedToMakeRequest, err)
	}
	defer resp.Body.Close()

	if err := handleAPIResponse(resp, "UploadInstances"); err != nil {
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrFailedToUploadInstances, err)
	}
	log.Debug("Uploaded instances.", "endpoint", endpoint)

	return nil
}
