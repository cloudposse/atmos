package pro

import (
	"bytes"
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

// UploadExecMetadata uploads a single command-execution record to Atmos Pro.
// The base envelope, resource-usage metrics, and structured-data summary
// (dto.Data) are small and bounded, so they are always sent in full and
// repeated on every request. When dto.DataItems (the potentially large,
// per-resource structured data) would push the marshaled record over the
// payload size limit, it is split across multiple correlated requests —
// never truncated or dropped — reusing the same sendChunked/BatchInfo
// mechanism as UploadAffectedStacks and UploadInstances.
func (c *AtmosProAPIClient) UploadExecMetadata(dto *dtos.ExecUploadRequest) error {
	url := fmt.Sprintf("%s/%s/atmos/exec", c.BaseURL, c.BaseAPIEndpoint)

	// Estimate metadata overhead (everything except the DataItems array).
	overheadDTO := dtos.ExecUploadRequest{
		AtmosProRunID: dto.AtmosProRunID,
		AtmosVersion:  dto.AtmosVersion,
		AtmosOS:       dto.AtmosOS,
		AtmosArch:     dto.AtmosArch,
		Command:       dto.Command,
		Args:          dto.Args,
		ExitCode:      dto.ExitCode,
		GitSHA:        dto.GitSHA,
		RepoURL:       dto.RepoURL,
		RepoName:      dto.RepoName,
		RepoOwner:     dto.RepoOwner,
		RepoHost:      dto.RepoHost,
		Metrics:       dto.Metrics,
		Data:          dto.Data,
		DataItems:     []json.RawMessage{},
	}
	overhead := metadataOverhead(overheadDTO)

	return sendChunked(dto.DataItems, c.MaxPayloadBytes, overhead, func(chunk []json.RawMessage, batch *BatchInfo) error {
		chunkDTO := &dtos.ExecUploadRequest{
			AtmosProRunID: dto.AtmosProRunID,
			AtmosVersion:  dto.AtmosVersion,
			AtmosOS:       dto.AtmosOS,
			AtmosArch:     dto.AtmosArch,
			Command:       dto.Command,
			Args:          dto.Args,
			ExitCode:      dto.ExitCode,
			GitSHA:        dto.GitSHA,
			RepoURL:       dto.RepoURL,
			RepoName:      dto.RepoName,
			RepoOwner:     dto.RepoOwner,
			RepoHost:      dto.RepoHost,
			Metrics:       dto.Metrics,
			Data:          dto.Data,
			DataItems:     chunk,
		}
		if batch != nil {
			chunkDTO.BatchID = batch.BatchID
			chunkDTO.BatchIndex = &batch.BatchIndex
			chunkDTO.BatchTotal = &batch.BatchTotal
		}
		return c.sendExecMetadataRequest(url, chunkDTO)
	})
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
