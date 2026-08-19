package pro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

// UploadExecMetadata uploads a single command-execution record to Atmos Pro.
// The base envelope and resource-usage metrics are small and bounded, so
// they are always sent in full. dto.Data may be arbitrarily large (e.g. the
// per-resource change list for terraform plan/apply): UploadExecMetadata
// marshals the whole record once and, when it is under the payload size
// threshold, sends Data inline; when at/over the threshold, it first uploads
// Data out-of-band via UploadExecData (a single request, never chunked) and
// sends the returned URL in Data's place instead (FR-011, research.md
// Decision 16).
func (c *AtmosProAPIClient) UploadExecMetadata(dto *dtos.ExecUploadRequest) error {
	url := fmt.Sprintf("%s/%s/atmos/exec", c.BaseURL, c.BaseAPIEndpoint)

	maxBytes := c.MaxPayloadBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxPayloadBytes
	}

	marshaled, err := json.Marshal(dto)
	if err != nil {
		return wrapErr(errUtils.ErrFailedToMarshalPayload, err)
	}

	if len(marshaled) < maxBytes {
		return c.sendExecMetadataRequest(url, dto)
	}

	dataResp, err := c.UploadExecData(&dtos.ExecDataUploadRequest{
		ExecutionID: dto.ExecutionID,
		Data:        dto.Data,
	})
	if err != nil {
		return err
	}

	urlJSON, err := json.Marshal(dataResp.URL)
	if err != nil {
		return wrapErr(errUtils.ErrFailedToMarshalPayload, err)
	}

	outOfBandDTO := *dto
	outOfBandDTO.Data = urlJSON

	return c.sendExecMetadataRequest(url, &outOfBandDTO)
}

// sendExecMetadataRequest sends a single command-execution metadata request.
func (c *AtmosProAPIClient) sendExecMetadataRequest(url string, dto *dtos.ExecUploadRequest) error {
	data, err := json.Marshal(dto)
	if err != nil {
		return wrapErr(errUtils.ErrFailedToMarshalPayload, err)
	}

	log.Debug("Uploading command-execution metadata.", logKeyURL, url)

	err = doWithRetry("UploadExecMetadata", func() error {
		req, reqErr := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer(data))
		if reqErr != nil {
			return wrapErr(errUtils.ErrFailedToCreateAuthRequest, reqErr)
		}

		resp, doErr := c.HTTPClient.Do(req) //nolint:gosec // URL constructed from trusted config, not user input.
		if doErr != nil {
			return wrapErr(errUtils.ErrFailedToMakeRequest, doErr)
		}
		defer resp.Body.Close()

		return handleAPIResponse(resp, "UploadExecMetadata")
	}, c, defaultRetryConfig())
	if err != nil {
		return wrapErr(errUtils.ErrFailedToUploadExecMetadata, err)
	}

	log.Debug("Uploaded command-execution metadata.", logKeyURL, url)

	return nil
}

// UploadExecData uploads a command's structured Data out-of-band, in a
// single request (never chunked — the blob store handles arbitrarily large
// single uploads), when the parent ExecutionRecord would otherwise exceed
// the payload size threshold (FR-011). Returns the blob's URL on success.
func (c *AtmosProAPIClient) UploadExecData(dto *dtos.ExecDataUploadRequest) (*dtos.ExecDataUploadResponse, error) {
	url := fmt.Sprintf("%s/%s/atmos/exec/data", c.BaseURL, c.BaseAPIEndpoint)

	data, err := json.Marshal(dto)
	if err != nil {
		return nil, wrapErr(errUtils.ErrFailedToMarshalPayload, err)
	}

	log.Debug("Uploading out-of-band command-execution data.", logKeyURL, url)

	var respDTO dtos.ExecDataUploadResponse
	err = doWithRetry("UploadExecData", func() error {
		req, reqErr := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer(data))
		if reqErr != nil {
			return wrapErr(errUtils.ErrFailedToCreateAuthRequest, reqErr)
		}

		resp, doErr := c.HTTPClient.Do(req) //nolint:gosec // URL constructed from trusted config, not user input.
		if doErr != nil {
			return wrapErr(errUtils.ErrFailedToMakeRequest, doErr)
		}
		defer resp.Body.Close()

		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return wrapErr(errUtils.ErrFailedToReadResponseBody, readErr)
		}

		var parsed dtos.ExecDataUploadResponse
		if unmarshalErr := json.Unmarshal(body, &parsed); unmarshalErr != nil {
			if resp.StatusCode < 200 || resp.StatusCode >= 400 {
				return &APIError{StatusCode: resp.StatusCode, Operation: "UploadExecData", Err: unmarshalErr}
			}
			return nil
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			respDTO = parsed
			return nil
		}

		return &APIError{
			StatusCode: resp.StatusCode,
			Operation:  "UploadExecData",
			Err:        buildProAPIError("UploadExecData", resp.StatusCode, parsed.AtmosApiResponse),
		}
	}, c, defaultRetryConfig())
	if err != nil {
		return nil, wrapErr(errUtils.ErrFailedToUploadExecData, err)
	}

	log.Debug("Uploaded out-of-band command-execution data.", logKeyURL, url)

	return &respDTO, nil
}
