package pro

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	// DefaultHTTPClientTimeout is the default timeout for HTTP client requests.
	DefaultHTTPClientTimeout = 10 * time.Second
)

// UploadInstances uploads drift detection data to the API.
// It retries on transient 401/5xx failures with exponential backoff, refreshing
// the OIDC token on 401 errors before each retry.
func (c *AtmosProAPIClient) UploadInstances(dto *dtos.InstancesUploadRequest) error {
	if dto == nil {
		return errors.Join(
			errUtils.ErrFailedToUploadInstances,
			fmt.Errorf("UploadInstances: %w", errUtils.ErrNilRequestDTO),
		)
	}
	endpoint := fmt.Sprintf("%s/%s/instances", c.BaseURL, c.BaseAPIEndpoint)

	// Guard against nil HTTPClient by ensuring a default client with a sane timeout.
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: DefaultHTTPClientTimeout}
	}

	data, err := utils.ConvertToJSON(dto)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToMarshalPayload, err)
	}

	// Log safe metadata instead of full payload to prevent secret leakage.
	hash := sha256.Sum256([]byte(data))
	log.Debug("Uploading instances DTO.", "repo_owner", dto.RepoOwner, "repo_name", dto.RepoName, "instances_count", len(dto.Instances), "payload_hash", hex.EncodeToString(hash[:]))

	log.Debug("Uploading instances.", "endpoint", endpoint)

	// Wrap the HTTP call in retry logic to handle transient 401/5xx failures.
	err = doWithRetry("UploadInstances", func() error {
		req, reqErr := getAuthenticatedRequest(c, "POST", endpoint, strings.NewReader(data))
		if reqErr != nil {
			return errors.Join(errUtils.ErrFailedToCreateAuthRequest, reqErr)
		}

		resp, doErr := client.Do(req) //nolint:gosec // URL constructed from trusted config, not user input.
		if doErr != nil {
			return errors.Join(errUtils.ErrFailedToMakeRequest, doErr)
		}
		defer resp.Body.Close()

		return handleAPIResponse(resp, "UploadInstances")
	}, c, defaultRetryConfig())
	if err != nil {
		return errors.Join(errUtils.ErrFailedToUploadInstances, err)
	}

	log.Debug("Uploaded instances.", "endpoint", endpoint)

	return nil
}
