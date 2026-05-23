package pro

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

// UploadSecurityFindings uploads a SARIF security report to Atmos Pro.
// The endpoint is currently experimental — callers should advertise this to
// users (e.g., via an "(experimental)" flag description).
func (c *AtmosProAPIClient) UploadSecurityFindings(dto *dtos.SecurityFindingsUploadRequest) error {
	defer perf.Track(nil, "pro.AtmosProAPIClient.UploadSecurityFindings")()

	if dto == nil {
		return wrapErr(errUtils.ErrAWSSecurityUploadFailed,
			fmt.Errorf("UploadSecurityFindings: %w", errUtils.ErrNilRequestDTO))
	}

	endpoint := fmt.Sprintf("%s/%s/security-findings", c.BaseURL, c.BaseAPIEndpoint)

	data, err := json.Marshal(dto)
	if err != nil {
		return wrapErr(errUtils.ErrFailedToMarshalPayload, err)
	}

	// Hash the payload for safe debug logging — the SARIF body may contain
	// resource ARNs and other sensitive identifiers, so we never log it raw.
	hash := sha256.Sum256(data)
	log.Debug(
		"Uploading security findings DTO.",
		"repo_owner", dto.RepoOwner,
		"repo_name", dto.RepoName,
		"format", dto.Format,
		"payload_bytes", len(data),
		"payload_hash", hex.EncodeToString(hash[:]),
	)

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: DefaultHTTPClientTimeout}
	}

	err = doWithRetry("UploadSecurityFindings", func() error {
		req, reqErr := getAuthenticatedRequest(c, http.MethodPost, endpoint, bytes.NewReader(data))
		if reqErr != nil {
			return wrapErr(errUtils.ErrFailedToCreateAuthRequest, reqErr)
		}

		resp, doErr := client.Do(req) //nolint:gosec // URL built from trusted config.
		if doErr != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return wrapErr(errUtils.ErrFailedToMakeRequest, doErr)
		}
		defer resp.Body.Close()

		return handleAPIResponse(resp, "UploadSecurityFindings")
	}, c, defaultRetryConfig())
	if err != nil {
		return wrapErr(errUtils.ErrAWSSecurityUploadFailed, err)
	}

	log.Debug("Uploaded security findings.", logKeyURL, endpoint)
	return nil
}
